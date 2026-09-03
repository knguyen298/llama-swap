package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mostlygeek/llama-swap/internal/config"
)

func TestServer_SanitizeAccessControlRequestHeaders(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Content-Type, Authorization", "Content-Type, Authorization"},
		{"  X-Custom ,  Accept ", "X-Custom, Accept"},
		{"Valid, Bad Header", "Valid"},
		{"Bad@Header", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := sanitizeAccessControlRequestHeaderValues(c.in); got != c.want {
			t.Errorf("sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestServer_IsTokenChar(t *testing.T) {
	for _, r := range "abcXYZ0129!#$%&'*+-.^_`|~" {
		if !isTokenChar(r) {
			t.Errorf("isTokenChar(%q) = false, want true", r)
		}
	}
	for _, r := range " @()/\t\"" {
		if isTokenChar(r) {
			t.Errorf("isTokenChar(%q) = true, want false", r)
		}
	}
}

func TestServer_RequestContextMiddleware(t *testing.T) {
	cfg := config.Config{
		Models: map[string]config.ModelConfig{
			"llama3": {},
		},
	}

	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := CreateRequestContextMiddleware(cfg)

	t.Run("known model passes through", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llama3"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("missing model returns 404", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestServer_AuthMiddleware(t *testing.T) {
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no keys configured passes through", func(t *testing.T) {
		mw := CreateAuthMiddleware(config.Config{})
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	cfg := config.Config{RequiredAPIKeys: []string{"secret"}}

	t.Run("valid key", func(t *testing.T) {
		mw := CreateAuthMiddleware(cfg)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		mw := CreateAuthMiddleware(cfg)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer wrong")
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
		if w.Header().Get("WWW-Authenticate") == "" {
			t.Error("missing WWW-Authenticate header")
		}
	})
}

func trustedHeaderConfig(proxies ...string) config.Config {
	raw := "apiKeys: [\"secret\"]\nauth:\n  trustedHeader: Remote-User\n"
	if len(proxies) > 0 {
		raw += "  trustedProxies: [" + strings.Join(proxies, ", ") + "]\n"
	}
	cfg, err := config.LoadConfigFromReader(strings.NewReader(raw))
	if err != nil {
		panic(err)
	}
	return cfg
}

func TestServer_UIAuthMiddleware(t *testing.T) {
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	get := func(mw func(http.Handler) http.Handler, key, user, remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		if user != "" {
			r.Header.Set("Remote-User", user)
		}
		if remote != "" {
			r.RemoteAddr = remote
		}
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, r)
		return w
	}

	t.Run("without trustedHeader apiKeys guard the UI", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(config.Config{RequiredAPIKeys: []string{"secret"}})
		if w := get(mw, "secret", "", ""); w.Code != http.StatusOK {
			t.Errorf("key status = %d, want 200", w.Code)
		}
		if w := get(mw, "", "alice", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("header without config status = %d, want 401", w.Code)
		}
	})

	cfg := trustedHeaderConfig()

	t.Run("UI accepts only the trusted header", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(cfg)
		if w := get(mw, "", "alice", ""); w.Code != http.StatusOK {
			t.Errorf("header status = %d, want 200", w.Code)
		}
		if w := get(mw, "secret", "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("api key status = %d, want 401", w.Code)
		}
		if w := get(mw, "", "   ", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("blank header status = %d, want 401", w.Code)
		}
		if w := get(mw, "", "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("nothing status = %d, want 401", w.Code)
		} else if w.Header().Get("WWW-Authenticate") != "" {
			t.Error("UI 401 must not trigger a Basic auth prompt")
		}
	})

	t.Run("inference accepts a key or the trusted header", func(t *testing.T) {
		mw := CreateAuthMiddleware(cfg)
		if w := get(mw, "secret", "", ""); w.Code != http.StatusOK {
			t.Errorf("key status = %d, want 200", w.Code)
		}
		if w := get(mw, "", "alice", ""); w.Code != http.StatusOK {
			t.Errorf("header status = %d, want 200", w.Code)
		}
		if w := get(mw, "", "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("nothing status = %d, want 401", w.Code)
		}
	})

	t.Run("trustedProxies limits who may present the header", func(t *testing.T) {
		limited := trustedHeaderConfig(`"10.0.0.0/8"`, `"192.168.1.1"`)
		mw := CreateUIAuthMiddleware(limited)
		if w := get(mw, "", "alice", "10.2.3.4:5555"); w.Code != http.StatusOK {
			t.Errorf("trusted cidr status = %d, want 200", w.Code)
		}
		if w := get(mw, "", "alice", "192.168.1.1:80"); w.Code != http.StatusOK {
			t.Errorf("trusted ip status = %d, want 200", w.Code)
		}
		if w := get(mw, "", "alice", "203.0.113.9:4444"); w.Code != http.StatusUnauthorized {
			t.Errorf("untrusted source status = %d, want 401", w.Code)
		}
		if w := get(mw, "", "alice", "garbage"); w.Code != http.StatusUnauthorized {
			t.Errorf("unparseable remote status = %d, want 401", w.Code)
		}
		// A spoofed X-Forwarded-For must not help.
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "203.0.113.9:4444"
		r.Header.Set("Remote-User", "alice")
		r.Header.Set("X-Forwarded-For", "10.0.0.1")
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("spoofed X-Forwarded-For status = %d, want 401", w.Code)
		}
	})
}

