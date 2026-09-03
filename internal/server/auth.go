package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/mostlygeek/llama-swap/internal/chain"
	"github.com/mostlygeek/llama-swap/internal/config"
	"github.com/mostlygeek/llama-swap/internal/swaputil"
)

// CreateAuthMiddleware returns middleware that validates API keys when the
// config declares any. It accepts the key via Authorization: Bearer,
// Authorization: Basic (password field), or x-api-key. When auth.trustedHeader
// is configured a request vouched for by the proxy is accepted as well, so a
// web UI session can reach inference. When no keys are configured the
// middleware is a pass-through.
func CreateAuthMiddleware(cfg config.Config) chain.Middleware {
	keys := cfg.RequiredAPIKeys
	return func(next http.Handler) http.Handler {
		if len(keys) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hasValidAPIKey(r, keys) && !isTrustedUser(r, cfg.Auth) {
				w.Header().Set("WWW-Authenticate", `Basic realm="llama-swap"`)
				swaputil.SendResponse(w, r, http.StatusUnauthorized, "unauthorized: invalid or missing API key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CreateUIAuthMiddleware returns the middleware for the web UI and the
// endpoints only it uses. Without auth.trustedHeader it is the API key
// middleware, so apiKeys guard the UI as they always have. With it, only
// requests vouched for by the proxy are accepted; API keys never open the UI.
func CreateUIAuthMiddleware(cfg config.Config) chain.Middleware {
	if !cfg.Auth.Enabled() {
		return CreateAuthMiddleware(cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isTrustedUser(r, cfg.Auth) {
				swaputil.SendResponse(w, r, http.StatusUnauthorized,
					"unauthorized: request did not come through the authenticating proxy")
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

// isTrustedUser reports whether the request carries a non-empty trusted
// header and arrived from a trusted proxy.
func isTrustedUser(r *http.Request, auth config.AuthConfig) bool {
	if !auth.Enabled() {
		return false
	}
	if strings.TrimSpace(r.Header.Get(auth.TrustedHeader)) == "" {
		return false
	}
	return auth.IsTrustedSource(remoteIP(r))
}

// remoteIP parses the peer address of the connection. It deliberately ignores
// X-Forwarded-For, which anyone can set; trustedProxies is about who is
// connected to us, not who they claim to relay.
func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
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
