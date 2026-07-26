package api

import (
	"net/http"
	"strings"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/file"
	"github.com/djylb/nps/server"
	"github.com/djylb/nps/server/tool"
)

// Tunnel endpoints.
//
// A tunnel binds a listening port on this server to a target reachable from
// one client, so every route here answers two questions before touching
// anything: does the caller own that client, and is the requested field theirs
// to set. Port allocation additionally goes through tool.TestServerPort so the
// allow_ports policy in nps.conf is enforced no matter what the request says.

// handleListTunnels returns a page of tunnels.
//
// The underlying server.GetTunnel filter treats type and clientId as a pair:
// with a type, clientId 0 means "all clients"; without a type it matches
// clientId exactly. The old UI always sent a type (one page per mode), and the
// new one does too, so the pass-through keeps the admin "all clients" view
// working while resolveClientScope still pins users to their own rows.
func handleListTunnels(w http.ResponseWriter, r *http.Request) {
	p := CurrentUser(r)
	q := parseListQuery(r)
	scope := resolveClientScope(p, q.ClientID)
	mode := strings.TrimSpace(r.URL.Query().Get("type"))

	list, total := server.GetTunnel(q.Offset, q.Limit, mode, scope, q.Search, q.Sort, q.Order)
	rows := make([]TunnelView, 0, len(list))
	for _, t := range list {
		rows = append(rows, NewTunnelView(t, p))
	}
	OkList(w, r, rows, int64(total))
}

// handleGetTunnel returns one tunnel.
func handleGetTunnel(w http.ResponseWriter, r *http.Request) {
	t, ok := requireTunnelAccess(w, r)
	if !ok {
		return
	}
	Ok(w, r, NewTunnelView(t, CurrentUser(r)))
}

// TunnelRequest is the create/update body; pointers distinguish "absent" from
// "zero" so an update only touches what it names.
type TunnelRequest struct {
	ClientID      *int    `json:"clientId"`
	Mode          *string `json:"mode"`
	Port          *int    `json:"port"`
	ServerIP      *string `json:"serverIp"`
	Remark        *string `json:"remark"`
	Password      *string `json:"password"`
	Target        *string `json:"target"`
	TargetType    *string `json:"targetType"`
	ProxyProtocol *int    `json:"proxyProtocol"`
	LocalProxy    *bool   `json:"localProxy"`
	Auth          *string `json:"auth"`
	LocalPath     *string `json:"localPath"`
	StripPre      *string `json:"stripPre"`
	HttpProxy     *bool   `json:"httpProxy"`
	Socks5Proxy   *bool   `json:"socks5Proxy"`
	DestAclMode   *int    `json:"destAclMode"`
	DestAclRules  *string `json:"destAclRules"`
	FlowLimit     *int64  `json:"flowLimit"`
	TimeLimit     *string `json:"timeLimit"`
	FlowReset     *bool   `json:"flowReset"`
}

// handleCreateTunnel adds and starts a tunnel.
func handleCreateTunnel(w http.ResponseWriter, r *http.Request) {
	p := CurrentUser(r)
	var req TunnelRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	if req.Mode == nil || strings.TrimSpace(*req.Mode) == "" {
		BadRequest(w, r, "mode is required")
		return
	}

	clientID := 0
	if req.ClientID != nil {
		clientID = *req.ClientID
	}
	if !p.IsAdmin {
		// Whatever the body says, a user creates tunnels for themselves.
		clientID = p.ClientID
	}
	client, err := file.GetDb().GetClient(clientID)
	if err != nil {
		BadRequest(w, r, "no such client")
		return
	}
	if client.MaxTunnelNum != 0 && client.GetTunnelNum() >= client.MaxTunnelNum {
		Forbidden(w, r, "the number of tunnels exceeds the limit")
		return
	}

	t := &file.Tunnel{
		Id:       int(file.GetDb().JsonDb.GetTaskId()),
		Mode:     strings.TrimSpace(*req.Mode),
		Status:   true,
		Client:   client,
		Target:   &file.Target{},
		UserAuth: &file.MultiAccount{},
		Flow:     &file.Flow{},
	}
	applyTunnelFields(t, &req, p)

	if t.Port <= 0 {
		t.Port = tool.GenerateServerPort(t.Mode)
	}
	if !tool.TestServerPort(t.Port, t.Mode) {
		Conflict(w, r, "the port cannot be opened because it may be occupied or is no longer allowed")
		return
	}

	if err := file.GetDb().NewTask(t); err != nil {
		Conflict(w, r, err.Error())
		return
	}
	// NewTask replaces t.Flow with a fresh zero value, so the requested limits
	// have to be applied after it — before it they would be silently lost.
	if applyTunnelFlowFields(t, &req, p) {
		file.GetDb().JsonDb.StoreTasksToJsonFile()
	}
	if err := server.AddTask(t); err != nil {
		// The listener failed to come up; keep the DB consistent with reality.
		_ = file.GetDb().DelTask(t.Id)
		Internal(w, r, err)
		return
	}
	Ok(w, r, NewTunnelView(t, p))
}

