package api

import (
	"net/http"
	"strings"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/crypt"
	"github.com/djylb/nps/lib/file"
	"github.com/djylb/nps/server"
)

// Host (domain reverse-proxy) endpoints.
//
// Hosts have no per-record listener — the shared HTTP/HTTPS proxy routes by
// Host header through file.HostIndex, with resolved results memoised in
// server.HttpProxyCache. Two consequences shape every handler here:
//
//   - any mutation must call server.HttpProxyCache.Remove(id), or the proxy
//     keeps serving the pre-edit configuration until the entry ages out;
//   - a host/location/scheme change is a rename in HostIndex, which needs an
//     explicit uniqueness check plus Remove/Add — storing alone would leave the
//     old key routing to this record forever.

// handleListHosts returns a page of hosts, scoped to the caller.
func handleListHosts(w http.ResponseWriter, r *http.Request) {
	p := CurrentUser(r)
	q := parseListQuery(r)
	scope := resolveClientScope(p, q.ClientID)

	list, total := server.GetHostList(q.Offset, q.Limit, scope, q.Search, q.Sort, q.Order)
	rows := make([]HostView, 0, len(list))
	for _, h := range list {
		rows = append(rows, NewHostView(h, p))
	}
	OkList(w, r, rows, int64(total))
}

// handleGetHost returns one host.
func handleGetHost(w http.ResponseWriter, r *http.Request) {
	h, ok := requireHostAccess(w, r)
	if !ok {
		return
	}
	Ok(w, r, NewHostView(h, CurrentUser(r)))
}

// HostRequest is the create/update body; pointers distinguish "absent" from
// "zero" so an update only touches what it names.
type HostRequest struct {
	ClientID         *int    `json:"clientId"`
	Host             *string `json:"host"`
	Scheme           *string `json:"scheme"`
	Location         *string `json:"location"`
	PathRewrite      *string `json:"pathRewrite"`
	RedirectURL      *string `json:"redirectUrl"`
	Remark           *string `json:"remark"`
	Target           *string `json:"target"`
	ProxyProtocol    *int    `json:"proxyProtocol"`
	LocalProxy       *bool   `json:"localProxy"`
	TargetIsHttps    *bool   `json:"targetIsHttps"`
	HeaderChange     *string `json:"headerChange"`
	RespHeaderChange *string `json:"respHeaderChange"`
	HostChange       *string `json:"hostChange"`
	Auth             *string `json:"auth"`
	HttpsJustProxy   *bool   `json:"httpsJustProxy"`
	TlsOffload       *bool   `json:"tlsOffload"`
	AutoSSL          *bool   `json:"autoSsl"`
	AutoHttps        *bool   `json:"autoHttps"`
	AutoCORS         *bool   `json:"autoCors"`
	CompatMode       *bool   `json:"compatMode"`
	CertFile         *string `json:"certFile"`
	KeyFile          *string `json:"keyFile"`
	FlowLimit        *int64  `json:"flowLimit"`
	TimeLimit        *string `json:"timeLimit"`
	FlowReset        *bool   `json:"flowReset"`
}

// handleCreateHost adds a host record.
func handleCreateHost(w http.ResponseWriter, r *http.Request) {
	p := CurrentUser(r)
	var req HostRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	if req.Host == nil || strings.TrimSpace(*req.Host) == "" {
		BadRequest(w, r, "host is required")
		return
	}

	clientID := 0
	if req.ClientID != nil {
		clientID = *req.ClientID
	}
	if !p.IsAdmin {
		clientID = p.ClientID
	}
	client, err := file.GetDb().GetClient(clientID)
	if err != nil {
		BadRequest(w, r, "no such client")
		return
	}
	// Hosts count against the same quota as tunnels: GetTunnelNum sums both.
	if client.MaxTunnelNum != 0 && client.GetTunnelNum() >= client.MaxTunnelNum {
		Forbidden(w, r, "the number of tunnels exceeds the limit")
		return
	}

	h := &file.Host{
		Id:       int(file.GetDb().JsonDb.GetHostId()),
		Client:   client,
		Target:   &file.Target{},
		UserAuth: &file.MultiAccount{},
		Flow:     &file.Flow{},
	}
	applyHostFields(h, &req, p)

	if err := file.GetDb().NewHost(h); err != nil {
		// The only NewHost failure is the host+location+scheme triple already
		// being routed.
		Conflict(w, r, err.Error())
		return
	}
	// NewHost replaces h.Flow with a fresh zero value, so limits go on after.
	if applyHostFlowFields(h, &req, p) {
		file.GetDb().JsonDb.StoreHostToJsonFile()
	}
	Ok(w, r, NewHostView(h, p))
}

