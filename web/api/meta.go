package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/djylb/nps/bridge"
	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/server"
	"github.com/djylb/nps/server/connection"
)

// The bootstrap endpoint replaces the pile of template variables the old base
// controller stuffed into every page render. The SPA fetches it once after
// login and uses it to build the npc command lines shown on the client page.

// BridgeEndpoint is one way an npc can reach this server.
type BridgeEndpoint struct {
	// Type is the -type= value: tcp, kcp, tls, quic, ws or wss.
	Type string `json:"type"`
	IP   string `json:"ip"`
	Port string `json:"port"`
	// Addr is the assembled -server= value, including the websocket path or
	// the QUIC ALPN suffix where those apply.
	Addr string `json:"addr"`
	// Path is the websocket path, empty for the other transports.
	Path string `json:"path,omitempty"`
	// Alpn is the QUIC ALPN, empty for the other transports.
	Alpn string `json:"alpn,omitempty"`
}

// BootstrapResponse is the deployment-level context the UI needs.
type BootstrapResponse struct {
	Version    string `json:"version"`
	MinVersion string `json:"minVersion"`
	Year       int    `json:"year"`
	// WebBaseURL is the sub-path the UI is mounted under, "" at the root.
	WebBaseURL string `json:"webBaseUrl"`
	// HeadCustomCode is operator-supplied markup. It is raw HTML by design —
	// only someone who can already edit nps.conf can set it.
	HeadCustomCode string `json:"headCustomCode,omitempty"`
	// Preferred is the endpoint the UI should show first; Endpoints lists
	// every enabled transport so the user can pick another.
	Preferred BridgeEndpoint   `json:"preferred"`
	Endpoints []BridgeEndpoint `json:"endpoints"`
	// HTTPProxyPort and HTTPSProxyPort are where the domain-based reverse
	// proxy listens, shown next to each host row.
	HTTPProxyPort  string `json:"httpProxyPort"`
	HTTPSProxyPort string `json:"httpsProxyPort"`
	// P2PAddr is the rendezvous address for p2p tunnels.
	P2PAddr string `json:"p2pAddr"`
	// ServerIsWindows tells the UI to suggest npc.exe rather than ./npc.
	ServerIsWindows bool `json:"serverIsWindows"`
	// Permissions repeats the feature switches so a page can be rendered from
	// bootstrap alone, without also holding the /auth/me response.
	Permissions Permissions `json:"permissions"`
}

// handleBootstrap serves the deployment context. It requires authentication
// because it discloses the bridge topology and the operator's custom markup.
func handleBootstrap(w http.ResponseWriter, r *http.Request) {
	p := CurrentUser(r)
	if p == nil {
		Unauthorized(w, r, "authentication required")
		return
	}

	// The host header is the best guess at an address the browser can reach,
	// and it is what the old UI used when bridge_addr is unset.
	host := common.GetIpByAddr(r.Host)
	endpoints := bridgeEndpoints(host)

	Ok(w, r, BootstrapResponse{
		Version:         server.GetVersion(),
		MinVersion:      server.GetMinVersion(),
		Year:            server.GetCurrentYear(),
		WebBaseURL:      WebBaseURL(),
		HeadCustomCode:  HeadCustomCode(),
		Preferred:       preferredEndpoint(endpoints, host),
		Endpoints:       endpoints,
		HTTPProxyPort:   cfg().String("http_proxy_port"),
		HTTPSProxyPort:  cfg().String("https_proxy_port"),
		P2PAddr:         common.BuildAddress(common.GetServerIp(connection.P2pIp), strconv.Itoa(connection.P2pPort)),
		ServerIsWindows: common.IsWindows(),
		Permissions:     describe(p).Permissions,
	})
}

