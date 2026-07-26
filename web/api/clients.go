package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/crypt"
	"github.com/djylb/nps/lib/file"
	"github.com/djylb/nps/lib/rate"
	"github.com/djylb/nps/server"
)

// Client endpoints.
//
// The authorization rule for this collection has two halves, and both matter:
//
//   - which clients you can see or touch at all — enforced by
//     resolveClientScope on the list and by requireClientAccess on every
//     single-record route;
//   - which fields you may change — a user may edit their own remark and web
//     credentials, but the vkey, the quotas and the rate limits belong to the
//     operator. That split lives in applyClientAdminFields, called only for an
//     admin.
//
// Both were previously scattered through the controller as inline
// `GetSession("isAdmin").(bool)` checks; a missed one there was a privilege
// escalation.

// handleListClients returns a page of clients, scoped to the caller.
func handleListClients(w http.ResponseWriter, r *http.Request) {
	p := CurrentUser(r)
	q := parseListQuery(r)
	scope := resolveClientScope(p, q.ClientID)

	list, total := server.GetClientList(q.Offset, q.Limit, q.Search, q.Sort, q.Order, scope)
	rows := make([]ClientView, 0, len(list))
	for _, c := range list {
		rows = append(rows, NewClientView(c, p))
	}
	OkList(w, r, rows, int64(total))
}

// handleGetClient returns one client.
func handleGetClient(w http.ResponseWriter, r *http.Request) {
	c, ok := requireClientAccess(w, r)
	if !ok {
		return
	}
	Ok(w, r, NewClientView(c, CurrentUser(r)))
}

// ClientRequest is the create/update body. Every field is a pointer so that
// "absent" and "set to the zero value" are distinguishable: a PATCH that omits
// maxConn must not silently reset the operator's connection cap to unlimited.
type ClientRequest struct {
	Remark          *string `json:"remark"`
	VerifyKey       *string `json:"verifyKey"`
	RateLimit       *int    `json:"rateLimit"`
	MaxConn         *int    `json:"maxConn"`
	MaxTunnelNum    *int    `json:"maxTunnelNum"`
	FlowLimit       *int64  `json:"flowLimit"`
	TimeLimit       *string `json:"timeLimit"`
	ConfigConnAllow *bool   `json:"configConnAllow"`
	Compress        *bool   `json:"compress"`
	Crypt           *bool   `json:"crypt"`
	BasicUser       *string `json:"basicUser"`
	BasicPassword   *string `json:"basicPassword"`
	WebUserName     *string `json:"webUserName"`
	WebPassword     *string `json:"webPassword"`
	WebTotpSecret   *string `json:"webTotpSecret"`
	BlackIPList     *string `json:"blackIpList"`
	Status          *bool   `json:"status"`
	// FlowReset zeroes the traffic counters as part of the same update, which
	// is how the old edit form's "reset flow" checkbox behaved.
	FlowReset *bool `json:"flowReset"`
}

// handleCreateClient adds a client. Admin only: creating one hands out a vkey,
// which is a credential for the bridge.
func handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req ClientRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	// Id stays 0: file.DbUtils.NewClient assigns the next id, generates a vkey
	// when none was supplied, and starts the rate limiter.
	c := &file.Client{
		Status:     true,
		Cnf:        &file.Config{},
		Flow:       &file.Flow{},
		CreateTime: time.Now().Format("2006-01-02 15:04:05"),
	}
	applyClientCommonFields(c, &req)
	applyClientAdminFields(c, &req)
	if req.Status != nil {
		c.Status = *req.Status
	}

	// A web username that collides with the operator's own would let the
	// client's holder log in where the admin does.
	if c.WebUserName != "" && c.WebUserName == AdminUsername() {
		Conflict(w, r, "web login username duplicate, please reset")
		return
	}
	if err := file.GetDb().NewClient(c); err != nil {
		// NewClient's failures are all duplicate-key conditions the caller can
		// fix by choosing a different value.
		Conflict(w, r, err.Error())
		return
	}
	Ok(w, r, NewClientView(c, CurrentUser(r)))
}

