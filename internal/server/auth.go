package server

import (
	"context"
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
func CreateUIAuthMiddleware(cfg config.Config) chain.Middleware {
	if cfg.Auth.UIMode() == config.UIAuthNone {
		return func(next http.Handler) http.Handler { return next }
	}
	return CreateAuthMiddleware(cfg)
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

type doorPrefixKey struct{}

// doorPrefix returns "/ui" when the request came through the UI door, or ""
// when it came through the public door. Handlers that send a redirect use it
// so the browser stays behind the same door.
func doorPrefix(r *http.Request) string {
	if p, ok := r.Context().Value(doorPrefixKey{}).(string); ok {
		return p
	}
	return ""
}

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
func uiDoor(inner *http.ServeMux, static http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, uiPrefix+"/") {
			static.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(context.WithValue(r.Context(), doorPrefixKey{}, uiPrefix))
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, uiPrefix)
		r2.URL.RawPath = swaputil.EscapedPathSuffix(r.URL.EscapedPath(), uiPrefix)
		if _, pattern := inner.Handler(r2); pattern == "" {
			static.ServeHTTP(w, r)
			return
		}
		inner.ServeHTTP(w, r2)
	})
}

// CreateRequestContextMiddleware returns middleware that extracts model and
// auth info from the request into the context. Requests where no model can be
// identified are rejected with a 404.
func CreateRequestContextMiddleware(cfg config.Config) chain.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = markInflightStart(r)
			data, err := swaputil.FetchContext(r, cfg)
			if err != nil {
				swaputil.SendError(w, r, swaputil.ErrNoModelInContext)
				return
			}
			_ = data
			next.ServeHTTP(w, r)
		})
	}
}

// CreateCORSMiddleware returns middleware that answers OPTIONS preflight
// requests with permissive CORS headers (see issues #81, #77, #42). Non-OPTIONS
// requests pass through untouched.
func CreateCORSMiddleware() chain.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if headers := r.Header.Get("Access-Control-Request-Headers"); headers != "" {
				w.Header().Set("Access-Control-Allow-Headers", sanitizeAccessControlRequestHeaderValues(headers))
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-Requested-With")
			}
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func isTokenChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
	case r >= 'A' && r <= 'Z':
	case r >= '0' && r <= '9':
	case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
	default:
		return false
	}
	return true
}

// sanitizeAccessControlRequestHeaderValues drops any header names that contain
// characters outside the HTTP token grammar before echoing them back.
func sanitizeAccessControlRequestHeaderValues(headerValues string) string {
	parts := strings.Split(headerValues, ",")
	valid := make([]string, 0, len(parts))

	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}

		validPart := true
		for _, c := range v {
			if !isTokenChar(c) {
				validPart = false
				break
			}
		}
		if validPart {
			valid = append(valid, v)
		}
	}

	return strings.Join(valid, ", ")
}