// handleUpdateTunnel edits a tunnel and restarts its listener.
func handleUpdateTunnel(w http.ResponseWriter, r *http.Request) {
	t, ok := requireTunnelAccess(w, r)
	if !ok {
		return
	}
	p := CurrentUser(r)

	var req TunnelRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	// Re-homing a tunnel to another client is an admin move: for a user it
	// would be a way to run traffic through a tenant they don't own.
	if req.ClientID != nil && *req.ClientID != t.Client.Id {
		if !p.IsAdmin {
			Forbidden(w, r, "moving a tunnel to another client requires administrator privileges")
			return
		}
		client, err := file.GetDb().GetClient(*req.ClientID)
		if err != nil {
			BadRequest(w, r, "no such client")
			return
		}
		t.Client = client
	}

	if req.Port != nil && *req.Port != t.Port {
		port := *req.Port
		if port <= 0 {
			port = tool.GenerateServerPort(t.Mode)
		}
		if !tool.TestServerPort(port, t.Mode) {
			Conflict(w, r, "the port cannot be opened because it may be occupied or is no longer allowed")
			return
		}
		t.Port = port
	}

	applyTunnelFields(t, &req, p)
	applyTunnelFlowFields(t, &req, p)

	if err := file.GetDb().UpdateTask(t); err != nil {
		Internal(w, r, err)
		return
	}
	_ = server.StopServer(t.Id)
	if err := server.StartTask(t.Id); err != nil {
		Internal(w, r, err)
		return
	}
	Ok(w, r, NewTunnelView(t, p))
}

// applyTunnelFields writes the non-flow fields present in the request.
func applyTunnelFields(t *file.Tunnel, req *TunnelRequest, p *Principal) {
	if req.Mode != nil && strings.TrimSpace(*req.Mode) != "" {
		t.Mode = strings.TrimSpace(*req.Mode)
	}
	if req.ServerIP != nil {
		t.ServerIp = strings.TrimSpace(*req.ServerIP)
	}
	if req.Remark != nil {
		t.Remark = *req.Remark
	}
	if req.Password != nil {
		t.Password = *req.Password
	}
	if req.TargetType != nil {
		t.TargetType = *req.TargetType
	}
	if req.LocalPath != nil {
		t.LocalPath = *req.LocalPath
	}
	if req.StripPre != nil {
		t.StripPre = *req.StripPre
	}
	if req.HttpProxy != nil {
		t.HttpProxy = *req.HttpProxy
	}
	if req.Socks5Proxy != nil {
		t.Socks5Proxy = *req.Socks5Proxy
	}
	if req.DestAclMode != nil {
		mode := *req.DestAclMode
		if mode != file.AclOff && mode != file.AclWhitelist && mode != file.AclBlacklist {
			mode = file.AclOff
		}
		t.DestAclMode = mode
	}
	if req.DestAclRules != nil {
		t.DestAclRules = normalizeMultiline(*req.DestAclRules)
	}
	if req.Auth != nil {
		t.UserAuth = &file.MultiAccount{
			Content:    *req.Auth,
			AccountMap: common.DealMultiUser(*req.Auth),
		}
	}

	if t.Target == nil {
		t.Target = &file.Target{}
	}
	if req.Target != nil {
		target := normalizeMultiline(*req.Target)
		// bridge:// targets make the server dial through another client's
		// bridge connection; only an operator may point traffic there. On an
		// edit the existing value survives, on a create it becomes empty.
		if !p.IsAdmin && strings.Contains(target, "bridge://") {
			target = t.Target.TargetStr
		}
		t.Target.TargetStr = target
	}
	if req.ProxyProtocol != nil {
		t.Target.ProxyProtocol = *req.ProxyProtocol
	}
	if req.LocalProxy != nil {
		allowLocal := p.IsAdmin || AllowUserLocal()
		t.Target.LocalProxy = (t.Client.Id > 0 && *req.LocalProxy && allowLocal) || t.Client.Id <= 0
	}
}