// handleUpdateClient edits a client in place.
func handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	c, ok := requireClientAccess(w, r)
	if !ok {
		return
	}
	p := CurrentUser(r)

	var req ClientRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	// Renaming is gated separately from the rest: allow_user_change_username
	// exists precisely so an operator can pin the login names they handed out.
	if req.WebUserName != nil && *req.WebUserName != c.WebUserName {
		if !p.IsAdmin && !AllowUserChangeUsername() {
			Forbidden(w, r, "changing the login name is not allowed")
			return
		}
		name := strings.TrimSpace(*req.WebUserName)
		if name != "" && (name == AdminUsername() || !file.GetDb().VerifyUserName(name, c.Id)) {
			Conflict(w, r, "web login username duplicate, please reset")
			return
		}
	}

	if req.VerifyKey != nil && *req.VerifyKey != c.VerifyKey {
		if !p.IsAdmin {
			Forbidden(w, r, "changing the connection key requires administrator privileges")
			return
		}
		if !file.GetDb().VerifyVkey(*req.VerifyKey, c.Id) {
			Conflict(w, r, "vkey duplicate, please reset")
			return
		}
		// The vkey is indexed by hash for the bridge handshake, so the old
		// entry has to go before the new one is stored.
		file.Blake2bVkeyIndex.Remove(crypt.Blake2b(c.VerifyKey))
		c.VerifyKey = *req.VerifyKey
		file.Blake2bVkeyIndex.Add(crypt.Blake2b(c.VerifyKey), c.Id)
	}

	applyClientCommonFields(c, &req)
	if p.IsAdmin {
		applyClientAdminFields(c, &req)
		if req.Status != nil && *req.Status != c.Status {
			c.Status = *req.Status
			if !c.Status {
				// A disabled client must be disconnected now, not at its next
				// reconnect, or it keeps serving traffic indefinitely.
				server.DelClientConnect(c.Id)
			}
		}
	}

	c.EnsureWebPassword()
	applyRateLimit(c)
	file.GetDb().JsonDb.StoreClientsToJsonFile()
	Ok(w, r, NewClientView(c, p))
}

// applyClientCommonFields writes the fields any owner may change.
func applyClientCommonFields(c *file.Client, req *ClientRequest) {
	if req.Remark != nil {
		c.Remark = *req.Remark
	}
	if req.ConfigConnAllow != nil {
		c.ConfigConnAllow = *req.ConfigConnAllow
	}
	if c.Cnf == nil {
		c.Cnf = &file.Config{}
	}
	if req.BasicUser != nil {
		c.Cnf.U = *req.BasicUser
	}
	if req.BasicPassword != nil {
		c.Cnf.P = *req.BasicPassword
	}
	if req.Compress != nil {
		c.Cnf.Compress = *req.Compress
	}
	if req.Crypt != nil {
		c.Cnf.Crypt = *req.Crypt
	}
	if req.WebUserName != nil {
		c.WebUserName = strings.TrimSpace(*req.WebUserName)
	}
	if req.WebPassword != nil {
		c.WebPassword = *req.WebPassword
	}
	if req.WebTotpSecret != nil {
		c.WebTotpSecret = *req.WebTotpSecret
	}
	if req.BlackIPList != nil {
		c.BlackIpList = splitLines(*req.BlackIPList)
	}
}

// applyClientAdminFields writes the fields only an operator may change: the
// vkey is handled by the caller, everything else that costs the server
// resources is here.
func applyClientAdminFields(c *file.Client, req *ClientRequest) {
	if req.VerifyKey != nil && c.VerifyKey == "" {
		// Creation path: the update path handles re-keying, with its own
		// index maintenance.
		c.VerifyKey = *req.VerifyKey
	}
	if req.RateLimit != nil {
		c.RateLimit = *req.RateLimit
	}
	if req.MaxConn != nil {
		c.MaxConn = *req.MaxConn
	}
	if req.MaxTunnelNum != nil {
		c.MaxTunnelNum = *req.MaxTunnelNum
	}
	if c.Flow == nil {
		c.Flow = &file.Flow{}
	}
	if req.FlowLimit != nil {
		c.Flow.FlowLimit = *req.FlowLimit
	}
	if req.TimeLimit != nil {
		c.Flow.TimeLimit = common.GetTimeNoErrByStr(*req.TimeLimit)
	}
	if req.FlowReset != nil && *req.FlowReset {
		c.Flow.ExportFlow = 0
		c.Flow.InletFlow = 0
	}
}

// applyRateLimit reconciles the live limiter with the configured RateLimit.
// The limiter is a running goroutine, so changing the number is not enough;
// it has to be told, and started if it was never running.
func applyRateLimit(c *file.Client) {
	var limit int64
	if c.RateLimit > 0 {
		limit = int64(c.RateLimit) * 1024
	}
	if c.Rate == nil {
		c.Rate = rate.NewRate(limit)
		c.Rate.Start()
		return
	}
	if c.Rate.Limit() != limit {
		c.Rate.SetLimit(limit)
	}
	c.Rate.Start()
}

// handleDeleteClient removes a client along with everything it owns. Admin
// only: a user deleting their own account would orphan the tunnels an operator
// configured for them.
func handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return
	}
	if _, err := file.GetDb().GetClient(id); err != nil {
		NotFound(w, r, "no such client")
		return
	}
	if err := file.GetDb().DelClient(id); err != nil {
		Internal(w, r, err)
		return
	}
	server.DelTunnelAndHostByClientId(id, false)
	server.DelClientConnect(id)
	Ok(w, r, nil)
}

// ClientStatusRequest toggles whether a client may connect.
type ClientStatusRequest struct {
	Status bool `json:"status"`
}

// handleClientStatus enables or disables a client. Admin only.
func handleClientStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return
	}
	c, err := file.GetDb().GetClient(id)
	if err != nil {
		NotFound(w, r, "no such client")
		return
	}
	var req ClientStatusRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	c.Status = req.Status
	if !c.Status {
		server.DelClientConnect(c.Id)
	}
	file.GetDb().JsonDb.StoreClientsToJsonFile()
	Ok(w, r, nil)
}

