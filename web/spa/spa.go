// Package spa serves a single-page application from an fs.FS.
//
// Two things make this more than a call to http.FileServer:
//
//   - Deep links. /clients is a client-side route, not a file. Any request that
//     does not match a real asset has to fall back to index.html so the SPA can
//     route it, while genuinely missing assets must still 404 rather than being
//     answered with HTML.
//   - Sub-path mounting. When web_base_url is set, the built index.html has to
//     have its asset references and its router base rewritten to match, because
//     the build is produced once and mounted wherever the operator configures.
package spa

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// Handler serves an embedded SPA build.
type Handler struct {
	fsys     fs.FS
	basePath string // normalised: "" or "/nps"

	// index is the rewritten index.html, built once on first use.
	indexOnce sync.Once
	indexBody []byte
	indexErr  error

	// modTime stamps every response. The embedded FS reports a zero time, which
	// would disable conditional requests entirely, so the process start time is
	// used instead: it changes exactly when the binary is replaced.
	modTime time.Time
}

// New returns a handler serving fsys under basePath. basePath must already be
// normalised (see web/basepath): "" or a path like "/nps".
func New(fsys fs.FS, basePath string, modTime time.Time) *Handler {
	return &Handler{fsys: fsys, basePath: basePath, modTime: modTime}
}

// Available reports whether the build actually contains an index.html. A binary
// compiled without running the frontend build still embeds a placeholder
// directory, and serving its emptiness as a blank page would be a confusing way
// to discover that.
func (h *Handler) Available() bool {
	f, err := h.fsys.Open("index.html")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		h.serveIndex(w, r)
		return
	}

	f, err := h.fsys.Open(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			h.serveFallback(w, r, name)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if st.IsDir() {
		h.serveFallback(w, r, name)
		return
	}
	// index.html always goes through the rewriting path, even when requested by
	// name, so that a bookmarked /index.html gets the same base-path fixes.
	if name == "index.html" {
		h.serveIndex(w, r)
		return
	}

	seeker, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.setCacheHeaders(w, name)
	http.ServeContent(w, r, st.Name(), h.modTime, seeker)
}

// serveFallback answers a request that matched no file. Client-side routes get
// index.html; anything that looks like an asset gets an honest 404, so a broken
// script tag surfaces as a failed request instead of a parse error on HTML.
func (h *Handler) serveFallback(w http.ResponseWriter, r *http.Request, name string) {
	if isAssetRequest(name, r) {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

// isAssetRequest reports whether a miss should 404 rather than fall back. A
// request under the build's asset directory, or one carrying a file extension,
// or one whose Accept header does not admit HTML, is an asset.
func isAssetRequest(name string, r *http.Request) bool {
	if strings.HasPrefix(name, "assets/") {
		return true
	}
	if path.Ext(name) != "" {
		return true
	}
	if accept := r.Header.Get("Accept"); accept != "" &&
		!strings.Contains(accept, "text/html") && !strings.Contains(accept, "*/*") {
		return true
	}
	return false
}

// setCacheHeaders lets hashed build assets be cached hard while anything else
// is revalidated. Vite emits assets/<name>-<hash>.<ext>, so the content of a
// given URL under assets/ never changes.
func (h *Handler) setCacheHeaders(w http.ResponseWriter, name string) {
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	h.indexOnce.Do(func() {
		raw, err := fs.ReadFile(h.fsys, "index.html")
		if err != nil {
			h.indexErr = err
			return
		}
		h.indexBody = rewriteBase(raw, h.basePath)
	})
	if h.indexErr != nil {
		http.Error(w, "web UI is not built into this binary", http.StatusNotFound)
		return
	}
	// index.html must never be cached: it names the hashed asset bundles, so a
	// stale copy would keep pointing at assets that an upgrade removed.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", h.modTime, bytes.NewReader(h.indexBody))
}

// rewriteBase adapts a build produced for the root to the configured sub-path.
// The build is emitted with absolute "/assets/..." references and a <base>
// element; pointing both at basePath is enough for the app and its router to
// find themselves.
func rewriteBase(raw []byte, basePath string) []byte {
	if basePath == "" {
		return raw
	}
	out := bytes.ReplaceAll(raw, []byte(`<base href="/">`), []byte(`<base href="`+basePath+`/">`))
	// Anchor on the quote so that only URL-valued attributes are touched and an
	// unrelated occurrence of the text /assets/ in a script cannot be rewritten.
	for _, q := range []string{`"`, `'`} {
		out = bytes.ReplaceAll(out,
			[]byte(q+"/assets/"),
			[]byte(q+basePath+"/assets/"))
	}
	return out
}
