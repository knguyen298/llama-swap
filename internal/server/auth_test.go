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

func authConfig(t *testing.T, authYAML string) config.Config {
	t.Helper()
	cfg, err := config.LoadConfigFromReader(strings.NewReader("apiKeys: [\"secret\"]\n" + authYAML))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	return cfg
}

const noneYAML = "auth:\n  ui: none\n"

func TestServer_UIAuthMiddleware(t *testing.T) {
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	get := func(mw func(http.Handler) http.Handler, key string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		mw(final).ServeHTTP(w, r)
		return w.Code
	}

	t.Run("default mode: apiKeys guard the UI", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(authConfig(t, ""))
		if got := get(mw, "secret"); got != http.StatusOK {
			t.Errorf("key status = %d, want 200", got)
		}
		if got := get(mw, ""); got != http.StatusUnauthorized {
			t.Errorf("no key status = %d, want 401", got)
		}
	})

	t.Run("none mode: UI is open", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(authConfig(t, noneYAML))
		if got := get(mw, ""); got != http.StatusOK {
			t.Errorf("no key status = %d, want 200", got)
		}
	})

	t.Run("inference always requires an API key", func(t *testing.T) {
		for _, yml := range []string{"", noneYAML} {
			mw := CreateAuthMiddleware(authConfig(t, yml))
			if got := get(mw, "secret"); got != http.StatusOK {
				t.Errorf("%q key status = %d, want 200", yml, got)
			}
			if got := get(mw, ""); got != http.StatusUnauthorized {
				t.Errorf("%q no key status = %d, want 401", yml, got)
			}
			if got := get(mw, "wrong"); got != http.StatusUnauthorized {
				t.Errorf("%q wrong key status = %d, want 401", yml, got)
			}
		}
	})
}

// routeCase drives one request through the router with an optional API key.
type routeCase struct {
	method, target, key string
	want                int
}

func runRouteCases(t *testing.T, s *Server, cases []routeCase) {
	t.Helper()
	for _, c := range cases {
		body := ""
		if c.method == http.MethodPost {
			body = `{"model":"m1"}`
		}
		r := httptest.NewRequest(c.method, c.target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if c.key != "" {
			r.Header.Set("Authorization", "Bearer "+c.key)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s key=%q status = %d, want %d (body=%q)", c.method, c.target, c.key, w.Code, c.want, w.Body.String())
		}
	}
}

func authServer(t *testing.T, authYAML string) *Server {
	t.Helper()
	s := newTestServer(newStubRouter([]string{"m1"}, "ok"), newStubRouter(nil, ""))
	cfg := authConfig(t, authYAML)
	cfg.Models = map[string]config.ModelConfig{"m1": {}}
	s.cfg = cfg
	s.routes()
	return s
}

// TestServer_Routes_NoneMode: UI routes and the /api mirrors of the model
// endpoints are open; the public model endpoints and the operations
// endpoints still require an API key.
func TestServer_Routes_NoneMode(t *testing.T) {
	runRouteCases(t, authServer(t, noneYAML), []routeCase{
		// Open: the UI and what only it uses.
		{http.MethodGet, "/api/version", "", http.StatusOK},
		{http.MethodGet, "/api/profiles", "", http.StatusOK},
		{http.MethodGet, "/logs", "", http.StatusOK},
		// Auth passes; 404 because the test binary embeds no UI assets.
		{http.MethodGet, "/ui/", "", http.StatusNotFound},

		// Open: the /api mirrors, so the Playground works without a key.
		{http.MethodPost, "/api/v1/chat/completions", "", http.StatusOK},
		{http.MethodGet, "/api/v1/models", "", http.StatusOK},
		{http.MethodGet, "/api/upstream/m1/health", "", http.StatusOK},
		{http.MethodPost, "/api/v1/nope", "", http.StatusNotFound},

		// Still key-only: the public model and operations endpoints.
		{http.MethodPost, "/v1/chat/completions", "", http.StatusUnauthorized},
		{http.MethodPost, "/v1/chat/completions", "secret", http.StatusOK},
		{http.MethodGet, "/v1/models", "", http.StatusUnauthorized},
		{http.MethodGet, "/models", "", http.StatusUnauthorized},
		{http.MethodGet, "/upstream/m1/health", "", http.StatusUnauthorized},
		{http.MethodGet, "/upstream/m1/health", "secret", http.StatusOK},
		{http.MethodGet, "/running", "", http.StatusUnauthorized},
		{http.MethodGet, "/running", "secret", http.StatusOK},
		{http.MethodGet, "/metrics", "", http.StatusUnauthorized},
	})
}

// TestServer_Routes_DefaultMode: without auth.ui, apiKeys guard everything
// including the mirrors, exactly as before.
func TestServer_Routes_DefaultMode(t *testing.T) {
	runRouteCases(t, authServer(t, ""), []routeCase{
		{http.MethodGet, "/api/version", "secret", http.StatusOK},
		{http.MethodGet, "/api/version", "", http.StatusUnauthorized},
		{http.MethodGet, "/ui/", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/chat/completions", "secret", http.StatusOK},
		{http.MethodPost, "/api/v1/chat/completions", "", http.StatusUnauthorized},
		{http.MethodPost, "/v1/chat/completions", "secret", http.StatusOK},
		{http.MethodPost, "/v1/chat/completions", "", http.StatusUnauthorized},
	})
}

// TestServer_UIModelMirror_PathHandling checks the mirror strips its prefix
// while preserving escaping, and that redirects stay under the prefix.
func TestServer_UIModelMirror_PathHandling(t *testing.T) {
	local := newStubRouter([]string{"author/model", "m1"}, "")
	var gotPath string
	local.serveHTTP = func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
	}
	s := newTestServer(local, newStubRouter(nil, ""))
	s.cfg = config.Config{Models: map[string]config.ModelConfig{"author/model": {}, "m1": {}}}
	s.routes()

	do := func(method, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, target, nil))
		return w
	}

	if w := do(http.MethodPost, "/api/upstream/author%2Fmodel/api/x%2Fy"); w.Code != http.StatusOK {
		t.Fatalf("mirrored upstream status = %d body=%q", w.Code, w.Body.String())
	} else if gotPath != "/api/x%2Fy" {
		t.Errorf("mirrored upstream forwarded path = %q, want /api/x%%2Fy", gotPath)
	}

	if w := do(http.MethodGet, "/api/upstream/m1?x=1"); w.Code != http.StatusMovedPermanently {
		t.Errorf("mirrored upstream redirect status = %d, want 301", w.Code)
	} else if loc := w.Header().Get("Location"); loc != "/api/upstream/m1/?x=1" {
		t.Errorf("mirrored upstream redirect Location = %q, want /api/upstream/m1/?x=1", loc)
	}
	if w := do(http.MethodGet, "/upstream/m1"); w.Header().Get("Location") != "/upstream/m1/" {
		t.Errorf("public upstream redirect Location = %q, want /upstream/m1/", w.Header().Get("Location"))
	}

	if w := do(http.MethodGet, "/api/comfyui?t=1"); w.Header().Get("Location") != "/api/comfyui/?t=1" {
		t.Errorf("mirrored comfyui redirect Location = %q, want /api/comfyui/?t=1", w.Header().Get("Location"))
	}
	if w := do(http.MethodGet, "/comfyui?t=1"); w.Header().Get("Location") != "/comfyui/?t=1" {
		t.Errorf("public comfyui redirect Location = %q, want /comfyui/?t=1", w.Header().Get("Location"))
	}
}