// applyTunnelFlowFields writes the quota fields. A user only gets to set them
// when the corresponding allow_* switch is on; otherwise these are the
// operator's levers. Reports whether anything changed.
func applyTunnelFlowFields(t *file.Tunnel, req *TunnelRequest, p *Principal) bool {
	if t.Flow == nil {
		t.Flow = &file.Flow{}
	}
	changed := false
	if req.FlowLimit != nil && (p.IsAdmin || AllowFlowLimit()) {
		t.Flow.FlowLimit = *req.FlowLimit
		changed = true
	}
	if req.TimeLimit != nil && (p.IsAdmin || AllowTimeLimit()) {
		t.Flow.TimeLimit = common.GetTimeNoErrByStr(*req.TimeLimit)
		changed = true
	}
	if req.FlowReset != nil && *req.FlowReset && p.IsAdmin {
		t.Flow.ExportFlow = 0
		t.Flow.InletFlow = 0
		changed = true
	}
	return changed
}

// handleDeleteTunnel stops and removes a tunnel.
func handleDeleteTunnel(w http.ResponseWriter, r *http.Request) {
	t, ok := requireTunnelAccess(w, r)
	if !ok {
		return
	}
	if err := server.DelTask(t.Id); err != nil {
		Internal(w, r, err)
		return
	}
	Ok(w, r, nil)
}

// handleStartTunnel brings the listener up.
func handleStartTunnel(w http.ResponseWriter, r *http.Request) {
	t, ok := requireTunnelAccess(w, r)
	if !ok {
		return
	}
	if err := server.StartTask(t.Id); err != nil {
		if err.Error() == "the port open error" {
			Conflict(w, r, "the port cannot be opened because it may be occupied or is no longer allowed")
			return
		}
		Internal(w, r, err)
		return
	}
	Ok(w, r, nil)
}

// handleStopTunnel takes the listener down without deleting the record.
func handleStopTunnel(w http.ResponseWriter, r *http.Request) {
	t, ok := requireTunnelAccess(w, r)
	if !ok {
		return
	}
	if err := server.StopServer(t.Id); err != nil && err.Error() != "task is not running" {
		// An already-stopped task is the state the caller asked for; only a
		// real failure is worth reporting.
		Internal(w, r, err)
		return
	}
	Ok(w, r, nil)
}

// TunnelToggleRequest flips one boolean sub-feature of a tunnel.
type TunnelToggleRequest struct {
	// Name is "http" or "socks5" (the mixProxy sub-protocols).
	Name string `json:"name"`
	// Action is "start", "stop" or "toggle".
	Action string `json:"action"`
}

// handleToggleTunnel enables/disables the HTTP or SOCKS5 half of a mixProxy
// tunnel without restarting it.
func handleToggleTunnel(w http.ResponseWriter, r *http.Request) {
	t, ok := requireTunnelAccess(w, r)
	if !ok {
		return
	}
	var req TunnelToggleRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	var dst *bool
	switch req.Name {
	case "http":
		dst = &t.HttpProxy
	case "socks5":
		dst = &t.Socks5Proxy
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
	if err := file.GetDb().UpdateTask(t); err != nil {
		Internal(w, r, err)
		return
	}
	Ok(w, r, nil)
}

// handleClearTunnel resets a tunnel counter or quota. Admin only — each target
// is a limit the operator imposed.
func handleClearTunnel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return
	}
	t, err := file.GetDb().GetTask(id)
	if err != nil {
		NotFound(w, r, "no such tunnel")
		return
	}
	var req ClearRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	switch req.Mode {
	case "flow":
		t.Flow.ExportFlow = 0
		t.Flow.InletFlow = 0
	case "flow_limit":
		t.Flow.FlowLimit = 0
	case "time_limit":
		t.Flow.TimeLimit = common.GetTimeNoErrByStr("")
	default:
		BadRequest(w, r, "unknown mode")
		return
	}
	if err := file.GetDb().UpdateTask(t); err != nil {
		Internal(w, r, err)
		return
	}
	Ok(w, r, nil)
}

// requireTunnelAccess resolves {id} and checks the caller owns the tunnel's
// client. Like requireClientAccess, a foreign id and a nonexistent one get the
// same 404 so ids cannot be enumerated.
func requireTunnelAccess(w http.ResponseWriter, r *http.Request) (*file.Tunnel, bool) {
	id, ok := pathID(r)
	if !ok {
		BadRequest(w, r, "invalid id")
		return nil, false
	}
	t, err := file.GetDb().GetTask(id)
	if err != nil {
		NotFound(w, r, "no such tunnel")
		return nil, false
	}
	p := CurrentUser(r)
	if t.Client == nil || !OwnsClient(p, t.Client.Id) {
		NotFound(w, r, "no such tunnel")
		return nil, false
	}
	return t, true
}