// handleUpdateHost edits a host record in place.
func handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	h, ok := requireHostAccess(w, r)
	if !ok {
		return
	}
	p := CurrentUser(r)

	var req HostRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	if req.ClientID != nil && (h.Client == nil || *req.ClientID != h.Client.Id) {
		if !p.IsAdmin {
			Forbidden(w, r, "moving a host to another client requires administrator privileges")
			return
		}
		client, err := file.GetDb().GetClient(*req.ClientID)
		if err != nil {
			BadRequest(w, r, "no such client")
			return
		}
		h.Client = client
	}

	// Work out what the routing triple would become, and reject the edit
	// before mutating anything if it would collide with another record.
	newHost, newLocation, newScheme := h.Host, h.Location, h.Scheme
	if req.Host != nil {
		newHost = strings.TrimSpace(*req.Host)
	}
	if req.Location != nil {
		newLocation = strings.TrimSpace(*req.Location)
	}
	if req.Scheme != nil {
		newScheme = normalizeScheme(*req.Scheme)
	}
	if newHost != h.Host || newLocation != h.Location || newScheme != h.Scheme {
		probe := &file.Host{Id: h.Id, Host: newHost, Location: newLocation, Scheme: newScheme}
		if file.GetDb().IsHostExist(probe) {
			Conflict(w, r, "host has exist")
			return
		}
	}
	if newHost != h.Host {
		file.HostIndex.Remove(h.Host, h.Id)
		file.HostIndex.Add(newHost, h.Id)
	}
	h.Host = newHost
	h.Location = newLocation
	h.Scheme = newScheme

	applyHostFields(h, &req, p)
	applyHostFlowFields(h, &req, p)

	// The cert fingerprint feeds the TLS handshake cache; recompute it on
	// every edit like the old controller did, since CertFile/KeyFile may have
	// changed above.
	h.CertType = common.GetCertType(h.CertFile)
	h.CertHash = crypt.FNV1a64(h.CertType, h.CertFile, h.KeyFile)

	file.GetDb().JsonDb.StoreHostToJsonFile()
	server.HttpProxyCache.Remove(h.Id)
	Ok(w, r, NewHostView(h, p))
}

// normalizeScheme mirrors the stored invariant: anything but http/https is all.
func normalizeScheme(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s != "http" && s != "https" {
		return "all"
	}
	return s
}

// applyHostFields writes the non-flow fields present in the request. Host,
// Location and Scheme are handled by the callers, which own the uniqueness
// check and index maintenance around them.
func applyHostFields(h *file.Host, req *HostRequest, p *Principal) {
	// The create path funnels through here too; normalise its raw values.
	if req.Host != nil && h.Host == "" {
		h.Host = strings.TrimSpace(*req.Host)
	}
	if req.Location != nil && h.Location == "" {
		h.Location = strings.TrimSpace(*req.Location)
	}
	if req.Scheme != nil && h.Scheme == "" {
		h.Scheme = normalizeScheme(*req.Scheme)
	}
	if req.PathRewrite != nil {
		h.PathRewrite = *req.PathRewrite
	}
	if req.RedirectURL != nil {
		h.RedirectURL = *req.RedirectURL
	}
	if req.Remark != nil {
		h.Remark = *req.Remark
	}
	if req.HeaderChange != nil {
		h.HeaderChange = *req.HeaderChange
	}
	if req.RespHeaderChange != nil {
		h.RespHeaderChange = *req.RespHeaderChange
	}
	if req.HostChange != nil {
		h.HostChange = *req.HostChange
	}
	if req.Auth != nil {
		h.UserAuth = &file.MultiAccount{
			Content:    *req.Auth,
			AccountMap: common.DealMultiUser(*req.Auth),
		}
	}
	if req.HttpsJustProxy != nil {
		h.HttpsJustProxy = *req.HttpsJustProxy
	}
	if req.TlsOffload != nil {
		h.TlsOffload = *req.TlsOffload
	}
	if req.AutoSSL != nil {
		h.AutoSSL = *req.AutoSSL
	}
	if req.AutoHttps != nil {
		h.AutoHttps = *req.AutoHttps
	}
	if req.AutoCORS != nil {
		h.AutoCORS = *req.AutoCORS
	}
	if req.CompatMode != nil {
		h.CompatMode = *req.CompatMode
	}
	if req.TargetIsHttps != nil {
		h.TargetIsHttps = *req.TargetIsHttps
	}
	if req.CertFile != nil {
		h.CertFile = *req.CertFile
	}
	if req.KeyFile != nil {
		h.KeyFile = *req.KeyFile
	}

	if h.Target == nil {
		h.Target = &file.Target{}
	}
	if req.Target != nil {
		target := normalizeMultiline(*req.Target)
		// Same rule as tunnels: bridge:// dials through another client's
		// connection and is operator-only.
		if !p.IsAdmin && strings.Contains(target, "bridge://") {
			target = h.Target.TargetStr
		}
		h.Target.TargetStr = target
	}
	if req.ProxyProtocol != nil {
		h.Target.ProxyProtocol = *req.ProxyProtocol
	}
	if req.LocalProxy != nil {
		allowLocal := p.IsAdmin || AllowUserLocal()
		clientID := 0
		if h.Client != nil {
			clientID = h.Client.Id
		}
		h.Target.LocalProxy = (clientID > 0 && *req.LocalProxy && allowLocal) || clientID <= 0
	}
}

