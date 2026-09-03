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
	raw := "apiKeys: [\"secret\"]\n" + authYAML
	cfg, err := config.LoadConfigFromReader(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	return cfg
}

const trustedHeaderYAML = "auth:\n  ui: trustedHeader\n  trustedHeader: Remote-User\n"
const noneYAML = "auth:\n  ui: none\n"

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

	t.Run("default mode: apiKeys guard the UI", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(authConfig(t, ""))
		if w := get(mw, "secret", "", ""); w.Code != http.StatusOK {
			t.Errorf("key status = %d, want 200", w.Code)
		}
		if w := get(mw, "", "alice", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("header in apiKeys mode status = %d, want 401", w.Code)
		}
	})

	t.Run("trustedHeader mode: UI accepts only the header", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(authConfig(t, trustedHeaderYAML))
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

	t.Run("none mode: UI is open", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(authConfig(t, noneYAML))
		if w := get(mw, "", "", ""); w.Code != http.StatusOK {
			t.Errorf("nothing status = %d, want 200", w.Code)
		}
	})

	t.Run("inference accepts only API keys in every mode", func(t *testing.T) {
		for _, yml := range []string{"", trustedHeaderYAML, noneYAML} {
			mw := CreateAuthMiddleware(authConfig(t, yml))
			if w := get(mw, "secret", "", ""); w.Code != http.StatusOK {
				t.Errorf("%q key status = %d, want 200", yml, w.Code)
			}
			if w := get(mw, "", "alice", ""); w.Code != http.StatusUnauthorized {
				t.Errorf("%q header status = %d, want 401", yml, w.Code)
			}
			if w := get(mw, "", "", ""); w.Code != http.StatusUnauthorized {
				t.Errorf("%q nothing status = %d, want 401", yml, w.Code)
			}
		}
	})

	t.Run("trustedProxies limits who may present the header", func(t *testing.T) {
		mw := CreateUIAuthMiddleware(authConfig(t, trustedHeaderYAML+"  trustedProxies: [\"10.0.0.0/8\", \"192.168.1.1\"]\n"))
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

// routeCase drives one request through the router with an optional API key
// and trusted header value.
type routeCase struct {
	method, target, key, user string
	want                      int
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
		if c.user != "" {
			r.Header.Set("Remote-User", c.user)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s key=%q user=%q status = %d, want %d (body=%q)", c.method, c.target, c.key, c.user, w.Code, c.want, w.Body.String())
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

// TestServer_Routes_TrustedHeaderMode: UI routes and the /api mirrors of the
// model endpoints accept only the header; the public model endpoints and
// operations endpoints accept only API keys.
func TestServer_Routes_TrustedHeaderMode(t *testing.T) {
	runRouteCases(t, authServer(t, trustedHeaderYAML), []routeCase{
		// UI routes: header only.
		{http.MethodGet, "/api/version", "", "alice", http.StatusOK},
		{http.MethodGet, "/api/version", "secret", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/version", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/logs", "", "alice", http.StatusOK},
		{http.MethodGet, "/logs", "secret", "", http.StatusUnauthorized},
		{http.MethodGet, "/ui/", "secret", "", http.StatusUnauthorized},

		// UI mirrors of the model endpoints: header only.
		{http.MethodPost, "/api/v1/chat/completions", "", "alice", http.StatusOK},
		{http.MethodPost, "/api/v1/chat/completions", "secret", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/chat/completions", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/models", "", "alice", http.StatusOK},
		{http.MethodGet, "/api/v1/models", "secret", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/upstream/m1/health", "", "alice", http.StatusOK},
		{http.MethodGet, "/api/upstream/m1/health", "secret", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/nope", "", "alice", http.StatusNotFound},

		// Public model endpoints and operations: API key only.
		{http.MethodPost, "/v1/chat/completions", "secret", "", http.StatusOK},
		{http.MethodPost, "/v1/chat/completions", "", "alice", http.StatusUnauthorized},
		{http.MethodPost, "/v1/chat/completions", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/v1/models", "secret", "", http.StatusOK},
		{http.MethodGet, "/v1/models", "", "alice", http.StatusUnauthorized},
		{http.MethodGet, "/upstream/m1/health", "secret", "", http.StatusOK},
		{http.MethodGet, "/upstream/m1/health", "", "alice", http.StatusUnauthorized},
		{http.MethodGet, "/running", "secret", "", http.StatusOK},
		{http.MethodGet, "/running", "", "alice", http.StatusUnauthorized},
		{http.MethodGet, "/metrics", "", "alice", http.StatusUnauthorized},
	})
}

// TestServer_Routes_NoneMode: UI routes and mirrors are open; everything
// else still requires an API key.
func TestServer_Routes_NoneMode(t *testing.T) {
	runRouteCases(t, authServer(t, noneYAML), []routeCase{
		{http.MethodGet, "/api/version", "", "", http.StatusOK},
		{http.MethodGet, "/logs", "", "", http.StatusOK},
		{http.MethodPost, "/api/v1/chat/completions", "", "", http.StatusOK},
		{http.MethodGet, "/api/v1/models", "", "", http.StatusOK},
		{http.MethodGet, "/api/upstream/m1/health", "", "", http.StatusOK},

		{http.MethodPost, "/v1/chat/completions", "", "", http.StatusUnauthorized},
		{http.MethodPost, "/v1/chat/completions", "secret", "", http.StatusOK},
		{http.MethodGet, "/v1/models", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/upstream/m1/health", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/running", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/metrics", "", "", http.StatusUnauthorized},
	})
}

// TestServer_Routes_DefaultMode: without auth.ui, apiKeys guard everything
// including the mirrors, exactly as before.
func TestServer_Routes_DefaultMode(t *testing.T) {
	runRouteCases(t, authServer(t, ""), []routeCase{
		{http.MethodGet, "/api/version", "secret", "", http.StatusOK},
		{http.MethodGet, "/api/version", "", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/chat/completions", "secret", "", http.StatusOK},
		{http.MethodPost, "/api/v1/chat/completions", "", "", http.StatusUnauthorized},
		{http.MethodPost, "/v1/chat/completions", "secret", "", http.StatusOK},
		{http.MethodPost, "/v1/chat/completions", "", "", http.StatusUnauthorized},
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
