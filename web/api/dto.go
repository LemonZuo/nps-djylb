package api

import (
	"time"

	"github.com/djylb/nps/lib/file"
)

// The API serialises purpose-built view types rather than the lib/file models
// directly, for three reasons:
//
//   - lib/file structs ARE the on-disk format. StoreClientsToJsonFile marshals
//     the very same values into conf/*.json, so adding camelCase json tags to
//     them would silently rewrite every operator's database on the next save.
//   - they carry fields that must never leave the process: web passwords, TOTP
//     secrets, embedded mutexes, compiled ACL sets, live rate limiters.
//   - the wire shape can then stay stable while the storage model moves.
//
// Every view below is built by a Xxx() constructor that takes the caller's
// principal, so redaction is a property of the type and not something each
// handler has to remember.

// FlowView reports traffic counters and quota for a client, tunnel or host.
type FlowView struct {
	// InletFlow and ExportFlow are byte counters.
	InletFlow  int64 `json:"inletFlow"`
	ExportFlow int64 `json:"exportFlow"`
	// FlowLimit is the quota in MiB; 0 means unlimited.
	FlowLimit int64 `json:"flowLimit"`
	// TimeLimit is the expiry as a Unix timestamp in seconds; 0 means none.
	TimeLimit int64 `json:"timeLimit"`
}

func newFlowView(f *file.Flow) FlowView {
	if f == nil {
		return FlowView{}
	}
	f.RLock()
	defer f.RUnlock()
	return FlowView{
		InletFlow:  f.InletFlow,
		ExportFlow: f.ExportFlow,
		FlowLimit:  f.FlowLimit,
		TimeLimit:  unixOrZero(f.TimeLimit),
	}
}

// unixOrZero maps the zero time to 0 rather than to the year 1, which would
// otherwise reach the UI as a negative timestamp and render as a date in 1
// AD.
func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// TargetView is the backend a tunnel or host forwards to.
type TargetView struct {
	// Target is the newline-separated backend list.
	Target string `json:"target"`
	// ProxyProtocol is 0 (off), 1 (v1) or 2 (v2).
	ProxyProtocol int `json:"proxyProtocol"`
	// LocalProxy makes the server dial the backend itself instead of asking
	// the client to.
	LocalProxy bool `json:"localProxy"`
}

func newTargetView(t *file.Target) TargetView {
	if t == nil {
		return TargetView{}
	}
	return TargetView{
		Target:        t.TargetStr,
		ProxyProtocol: t.ProxyProtocol,
		LocalProxy:    t.LocalProxy,
	}
}

func authContent(m *file.MultiAccount) string {
	if m == nil {
		return ""
	}
	return m.Content
}

// ClientRef identifies the owning client on a tunnel or host row, so a list
// can be rendered without a second lookup per row.
type ClientRef struct {
	ID        int    `json:"id"`
	Remark    string `json:"remark"`
	VerifyKey string `json:"verifyKey,omitempty"`
	IsConnect bool   `json:"isConnect"`
}

func newClientRef(c *file.Client, p *Principal) ClientRef {
	if c == nil {
		return ClientRef{}
	}
	ref := ClientRef{ID: c.Id, Remark: c.Remark, IsConnect: c.IsConnect}
	if OwnsClient(p, c.Id) {
		ref.VerifyKey = c.VerifyKey
	}
	return ref
}