// ClearRequest names the counter to reset.
type ClearRequest struct {
	// Mode is one of flow, flow_limit, time_limit, rate_limit, conn_limit,
	// tunnel_limit.
	Mode string `json:"mode"`
}

// handleClearClient resets one counter or quota. Admin only, because every
// target is a limit the operator set. id 0 means "every client", which is how
// the old UI offered a bulk reset.
func handleClearClient(w http.ResponseWriter, r *http.Request) {
	var req ClearRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	if !validClientClearMode(req.Mode) {
		BadRequest(w, r, "unknown mode")
		return
	}

	// This route is registered both with and without an id; the bulk form has
	// no path value at all.
	if r.PathValue("id") == "" {
		file.GetDb().JsonDb.Clients.Range(func(_, value any) bool {
			clearClientCounter(value.(*file.Client), req.Mode)
			return true
		})
		file.GetDb().JsonDb.StoreClientsToJsonFile()
		Ok(w, r, nil)
		return
	}

	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return
	}
	c, err := file.GetDb().GetClient(id)
	if err != nil {
		NotFound(w, r, "no such client")
		return
	}
	clearClientCounter(c, req.Mode)
	file.GetDb().JsonDb.StoreClientsToJsonFile()
	Ok(w, r, nil)
}

func validClientClearMode(mode string) bool {
	switch mode {
	case "flow", "flow_limit", "time_limit", "rate_limit", "conn_limit", "tunnel_limit":
		return true
	}
	return false
}

// clearClientCounter resets one counter. Clearing "flow" also clears the
// counters on everything the client owns, because the client total is the sum
// of those and leaving them would make it immediately non-zero again.
func clearClientCounter(c *file.Client, mode string) {
	switch mode {
	case "flow":
		c.Flow.ExportFlow = 0
		c.Flow.InletFlow = 0
		c.ExportFlow = 0
		c.InletFlow = 0
		file.GetDb().JsonDb.Hosts.Range(func(_, value any) bool {
			h := value.(*file.Host)
			if h.Client != nil && h.Client.Id == c.Id {
				h.Flow.InletFlow = 0
				h.Flow.ExportFlow = 0
			}
			return true
		})
		file.GetDb().JsonDb.Tasks.Range(func(_, value any) bool {
			t := value.(*file.Tunnel)
			if t.Client != nil && t.Client.Id == c.Id {
				t.Flow.InletFlow = 0
				t.Flow.ExportFlow = 0
			}
			return true
		})
	case "flow_limit":
		c.Flow.FlowLimit = 0
	case "time_limit":
		c.Flow.TimeLimit = time.Time{}
	case "rate_limit":
		c.RateLimit = 0
	case "conn_limit":
		c.MaxConn = 0
	case "tunnel_limit":
		c.MaxTunnelNum = 0
	}
	applyRateLimit(c)
}

// handlePingClient measures the round trip to a connected client.
func handlePingClient(w http.ResponseWriter, r *http.Request) {
	c, ok := requireClientAccess(w, r)
	if !ok {
		return
	}
	// -1 is PingClient's "no answer"; passing it through rather than turning
	// it into an error lets the UI distinguish "offline" from "request failed".
	Ok(w, r, map[string]any{"rtt": server.PingClient(c.Id, r.RemoteAddr)})
}

// handleClientQRCode renders a TOTP enrolment QR code as a PNG.
//
// It reads the secret from the stored client rather than accepting one in the
// query string, which is what the old endpoint did — that version would encode
// any secret anyone asked it to, turning the admin panel into an oracle for
// building QR codes for arbitrary accounts.
func handleClientQRCode(w http.ResponseWriter, r *http.Request) {
	c, ok := requireClientAccess(w, r)
	if !ok {
		return
	}
	if c.WebTotpSecret == "" {
		NotFound(w, r, "no TOTP secret is configured for this client")
		return
	}
	account := c.WebUserName
	if account == "" {
		account = c.Remark
	}
	uri := crypt.BuildTotpUri(cfg().String("appname"), account, c.WebTotpSecret)
	png, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		Internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The image embeds a shared secret, so it must not be cached anywhere.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// requireClientAccess resolves the {id} path value and checks the caller may
// act on it. It writes the error response itself, so a handler only has to
// check the boolean.
func requireClientAccess(w http.ResponseWriter, r *http.Request) (*file.Client, bool) {
	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return nil, false
	}
	p := CurrentUser(r)
	if !OwnsClient(p, id) {
		// Deliberately the same answer a nonexistent id gets, so this cannot
		// be used to enumerate which client ids exist.
		NotFound(w, r, "no such client")
		return nil, false
	}
	c, err := file.GetDb().GetClient(id)
	if err != nil {
		NotFound(w, r, "no such client")
		return nil, false
	}
	return c, true
}
