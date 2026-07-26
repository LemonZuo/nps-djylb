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

// registerRoutes wires the JSON endpoints.
//
// Routes are grouped by the credential they need. Anonymous routes are listed
// explicitly and everything else goes through RequireAuth, so adding an
// endpoint without thinking about authentication fails closed.
func registerRoutes(mux *http.ServeMux) {
	// --- anonymous ---

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		Ok(w, r, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /auth/challenge", handleChallenge)
	mux.HandleFunc("GET /auth/captcha", handleCaptcha)
	mux.HandleFunc("POST /auth/login", handleLogin)
	mux.HandleFunc("POST /auth/register", handleRegister)

	// --- authenticated ---

	auth := func(h http.HandlerFunc) http.Handler {
		return RequireAuth(h)
	}
	mux.Handle("GET /auth/me", auth(handleMe))
	mux.Handle("POST /auth/logout", auth(handleLogout))

	mux.Handle("GET /meta/bootstrap", auth(handleBootstrap))
	mux.Handle("GET /dashboard", auth(handleDashboard))
	mux.Handle("GET /dashboard/history", auth(handleDashboardHistory))

	// Per-record client routes go through requireClientAccess, which scopes a
	// user to their own record; the collection is scoped by resolveClientScope.
	mux.Handle("GET /clients", auth(handleListClients))
	mux.Handle("GET /clients/{id}", auth(handleGetClient))
	mux.Handle("PUT /clients/{id}", auth(handleUpdateClient))
	mux.Handle("POST /clients/{id}/ping", auth(handlePingClient))
	mux.Handle("GET /clients/{id}/qrcode", auth(handleClientQRCode))

	mux.Handle("GET /tunnels", auth(handleListTunnels))
	mux.Handle("POST /tunnels", auth(handleCreateTunnel))
	mux.Handle("GET /tunnels/{id}", auth(handleGetTunnel))
	mux.Handle("PUT /tunnels/{id}", auth(handleUpdateTunnel))
	mux.Handle("DELETE /tunnels/{id}", auth(handleDeleteTunnel))
	mux.Handle("POST /tunnels/{id}/start", auth(handleStartTunnel))
	mux.Handle("POST /tunnels/{id}/stop", auth(handleStopTunnel))
	mux.Handle("POST /tunnels/{id}/toggle", auth(handleToggleTunnel))

	mux.Handle("GET /hosts", auth(handleListHosts))
	mux.Handle("POST /hosts", auth(handleCreateHost))
	mux.Handle("GET /hosts/{id}", auth(handleGetHost))
	mux.Handle("PUT /hosts/{id}", auth(handleUpdateHost))
	mux.Handle("DELETE /hosts/{id}", auth(handleDeleteHost))
	mux.Handle("POST /hosts/{id}/start", auth(handleStartHost))
	mux.Handle("POST /hosts/{id}/stop", auth(handleStopHost))
	mux.Handle("POST /hosts/{id}/toggle", auth(handleToggleHost))

	// --- administrator only ---

	admin := func(h http.HandlerFunc) http.Handler {
		return RequireAuth(RequireAdmin(h))
	}
	mux.Handle("GET /auth/bans", admin(handleListBans))
	mux.Handle("DELETE /auth/bans", admin(handleClearBans))
	mux.Handle("POST /auth/bans/clean", admin(handleCleanBans))
	mux.Handle("DELETE /auth/bans/{key}", admin(handleRemoveBan))

	// Creating a client issues a vkey; deleting, disabling and quota resets
	// are operator levers. Users manage their own record via PUT above.
	mux.Handle("POST /clients", admin(handleCreateClient))
	mux.Handle("DELETE /clients/{id}", admin(handleDeleteClient))
	mux.Handle("POST /clients/{id}/status", admin(handleClientStatus))
	mux.Handle("POST /clients/{id}/clear", admin(handleClearClient))
	mux.Handle("POST /clients/clear", admin(handleClearClient))

	mux.Handle("POST /tunnels/{id}/clear", admin(handleClearTunnel))
	mux.Handle("POST /hosts/{id}/clear", admin(handleClearHost))

	mux.Handle("GET /global", admin(handleGetGlobal))
	mux.Handle("PUT /global", admin(handleUpdateGlobal))

	// Anything under the API prefix that matched no route is a client error in
	// JSON terms, so it must not fall through to the SPA's HTML.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		NotFound(w, r, "no such endpoint")
	})
}

// handleListBans reports the current login throttling state.
func handleListBans(w http.ResponseWriter, r *http.Request) {
	list := ListLoginBans()
	OkList(w, r, list, int64(len(list)))
}

// handleRemoveBan lifts the block on one address or username.
func handleRemoveBan(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		BadRequest(w, r, "key is required")
		return
	}
	if !RemoveLoginBan(key) {
		NotFound(w, r, "no such ban record")
		return
	}
	Ok(w, r, nil)
}

// handleClearBans lifts every block.
func handleClearBans(w http.ResponseWriter, r *http.Request) {
	RemoveAllLoginBans()
	Ok(w, r, nil)
}

// handleCleanBans drops expired records immediately instead of waiting for
// the minute-interval background sweep — the old UI's refresh button did
// this via /global/banclean before re-reading the table.
func handleCleanBans(w http.ResponseWriter, r *http.Request) {
	CleanBanRecords(true)
	Ok(w, r, nil)
}
