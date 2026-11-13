package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/mostlygeek/llama-swap/internal/chain"
	"github.com/mostlygeek/llama-swap/internal/config"
	"github.com/mostlygeek/llama-swap/internal/swaputil"
)

// CreateAuthMiddleware returns middleware that validates API keys when the
// config declares any. It accepts the key via Authorization: Bearer,
// Authorization: Basic (password field), or x-api-key. Nothing else is
// accepted, so the public door behaves like any hosted inference API. When
// no keys are configured the middleware is a pass-through.
func CreateAuthMiddleware(cfg config.Config) chain.Middleware {
	keys := cfg.RequiredAPIKeys
	return func(next http.Handler) http.Handler {
		if len(keys) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hasValidAPIKey(r, keys) {
				w.Header().Set("WWW-Authenticate", `Basic realm="llama-swap"`)
				swaputil.SendResponse(w, r, http.StatusUnauthorized, "unauthorized: invalid or missing API key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CreateUIAuthMiddleware returns the check for the UI door, based on auth.ui:
//
//   - apiKeys (default): the API key check, so apiKeys guard the web UI as
//     they always have.
//   - none: no check. A reverse proxy must gate the web UI.
//
// A Tailcat request always gets the API key check, in both modes. Tailcat
// traffic arrives on its own listener and never passes the reverse proxy, so
// none mode would leave the web UI open to it.
func CreateUIAuthMiddleware(cfg config.Config) chain.Middleware {
	keyCheck := CreateAuthMiddleware(cfg)
	if cfg.Auth.UIMode() != config.UIAuthNone {
		return keyCheck
	}
	return func(next http.Handler) http.Handler {
		guarded := keyCheck(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isTailcatRequest(r.Context()) {
				guarded.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func hasValidAPIKey(r *http.Request, keys []string) bool {
	provided := swaputil.ExtractAPIKey(r)
	if provided == "" {
		return false
	}
	for _, key := range keys {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) == 1 {
			return true
		}
	}
	return false
}

// uiPrefix is the path where the web UI lives. Every request that starts
// with it goes through the UI door. See routes() in server.go.
const uiPrefix = "/ui"

// publicDoor serves inner behind the API key check. A request with no
// matching handler skips the check and gets the same 404 or 405 it always
// got, so unknown paths do not turn into 401s.
func publicDoor(inner *http.ServeMux, mw chain.Chain) http.Handler {
	guarded := mw.Then(inner)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := inner.Handler(r); pattern == "" {
			inner.ServeHTTP(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

// uiDoor serves the web UI. It removes the /ui prefix and looks for a
// matching handler on inner. If one exists, that handler serves the request.
// If none exists, the request is for a static file of the web UI, and static
// serves it with the original path. The caller applies the auth.ui check in
// front of this handler.
//
// A request whose /ui/ prefix is percent-encoded, such as /%75i/, gets a 404.
//
// A wrong method gets a 405, as it does at the public door. If inner has the
// path under a different method, inner answers with its own Allow header.
// Static files are read-only, so any other request that is not a GET or a
// HEAD gets a 405 too.
//
// Redirects need no door awareness here: the handlers send a Location relative
// to the requested URL (see redirectRelative), so a request that arrived on
// /ui/comfyui redirects to /ui/comfyui/ and stays behind this door. The same
// relative Location keeps a reverse proxy's stripped path prefix intact.
func uiDoor(inner *http.ServeMux, static http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The prefix must be the literal text /ui/. ServeMux decodes the path
		// before it matches, so /%75i/ also arrives here. A reverse proxy that
		// matches on the raw path does not see that request as /ui/ and can
		// let it through without a login.
		if !strings.HasPrefix(r.URL.EscapedPath(), uiPrefix+"/") {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, uiPrefix+"/") {
			static.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, uiPrefix)
		r2.URL.RawPath = swaputil.EscapedPathSuffix(r.URL.EscapedPath(), uiPrefix)
		h, pattern := inner.Handler(r2)
		if pattern != "" {
			inner.ServeHTTP(w, r2)
			return
		}
		if isMethodNotAllowed(h, r2) {
			h.ServeHTTP(w, r2)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		static.ServeHTTP(w, r)
	})
}

// isMethodNotAllowed reports whether h, the handler a ServeMux returned with
// an empty pattern, answers 405. ServeMux returns an empty pattern for both
// "no such path" and "path exists under another method", and the status is
// the only way to tell them apart. Both handlers only write an error, so the
// call has no side effects.
func isMethodNotAllowed(h http.Handler, r *http.Request) bool {
	probe := statusProbe{header: http.Header{}}
	h.ServeHTTP(&probe, r)
	return probe.status == http.StatusMethodNotAllowed
}

// statusProbe is a ResponseWriter that keeps the status and drops the body.
type statusProbe struct {
	header http.Header
	status int
}

func (p *statusProbe) Header() http.Header { return p.header }

func (p *statusProbe) WriteHeader(status int) {
	if p.status == 0 {
		p.status = status
	}
}

func (p *statusProbe) Write(b []byte) (int, error) {
	if p.status == 0 {
		p.status = http.StatusOK
	}
	return len(b), nil
}

// CreateRequestContextMiddleware returns middleware that extracts model and
// auth info from the request into the context. Requests where no model can be
// identified are rejected with a 404, and bodies over the size cap with a 413.
//
// JSON and multipart bodies without a Content-Length are first read here, so
// this is where their size cap is usually hit.
func CreateRequestContextMiddleware(cfg config.Config) chain.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = markInflightStart(r)
			data, err := swaputil.FetchContext(r, cfg)
			if err != nil {
				if bodyErr := bodyTooLargeFrom(err); bodyErr != nil {
					swaputil.SendResponse(w, r, bodyErr.status, bodyErr.message)
					return
				}
				swaputil.SendError(w, r, swaputil.ErrNoModelInContext)
				return
			}
			_ = data
			next.ServeHTTP(w, r)
		})
	}
}