// ClientView is one row of the client list.
type ClientView struct {
	ID     int    `json:"id"`
	Remark string `json:"remark"`
	// VerifyKey is the npc connection key. It is the caller's own key or, for
	// an admin, anyone's; a user never sees another client's. The list is
	// already scoped, so this is belt and braces.
	VerifyKey string `json:"verifyKey,omitempty"`
	Addr      string `json:"addr"`
	LocalAddr string `json:"localAddr"`
	Mode      string `json:"mode"`
	Version   string `json:"version"`
	// Status is whether the client is permitted to connect; IsConnect is
	// whether it currently is.
	Status    bool `json:"status"`
	IsConnect bool `json:"isConnect"`
	// RateLimit is in KiB/s; 0 means unlimited.
	RateLimit int `json:"rateLimit"`
	// NowRate is the measured throughput in bytes/s.
	NowRate int64 `json:"nowRate"`
	// MaxConn caps concurrent connections; NowConn is the live count.
	MaxConn int   `json:"maxConn"`
	NowConn int32 `json:"nowConn"`
	// MaxTunnelNum caps tunnels plus hosts; 0 means unlimited.
	MaxTunnelNum int `json:"maxTunnelNum"`
	// ConfigConnAllow lets the client create tunnels from its own config file.
	ConfigConnAllow bool `json:"configConnAllow"`
	// WebUserName is the client's web login name. The matching password and
	// TOTP secret are deliberately absent: see the file comment on
	// clients.go's update handler for how they are changed without ever
	// being read back.
	WebUserName string `json:"webUserName"`
	// HasWebPassword and HasTotp let the UI show whether a credential is set
	// without disclosing it.
	HasWebPassword bool `json:"hasWebPassword"`
	HasTotp        bool `json:"hasTotp"`
	// BasicUser and BasicPassword are the HTTP basic-auth pair npc offers to
	// its local services. They are configuration the operator typed, not a
	// server-side secret, and the old UI showed them.
	BasicUser     string   `json:"basicUser"`
	BasicPassword string   `json:"basicPassword"`
	Compress      bool     `json:"compress"`
	Crypt         bool     `json:"crypt"`
	Flow          FlowView `json:"flow"`
	BlackIPList   []string `json:"blackIpList"`
	// TunnelNum is the client's current tunnel plus host count, so the UI can
	// show usage against MaxTunnelNum.
	TunnelNum      int    `json:"tunnelNum"`
	CreateTime     string `json:"createTime"`
	LastOnlineTime string `json:"lastOnlineTime"`
}

// NewClientView renders c for p. Nothing here is secret to an owner, except
// the credentials, which are never rendered at all.
func NewClientView(c *file.Client, p *Principal) ClientView {
	v := ClientView{
		ID:              c.Id,
		Remark:          c.Remark,
		Addr:            c.Addr,
		LocalAddr:       c.LocalAddr,
		Mode:            c.Mode,
		Version:         c.Version,
		Status:          c.Status,
		IsConnect:       c.IsConnect,
		RateLimit:       c.RateLimit,
		MaxConn:         c.MaxConn,
		NowConn:         c.NowConn,
		MaxTunnelNum:    c.MaxTunnelNum,
		ConfigConnAllow: c.ConfigConnAllow,
		WebUserName:     c.WebUserName,
		HasWebPassword:  c.WebPassword != "",
		HasTotp:         c.WebTotpSecret != "",
		Flow:            newFlowView(c.Flow),
		BlackIPList:     c.BlackIpList,
		TunnelNum:       c.GetTunnelNum(),
		CreateTime:      c.CreateTime,
		LastOnlineTime:  c.LastOnlineTime,
	}
	if OwnsClient(p, c.Id) {
		v.VerifyKey = c.VerifyKey
	}
	if c.Cnf != nil {
		v.BasicUser = c.Cnf.U
		v.BasicPassword = c.Cnf.P
		v.Compress = c.Cnf.Compress
		v.Crypt = c.Cnf.Crypt
	}
	if c.Rate != nil {
		v.NowRate = c.Rate.Now()
	}
	if v.BlackIPList == nil {
		// An absent list and an empty one mean the same thing here, and the
		// UI should not have to handle null.
		v.BlackIPList = []string{}
	}
	return v
}

// TunnelView is one row of the tunnel list.
type TunnelView struct {
	ID       int    `json:"id"`
	Port     int    `json:"port"`
	ServerIP string `json:"serverIp"`
	// Mode is tcp / udp / socks5 / httpProxy / mixProxy / file / secret / p2p.
	Mode string `json:"mode"`
	// Status is whether the tunnel is enabled; RunStatus whether its listener
	// is actually up right now.
	Status     bool       `json:"status"`
	RunStatus  bool       `json:"runStatus"`
	Client     ClientRef  `json:"client"`
	Remark     string     `json:"remark"`
	Password   string     `json:"password"`
	TargetType string     `json:"targetType"`
	Target     TargetView `json:"target"`
	// Auth is the newline-separated user:password list guarding the tunnel.
	Auth string `json:"auth"`
	// LocalPath and StripPre configure file mode.
	LocalPath string `json:"localPath"`
	StripPre  string `json:"stripPre"`
	// HttpProxy and Socks5Proxy select the sub-protocols of mixProxy mode.
	HttpProxy   bool `json:"httpProxy"`
	Socks5Proxy bool `json:"socks5Proxy"`
	// DestAclMode is 0 (off), 1 (whitelist) or 2 (blacklist).
	DestAclMode  int      `json:"destAclMode"`
	DestAclRules string   `json:"destAclRules"`
	NowConn      int32    `json:"nowConn"`
	Flow         FlowView `json:"flow"`
}

