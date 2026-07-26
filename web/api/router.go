package api

import (
	"net/http"
	"time"

	"github.com/djylb/nps/web"
	"github.com/djylb/nps/web/basepath"
	"github.com/djylb/nps/web/spa"
)

// APIPrefix is the route prefix every JSON endpoint lives under, before the
// configured web_base_url is applied.
const APIPrefix = "/api/v1"

// Router builds the complete HTTP handler for the management interface: the
// JSON API under APIPrefix, and the embedded SPA on everything else.
type Router struct {
	basePath string
	handler  http.Handler
}

// NewRouter constructs the handler tree. startTime stamps embedded assets so
// conditional requests work; it should be the process start time.
func NewRouter(startTime time.Time) *Router {
	base := basepath.Normalize(WebBaseURL())

	apiMux := http.NewServeMux()
	registerRoutes(apiMux)

	// The API is mounted at the root of its own mux, so handlers can declare
	// plain patterns like "GET /clients" and stay independent of both APIPrefix
	// and web_base_url.
	api := Chain(http.StripPrefix(APIPrefix, apiMux),
		WithRequestID,
		WithRecover,
		WithAccessLog,
		WithNoStore,
	)

	ui := spa.New(web.DistFS(), base, startTime)

	root := http.NewServeMux()
	root.Handle(APIPrefix+"/", api)
	root.Handle("/", ui)

	var handler http.Handler = root
	if base != "" {
		// Requests arrive with the base prefix; strip it once here so nothing
		// downstream has to know the server is mounted under a sub-path.
		handler = mountUnder(base, root)
	}

	return &Router{basePath: base, handler: handler}
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.handler.ServeHTTP(w, r)
}

// BasePath returns the normalised mount path ("" when mounted at the root).
func (rt *Router) BasePath() string { return rt.basePath }

// mountUnder serves next only for requests inside base. A request for base
// without its trailing slash is redirected so that relative URLs in the SPA
// resolve against the right directory.
func mountUnder(base string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == base {
			http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
			return
		}
		rest, ok := basepath.Strip(base, r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		// Clone rather than mutate: the original request is shared with the
		// server's own bookkeeping.
		r2 := r.Clone(r.Context())
		r2.URL.Path = rest
		next.ServeHTTP(w, r2)
	})
}

// registerRoutes wires the JSON endpoints. Later milestones fill this in; the
// health endpoint exists from the start so the transport can be verified
// independently of any business logic.
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		Ok(w, r, map[string]string{"status": "ok"})
	})

	// Anything under the API prefix that matched no route is a client error in
	// JSON terms, so it must not fall through to the SPA's HTML.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		NotFound(w, r, "no such endpoint")
	})
}
