package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/djylb/nps/lib/appconfig"
)

// loadConfig installs a temporary nps.conf so the accessors in config.go have
// something to read. Tests that care about a specific key set it here.
func loadConfig(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nps.conf")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := appconfig.LoadAppConfig("ini", path); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope from %q: %v", rec.Body.String(), err)
	}
	return env
}

func TestHealthEndpoint(t *testing.T) {
	loadConfig(t, "appname=nps\n")
	rt := NewRouter(time.Now())

	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	env := decodeEnvelope(t, rec)
	if env.Code != CodeOK {
		t.Errorf("code = %d, want %d", env.Code, CodeOK)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("no X-Request-Id header")
	}
	if env.RequestID == "" {
		t.Error("envelope carries no requestId")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestUnknownAPIPathReturnsJSON404(t *testing.T) {
	loadConfig(t, "appname=nps\n")
	rt := NewRouter(time.Now())

	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	// The SPA fallback must not swallow API paths, or a client would receive
	// HTML where it expects JSON and fail with an opaque parse error.
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	if env := decodeEnvelope(t, rec); env.Code != CodeNotFound {
		t.Errorf("code = %d, want %d", env.Code, CodeNotFound)
	}
}

func TestBasePathMounting(t *testing.T) {
	loadConfig(t, "appname=nps\nweb_base_url=/nps\n")
	rt := NewRouter(time.Now())

	if rt.BasePath() != "/nps" {
		t.Fatalf("BasePath() = %q, want %q", rt.BasePath(), "/nps")
	}

	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nps/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /nps/api/v1/health: status = %d, want 200", rec.Code)
	}

	// Without the prefix the route must not exist at all.
	rec = httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/health with a base path: status = %d, want 404", rec.Code)
	}

	// A textual near-match must not be routed into the app.
	rec = httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/npsx/api/v1/health", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /npsx/...: status = %d, want 404", rec.Code)
	}
}

func TestBasePathRootRedirects(t *testing.T) {
	loadConfig(t, "appname=nps\nweb_base_url=/nps\n")
	rt := NewRouter(time.Now())

	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nps", nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/nps/" {
		t.Errorf("Location = %q, want %q", loc, "/nps/")
	}
}

func TestPanicIsRecovered(t *testing.T) {
	loadConfig(t, "appname=nps\n")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		panic("handler exploded")
	})
	h := Chain(mux, WithRequestID, WithRecover)

	rec := httptest.NewRecorder()
	// A panic must become a 500 rather than unwinding into the server, which
	// shares this process with the tunnel data plane.
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if env := decodeEnvelope(t, rec); env.Code != CodeInternal {
		t.Errorf("code = %d, want %d", env.Code, CodeInternal)
	}
}