// bridgeEndpoints lists every transport the operator has enabled and chosen to
// advertise. Each transport has its own bridge_<t>_show_{ip,port} overrides,
// because the address a client should dial is frequently not the address the
// listener is bound to — a reverse proxy or a NAT sits in between.
func bridgeEndpoints(fallbackIP string) []BridgeEndpoint {
	ip := bracketIPv6(common.GetIpByAddr(cfg().DefaultString("bridge_addr", fallbackIP)))
	var out []BridgeEndpoint

	add := func(enabled bool, typ, ipKey, portKey string, defaultPort int) BridgeEndpoint {
		if !enabled {
			return BridgeEndpoint{}
		}
		e := BridgeEndpoint{
			Type: typ,
			IP:   bracketIPv6(cfg().DefaultString(ipKey, ip)),
			Port: cfg().DefaultString(portKey, strconv.Itoa(defaultPort)),
		}
		e.Addr = e.IP + ":" + e.Port
		out = append(out, e)
		return e
	}

	add(showBridge("bridge_tcp_show", bridge.ServerTcpEnable), "tcp",
		"bridge_tcp_show_ip", "bridge_tcp_show_port", connection.BridgeTcpPort)
	add(showBridge("bridge_kcp_show", bridge.ServerKcpEnable), "kcp",
		"bridge_kcp_show_ip", "bridge_kcp_show_port", connection.BridgeKcpPort)
	add(showBridge("bridge_tls_show", bridge.ServerTlsEnable), "tls",
		"bridge_tls_show_ip", "bridge_tls_show_port", connection.BridgeTlsPort)

	if showBridge("bridge_quic_show", bridge.ServerQuicEnable) {
		e := BridgeEndpoint{
			Type: "quic",
			IP:   bracketIPv6(cfg().DefaultString("bridge_quic_show_ip", ip)),
			Port: cfg().DefaultString("bridge_quic_show_port", strconv.Itoa(connection.BridgeQuicPort)),
			Alpn: cfg().DefaultString("bridge_quic_show_alpn", defaultQuicAlpn()),
		}
		e.Addr = e.IP + ":" + e.Port
		// "nps" is the default ALPN, so npc infers it; anything else has to be
		// spelled out in the -server= value.
		if e.Alpn != "" && e.Alpn != "nps" {
			e.Addr += "/" + e.Alpn
		}
		out = append(out, e)
	}

	// The websocket transports are only reachable when a path is configured;
	// without one the listener has nothing to match on.
	if wsPath := cfg().String("bridge_path"); wsPath != "" {
		path := cfg().DefaultString("bridge_show_path", wsPath)
		if showBridge("bridge_ws_show", bridge.ServerWsEnable) {
			e := BridgeEndpoint{
				Type: "ws",
				IP:   bracketIPv6(cfg().DefaultString("bridge_ws_show_ip", ip)),
				Port: cfg().DefaultString("bridge_ws_show_port", strconv.Itoa(connection.BridgeWsPort)),
				Path: path,
			}
			e.Addr = e.IP + ":" + e.Port + path
			out = append(out, e)
		}
		if showBridge("bridge_wss_show", bridge.ServerWssEnable) {
			e := BridgeEndpoint{
				Type: "wss",
				IP:   bracketIPv6(cfg().DefaultString("bridge_wss_show_ip", ip)),
				Port: cfg().DefaultString("bridge_wss_show_port", strconv.Itoa(connection.BridgeWssPort)),
				Path: path,
			}
			e.Addr = e.IP + ":" + e.Port + path
			out = append(out, e)
		}
	}

	return out
}

// preferredEndpoint picks the transport to show first. The order matches the
// old GetBestBridge: the encrypted transports come first, then plain TCP, then
// the fallbacks. When nothing is advertised, fall back to the generic bridge
// port so the page still shows a usable command.
func preferredEndpoint(list []BridgeEndpoint, fallbackIP string) BridgeEndpoint {
	for _, typ := range []string{"tls", "quic", "wss", "tcp", "kcp", "ws"} {
		for _, e := range list {
			if e.Type == typ {
				return e
			}
		}
	}
	ip := bracketIPv6(common.GetIpByAddr(cfg().DefaultString("bridge_addr", fallbackIP)))
	typ := cfg().String("bridge_type")
	if typ == "both" || typ == "" {
		typ = "tcp"
	}
	port := strconv.Itoa(connection.BridgePort)
	return BridgeEndpoint{Type: typ, IP: ip, Port: port, Addr: ip + ":" + port}
}

// showBridge reports whether a transport should be advertised: explicitly via
// bridge_<t>_show, or implicitly because its listener is running.
func showBridge(key string, running bool) bool {
	return cfg().DefaultBool(key, running)
}

func defaultQuicAlpn() string {
	if len(connection.QuicAlpn) == 0 {
		return "nps"
	}
	return connection.QuicAlpn[0]
}

// bracketIPv6 wraps a bare IPv6 literal in brackets so that appending ":port"
// produces a dialable address. An already-bracketed value is left alone.
func bracketIPv6(ip string) string {
	if !strings.Contains(ip, ":") {
		return ip
	}
	if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
		return ip
	}
	return "[" + ip + "]"
}