// NewTunnelView renders t for p.
func NewTunnelView(t *file.Tunnel, p *Principal) TunnelView {
	return TunnelView{
		ID:           t.Id,
		Port:         t.Port,
		ServerIP:     t.ServerIp,
		Mode:         t.Mode,
		Status:       t.Status,
		RunStatus:    t.RunStatus,
		Client:       newClientRef(t.Client, p),
		Remark:       t.Remark,
		Password:     t.Password,
		TargetType:   t.TargetType,
		Target:       newTargetView(t.Target),
		Auth:         authContent(t.UserAuth),
		LocalPath:    t.LocalPath,
		StripPre:     t.StripPre,
		HttpProxy:    t.HttpProxy,
		Socks5Proxy:  t.Socks5Proxy,
		DestAclMode:  t.DestAclMode,
		DestAclRules: t.DestAclRules,
		NowConn:      t.NowConn,
		Flow:         newFlowView(t.Flow),
	}
}

// HostView is one row of the domain (host) list.
type HostView struct {
	ID   int    `json:"id"`
	Host string `json:"host"`
	// Scheme is http, https or all.
	Scheme string `json:"scheme"`
	// Location is the path prefix this rule matches; PathRewrite replaces it.
	Location    string `json:"location"`
	PathRewrite string `json:"pathRewrite"`
	// RedirectURL, when set, serves a 307 instead of proxying.
	RedirectURL      string     `json:"redirectUrl"`
	Remark           string     `json:"remark"`
	Client           ClientRef  `json:"client"`
	Target           TargetView `json:"target"`
	TargetIsHttps    bool       `json:"targetIsHttps"`
	HeaderChange     string     `json:"headerChange"`
	RespHeaderChange string     `json:"respHeaderChange"`
	HostChange       string     `json:"hostChange"`
	Auth             string     `json:"auth"`
	// IsClose is the disabled flag; it is inverted from the tunnel list's
	// Status on purpose, because that is how it is stored.
	IsClose bool `json:"isClose"`
	// TLS handling.
	HttpsJustProxy bool   `json:"httpsJustProxy"`
	TlsOffload     bool   `json:"tlsOffload"`
	AutoSSL        bool   `json:"autoSsl"`
	AutoHttps      bool   `json:"autoHttps"`
	AutoCORS       bool   `json:"autoCors"`
	CompatMode     bool   `json:"compatMode"`
	CertType       string `json:"certType"`
	// CertFile and KeyFile are paths or inline PEM. The key is admin-only: an
	// inline PEM private key is a secret, and only an admin can set one.
	CertFile string   `json:"certFile"`
	KeyFile  string   `json:"keyFile,omitempty"`
	NowConn  int32    `json:"nowConn"`
	Flow     FlowView `json:"flow"`
}

// NewHostView renders h for p.
func NewHostView(h *file.Host, p *Principal) HostView {
	v := HostView{
		ID:               h.Id,
		Host:             h.Host,
		Scheme:           h.Scheme,
		Location:         h.Location,
		PathRewrite:      h.PathRewrite,
		RedirectURL:      h.RedirectURL,
		Remark:           h.Remark,
		Client:           newClientRef(h.Client, p),
		Target:           newTargetView(h.Target),
		TargetIsHttps:    h.TargetIsHttps,
		HeaderChange:     h.HeaderChange,
		RespHeaderChange: h.RespHeaderChange,
		HostChange:       h.HostChange,
		Auth:             authContent(h.UserAuth),
		IsClose:          h.IsClose,
		HttpsJustProxy:   h.HttpsJustProxy,
		TlsOffload:       h.TlsOffload,
		AutoSSL:          h.AutoSSL,
		AutoHttps:        h.AutoHttps,
		AutoCORS:         h.AutoCORS,
		CompatMode:       h.CompatMode,
		CertType:         h.CertType,
		CertFile:         h.CertFile,
		NowConn:          h.NowConn,
		Flow:             newFlowView(h.Flow),
	}
	if p != nil && p.IsAdmin {
		v.KeyFile = h.KeyFile
	}
	return v
}