// TestServer_TrustedHeaderRoutes checks the router: with auth.trustedHeader
// set, the UI and the endpoints only it uses require the header and reject
// API keys, while inference, the model list and the operations endpoints
// accept either.
func TestServer_TrustedHeaderRoutes(t *testing.T) {
	s := newTestServer(newStubRouter([]string{"m1"}, "ok"), newStubRouter(nil, ""))
	cfg := trustedHeaderConfig()
	cfg.Models = map[string]config.ModelConfig{"m1": {}}
	s.cfg = cfg
	s.routes()

	do := func(method, target, key, user string) int {
		body := ""
		if method == http.MethodPost {
			body = `{"model":"m1"}`
		}
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		if user != "" {
			r.Header.Set("Remote-User", user)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Code
	}

	cases := []struct {
		method, target, key, user string
		want                      int
	}{
		// UI routes: header only.
		{http.MethodGet, "/api/version", "", "alice", http.StatusOK},
		{http.MethodGet, "/api/version", "secret", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/version", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/profiles", "", "alice", http.StatusOK},
		{http.MethodGet, "/api/profiles", "secret", "", http.StatusUnauthorized},
		{http.MethodGet, "/logs", "", "alice", http.StatusOK},
		{http.MethodGet, "/logs", "secret", "", http.StatusUnauthorized},
		{http.MethodGet, "/ui/", "secret", "", http.StatusUnauthorized},

		// Everything else: key or header.
		{http.MethodPost, "/v1/chat/completions", "secret", "", http.StatusOK},
		{http.MethodPost, "/v1/chat/completions", "", "alice", http.StatusOK},
		{http.MethodPost, "/v1/chat/completions", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/v1/models", "secret", "", http.StatusOK},
		{http.MethodGet, "/v1/models", "", "alice", http.StatusOK},
		{http.MethodGet, "/v1/models", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/upstream/m1/health", "secret", "", http.StatusOK},
		{http.MethodGet, "/upstream/m1/health", "", "alice", http.StatusOK},
		{http.MethodGet, "/running", "secret", "", http.StatusOK},
		{http.MethodGet, "/running", "", "alice", http.StatusOK},
		{http.MethodGet, "/metrics", "", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		if got := do(c.method, c.target, c.key, c.user); got != c.want {
			t.Errorf("%s %s key=%q user=%q status = %d, want %d", c.method, c.target, c.key, c.user, got, c.want)
		}
	}
}
