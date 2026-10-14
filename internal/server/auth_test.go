package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mostlygeek/llama-swap/internal/config"
)

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

// TestServer_Routes_NoneMode: the UI door is open, the public door still
// requires an API key.
func TestServer_Routes_NoneMode(t *testing.T) {
	runRouteCases(t, authServer(t, noneYAML), []routeCase{
		// UI door, no key: every handler is reachable.
		{http.MethodGet, "/ui/api/version", "", http.StatusOK},
		{http.MethodGet, "/ui/api/profiles", "", http.StatusOK},
		{http.MethodPost, "/ui/v1/chat/completions", "", http.StatusOK},
		{http.MethodGet, "/ui/v1/models", "", http.StatusOK},
		{http.MethodGet, "/ui/upstream/m1/health", "", http.StatusOK},
		// audio.cpp passthrough: the UI door reaches the handler, which answers
		// 404 because the reserved audiocpp_auto model is not in this config.
		{http.MethodGet, "/ui/audiocpp/health", "", http.StatusNotFound},
		{http.MethodGet, "/ui/running", "", http.StatusOK},
		// The check passes; 404 because the test binary embeds no UI assets.
		// /ui/logs is one of these: GET /logs lives outside the door, so the
		// request falls through to the web UI's static files.
		{http.MethodGet, "/ui/", "", http.StatusNotFound},
		{http.MethodGet, "/ui/logs", "", http.StatusNotFound},
		{http.MethodGet, "/ui/ui/api/version", "", http.StatusNotFound},

		// Public door: a key is required, as before.
		{http.MethodPost, "/v1/chat/completions", "", http.StatusUnauthorized},
		{http.MethodPost, "/v1/chat/completions", "secret", http.StatusOK},
		{http.MethodGet, "/v1/models", "", http.StatusUnauthorized},
		{http.MethodGet, "/models", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/version", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/version", "secret", http.StatusOK},
		{http.MethodGet, "/logs", "", http.StatusUnauthorized},
		{http.MethodGet, "/logs", "secret", http.StatusOK},
		{http.MethodGet, "/upstream/m1/health", "", http.StatusUnauthorized},
		{http.MethodGet, "/upstream/m1/health", "secret", http.StatusOK},
		{http.MethodGet, "/audiocpp/health", "", http.StatusUnauthorized},
		{http.MethodGet, "/audiocpp/health", "secret", http.StatusNotFound},
		{http.MethodGet, "/running", "", http.StatusUnauthorized},
		{http.MethodGet, "/metrics", "", http.StatusUnauthorized},

		// Unknown paths keep their 404, with or without a key.
		{http.MethodGet, "/nope", "", http.StatusNotFound},
		{http.MethodGet, "/health", "", http.StatusOK},
	})
}

// TestServer_Routes_DefaultMode: without auth.ui, both doors require an API
// key, exactly as before.
func TestServer_Routes_DefaultMode(t *testing.T) {
	runRouteCases(t, authServer(t, ""), []routeCase{
		{http.MethodGet, "/api/version", "secret", http.StatusOK},
		{http.MethodGet, "/api/version", "", http.StatusUnauthorized},
		{http.MethodGet, "/ui/", "", http.StatusUnauthorized},
		{http.MethodGet, "/ui/api/version", "secret", http.StatusOK},
		{http.MethodGet, "/ui/api/version", "", http.StatusUnauthorized},
		{http.MethodPost, "/ui/v1/chat/completions", "secret", http.StatusOK},
		{http.MethodPost, "/ui/v1/chat/completions", "", http.StatusUnauthorized},
		{http.MethodPost, "/v1/chat/completions", "secret", http.StatusOK},
		{http.MethodPost, "/v1/chat/completions", "", http.StatusUnauthorized},
		{http.MethodGet, "/nope", "", http.StatusNotFound},
	})
}

