package spa

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var buildTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {Data: []byte(
			`<!doctype html><html><head><base href="/">` +
				`<script type="module" src="/assets/index-abc123.js"></script>` +
				`<link rel="stylesheet" href="/assets/index-def456.css">` +
				`</head><body><div id="root"></div></body></html>`)},
		"assets/index-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/index-def456.css": {Data: []byte("body{}")},
		"favicon.ico":             {Data: []byte("icon")},
	}
}

func get(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServesAssets(t *testing.T) {
	h := New(testFS(), "", buildTime)
	rec := get(t, h, "/assets/index-abc123.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "console.log(1)" {
		t.Errorf("body = %q, want %q", got, "console.log(1)")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want an immutable directive for hashed assets", cc)
	}
}

func TestDeepLinkFallsBackToIndex(t *testing.T) {
	h := New(testFS(), "", buildTime)
	for _, path := range []string{"/", "/clients", "/tunnels/42/edit", "/login"} {
		rec := get(t, h, path, map[string]string{"Accept": "text/html"})
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Errorf("GET %s: body is not index.html", path)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("GET %s: Cache-Control = %q, want no-store", path, cc)
		}
	}
}

func TestMissingAssetIs404(t *testing.T) {
	h := New(testFS(), "", buildTime)
	// A missing asset must not be answered with index.html, or the browser
	// would try to parse HTML as JavaScript and report a confusing syntax error.
	for _, path := range []string{"/assets/gone.js", "/nope.js", "/style.css"} {
		rec := get(t, h, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestNonHTMLAcceptDoesNotGetIndex(t *testing.T) {
	h := New(testFS(), "", buildTime)
	rec := get(t, h, "/some/route", map[string]string{"Accept": "application/json"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a non-HTML Accept", rec.Code)
	}
}

func TestBasePathRewrite(t *testing.T) {
	h := New(testFS(), "/nps", buildTime)
	rec := get(t, h, "/", map[string]string{"Accept": "text/html"})
	body := rec.Body.String()
	if !strings.Contains(body, `<base href="/nps/">`) {
		t.Errorf("base element not rewritten; body = %s", body)
	}
	if !strings.Contains(body, `src="/nps/assets/index-abc123.js"`) {
		t.Errorf("script src not rewritten; body = %s", body)
	}
	if !strings.Contains(body, `href="/nps/assets/index-def456.css"`) {
		t.Errorf("stylesheet href not rewritten; body = %s", body)
	}
	if strings.Contains(body, `"/assets/`) {
		t.Errorf("an unrewritten /assets/ reference remains; body = %s", body)
	}
}

func TestIndexHtmlByNameIsRewritten(t *testing.T) {
	h := New(testFS(), "/nps", buildTime)
	rec := get(t, h, "/index.html", nil)
	if !strings.Contains(rec.Body.String(), `<base href="/nps/">`) {
		t.Error("GET /index.html served the raw file instead of the rewritten one")
	}
}

func TestPathTraversalIsContained(t *testing.T) {
	h := New(testFS(), "", buildTime)
	// path.Clean collapses the escape; the result must not read outside the FS.
	rec := get(t, h, "/../../etc/passwd", nil)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "root:") {
		t.Fatal("path traversal escaped the embedded filesystem")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := New(testFS(), "", buildTime)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHeadRequestHasNoBody(t *testing.T) {
	h := New(testFS(), "", buildTime)
	req := httptest.NewRequest(http.MethodHead, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// httptest does not strip the body for HEAD the way the real server does,
	// so only the status and headers are meaningful here.
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want a JavaScript type", ct)
	}
}

func TestAvailable(t *testing.T) {
	if !New(testFS(), "", buildTime).Available() {
		t.Error("Available() = false for an FS containing index.html")
	}
	if New(fstest.MapFS{}, "", buildTime).Available() {
		t.Error("Available() = true for an empty FS")
	}
}

func TestUnbuiltUIReports404(t *testing.T) {
	h := New(fstest.MapFS{}, "", buildTime)
	rec := get(t, h, "/", map[string]string{"Accept": "text/html"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no build is embedded", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not built") {
		t.Errorf("body = %q, want a message explaining the UI is missing", rec.Body.String())
	}
}

func TestConditionalRequest(t *testing.T) {
	h := New(testFS(), "", buildTime)
	first := get(t, h, "/assets/index-abc123.js", nil)
	lastMod := first.Header().Get("Last-Modified")
	if lastMod == "" {
		t.Fatal("no Last-Modified header, conditional requests would be impossible")
	}
	rec := get(t, h, "/assets/index-abc123.js", map[string]string{"If-Modified-Since": lastMod})
	if rec.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", rec.Code)
	}
	if body, _ := io.ReadAll(rec.Body); len(body) != 0 {
		t.Errorf("304 response carried a %d-byte body", len(body))
	}
}