// applyHostFlowFields writes the quota fields under the same allow_* gates as
// tunnels. Reports whether anything changed.
func applyHostFlowFields(h *file.Host, req *HostRequest, p *Principal) bool {
	if h.Flow == nil {
		h.Flow = &file.Flow{}
	}
	changed := false
	if req.FlowLimit != nil && (p.IsAdmin || AllowFlowLimit()) {
		h.Flow.FlowLimit = *req.FlowLimit
		changed = true
	}
	if req.TimeLimit != nil && (p.IsAdmin || AllowTimeLimit()) {
		h.Flow.TimeLimit = common.GetTimeNoErrByStr(*req.TimeLimit)
		changed = true
	}
	if req.FlowReset != nil && *req.FlowReset && p.IsAdmin {
		h.Flow.ExportFlow = 0
		h.Flow.InletFlow = 0
		changed = true
	}
	return changed
}

// handleDeleteHost removes a host record.
func handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	h, ok := requireHostAccess(w, r)
	if !ok {
		return
	}
	server.HttpProxyCache.Remove(h.Id)
	if err := file.GetDb().DelHost(h.Id); err != nil {
		Internal(w, r, err)
		return
	}
	Ok(w, r, nil)
}

// handleStartHost reopens a closed host.
func handleStartHost(w http.ResponseWriter, r *http.Request) {
	setHostClosed(w, r, false)
}

// handleStopHost closes a host: the record stays, the proxy answers 404.
func handleStopHost(w http.ResponseWriter, r *http.Request) {
	setHostClosed(w, r, true)
}

func setHostClosed(w http.ResponseWriter, r *http.Request, closed bool) {
	h, ok := requireHostAccess(w, r)
	if !ok {
		return
	}
	h.IsClose = closed
	file.GetDb().JsonDb.StoreHostToJsonFile()
	server.HttpProxyCache.Remove(h.Id)
	Ok(w, r, nil)
}

// HostToggleRequest flips one boolean feature of a host.
type HostToggleRequest struct {
	// Name is one of auto_ssl, https_just_proxy, tls_offload, auto_https,
	// auto_cors, compat_mode, target_is_https.
	Name string `json:"name"`
	// Action is "start", "stop" or "toggle".
	Action string `json:"action"`
}

// handleToggleHost enables/disables one boolean feature without a full edit.
func handleToggleHost(w http.ResponseWriter, r *http.Request) {
	h, ok := requireHostAccess(w, r)
	if !ok {
		return
	}
	var req HostToggleRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	var dst *bool
	switch req.Name {
	case "auto_ssl":
		dst = &h.AutoSSL
	case "https_just_proxy":
		dst = &h.HttpsJustProxy
	case "tls_offload":
		dst = &h.TlsOffload
	case "auto_https":
		dst = &h.AutoHttps
	case "auto_cors":
		dst = &h.AutoCORS
	case "compat_mode":
		dst = &h.CompatMode
	case "target_is_https":
		dst = &h.TargetIsHttps
	default:
		BadRequest(w, r, "unknown name")
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "start", "true", "on":
		*dst = true
	case "stop", "false", "off":
		*dst = false
	case "toggle", "clear", "turn", "switch":
		*dst = !*dst
	default:
		BadRequest(w, r, "unknown action")
		return
	}
	file.GetDb().JsonDb.StoreHostToJsonFile()
	server.HttpProxyCache.Remove(h.Id)
	Ok(w, r, nil)
}

// handleClearHost resets a host counter or quota. Admin only.
func handleClearHost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return
	}
	h, err := file.GetDb().GetHostById(id)
	if err != nil {
		NotFound(w, r, "no such host")
		return
	}
	var req ClearRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	switch req.Mode {
	case "flow":
		h.Flow.ExportFlow = 0
		h.Flow.InletFlow = 0
	case "flow_limit":
		h.Flow.FlowLimit = 0
	case "time_limit":
		h.Flow.TimeLimit = common.GetTimeNoErrByStr("")
	default:
		BadRequest(w, r, "unknown mode")
		return
	}
	file.GetDb().JsonDb.StoreHostToJsonFile()
	server.HttpProxyCache.Remove(h.Id)
	Ok(w, r, nil)
}

// requireHostAccess resolves {id} and checks ownership, answering 404 for
// foreign and nonexistent ids alike.
func requireHostAccess(w http.ResponseWriter, r *http.Request) (*file.Host, bool) {
	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return nil, false
	}
	h, err := file.GetDb().GetHostById(id)
	if err != nil {
		NotFound(w, r, "no such host")
		return nil, false
	}
	p := CurrentUser(r)
	if h.Client == nil || !OwnsClient(p, h.Client.Id) {
		NotFound(w, r, "no such host")
		return nil, false
	}
	return h, true
}