// TestServer_Doors_EveryRoute walks every model route through both doors.
// The public door must reject a missing key on all of them. The UI door in
// none mode must reach the handler on all of them.
func TestServer_Doors_EveryRoute(t *testing.T) {
	s := authServer(t, noneYAML)
	type route struct{ method, path string }
	var routes []route
	for _, p := range modelPostJSONRoutes {
		routes = append(routes, route{http.MethodPost, p})
	}
	for _, p := range modelPostFormRoutes {
		routes = append(routes, route{http.MethodPost, p})
	}
	for _, p := range modelGetRoutes {
		routes = append(routes, route{http.MethodGet, p + "?model=m1"})
	}
	// GET /logs is served outside the UI door (its HTML redirect is relative to
	// the public path), so it is not in this list.
	for _, p := range []string{"/v1/models", "/models", "/metrics", "/unload", "/running", "/api/version", "/api/profiles", "/api/hardware"} {
		routes = append(routes, route{http.MethodGet, p})
	}
	status := func(method, target, key string) int {
		r := httptest.NewRequest(method, target, strings.NewReader(`{"model":"m1"}`))
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Code
	}
	for _, rt := range routes {
		if got := status(rt.method, rt.path, ""); got != http.StatusUnauthorized {
			t.Errorf("public %s %s without key: status = %d, want 401", rt.method, rt.path, got)
		}
		// Any status except 401 and 404 means the handler was reached.
		if got := status(rt.method, uiPrefix+rt.path, ""); got == http.StatusUnauthorized || got == http.StatusNotFound {
			t.Errorf("ui door %s %s without key: status = %d, want the handler", rt.method, rt.path, got)
		}
	}
}

// TestServer_UIDoor_PathHandling checks the UI door keeps escaped paths
// intact and that redirects stay behind the same door.
func TestServer_UIDoor_PathHandling(t *testing.T) {
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

	doAccept := func(accept, target string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}

	if w := do(http.MethodPost, "/ui/upstream/author%2Fmodel/api/x%2Fy"); w.Code != http.StatusOK {
		t.Fatalf("ui door upstream status = %d body=%q", w.Code, w.Body.String())
	} else if gotPath != "/api/x%2Fy" {
		t.Errorf("ui door upstream forwarded path = %q, want /api/x%%2Fy", gotPath)
	}

	// Redirects use a relative Location (upstream's redirectRelative) so a
	// reverse proxy's stripped prefix survives. What matters for the doors is
	// where the browser lands, so assert the resolved target: a request that
	// arrives on the UI door must stay on it. crossesDoor marks the redirects
	// that deliberately leave the public door for the UI.
	for _, tc := range []struct {
		request, wantLocation, wantResolved string
		accept                              string
		crossesDoor                         bool
	}{
		{"/ui/upstream/m1?x=1", "./m1/?x=1", "/ui/upstream/m1/?x=1", "", false},
		{"/upstream/m1", "./m1/", "/upstream/m1/", "", false},
		{"/ui/comfyui?t=1", "comfyui/?t=1", "/ui/comfyui/?t=1", "", false},
		{"/comfyui?t=1", "comfyui/?t=1", "/comfyui/?t=1", "", false},
		{"/ui/audiocpp?t=1", "audiocpp/?t=1", "/ui/audiocpp/?t=1", "", false},
		{"/audiocpp?t=1", "audiocpp/?t=1", "/audiocpp/?t=1", "", false},
		{"/upstream", "ui/#/models", "/ui/", "", true},
		{"/logs", "ui/", "/ui/", "text/html", true},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.request, nil)
		if tc.accept != "" {
			r.Header.Set("Accept", tc.accept)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		loc := w.Header().Get("Location")
		if loc != tc.wantLocation {
			t.Errorf("%s Location = %q, want %q", tc.request, loc, tc.wantLocation)
		}
		resolved := resolveAgainst(t, tc.request, loc)
		if resolved != tc.wantResolved {
			t.Errorf("%s resolves to %q, want %q", tc.request, resolved, tc.wantResolved)
		}
		cameIn := strings.HasPrefix(tc.request, uiPrefix+"/")
		wentTo := strings.HasPrefix(resolved, uiPrefix+"/")
		if !tc.crossesDoor && wentTo != cameIn {
			t.Errorf("%s changed doors: behind door on request=%v, on target=%v", tc.request, cameIn, wentTo)
		}
	}

	// The regression this protects: an HTML client that opens /ui/logs directly
	// must not be sent to /ui/ui/, where the UI cannot recover its prefix. The
	// request carries no Location, so it falls through to the web UI's own files
	// (404 here: the test binary embeds none).
	if w := doAccept("text/html", "/ui/logs"); w.Header().Get("Location") != "" {
		t.Errorf("ui door /logs Location = %q, want no redirect", w.Header().Get("Location"))
	} else if w.Code != http.StatusNotFound {
		t.Errorf("ui door /logs status = %d, want 404 from the static handler", w.Code)
	}
}

// resolveAgainst reports the URL a browser would request next: the Location
// resolved against the URL it asked for. RequestURI keeps the query string.
func resolveAgainst(t *testing.T, request, location string) string {
	t.Helper()
	base, err := url.Parse("http://localhost" + request)
	if err != nil {
		t.Fatalf("parse %q: %v", request, err)
	}
	ref, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location %q: %v", location, err)
	}
	return base.ResolveReference(ref).RequestURI()
}
