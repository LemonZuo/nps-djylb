package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/djylb/nps/bridge"
	"github.com/djylb/nps/lib/conn"
	"github.com/djylb/nps/lib/logs"
	"github.com/djylb/nps/server/connection"
	"github.com/djylb/nps/server/proxy"
	"github.com/djylb/nps/server/tool"
)

// webHandler serves the management interface. It is injected by the entry point
// rather than constructed here: the API handlers need to call into this package
// (task lists, dashboard data, the bridge), so this package must not import
// them back. cmd/nps wires the two together.
var (
	webHandlerMu sync.RWMutex
	webHandler   http.Handler
	webTLSConfig func() (enabled bool, certFile, keyFile string)
)

// SetWebHandler installs the management HTTP handler. It must be called before
// the web server task starts. tlsConfig reports whether the public listener
// should terminate TLS and with which key pair; it may be nil for plain HTTP.
func SetWebHandler(h http.Handler, tlsConfig func() (bool, string, string)) {
	webHandlerMu.Lock()
	defer webHandlerMu.Unlock()
	webHandler, webTLSConfig = h, tlsConfig
}

func getWebHandler() (http.Handler, func() (bool, string, string)) {
	webHandlerMu.RLock()
	defer webHandlerMu.RUnlock()
	return webHandler, webTLSConfig
}

type WebServer struct {
	proxy.BaseServer
	tcpListener net.Listener
	servers     []*http.Server
}

// webTimeouts guard the management listener against slow-client resource
// exhaustion. WriteTimeout is generous because the dashboard streams system
// metrics that can take a moment to collect on a loaded box.
const (
	webReadHeaderTimeout = 20 * time.Second
	webWriteTimeout      = 120 * time.Second
	webIdleTimeout       = 120 * time.Second
	webShutdownGrace     = 5 * time.Second
)

func (s *WebServer) Start() error {
	ip := connection.WebIp
	p := connection.WebPort

	handler, tlsConfig := getWebHandler()
	if handler == nil {
		return errors.New("web management handler was not installed before startup")
	}

	if tool.WebServerListener != nil {
		_ = tool.WebServerListener.Close()
		tool.WebServerListener = nil
	}
	lAddr := &net.TCPAddr{IP: net.ParseIP(ip), Port: p}
	tool.WebServerListener = conn.NewVirtualListener(lAddr)

	errCh := make(chan error, 2)

	// The virtual listener carries requests tunnelled in over the bridge, so it
	// is always served plain HTTP: the transport underneath already provides
	// whatever encryption the bridge type implies.
	virtual := s.newServer(handler)
	go func() { errCh <- virtual.Serve(tool.WebServerListener) }()

	if p > 0 {
		if l, err := connection.GetWebManagerListener(); err == nil {
			s.tcpListener = l
			public := s.newServer(handler)
			useTLS, certFile, keyFile := false, "", ""
			if tlsConfig != nil {
				useTLS, certFile, keyFile = tlsConfig()
			}
			go func() {
				if useTLS {
					errCh <- public.ServeTLS(l, certFile, keyFile)
				} else {
					errCh <- public.Serve(l)
				}
			}()
		} else {
			logs.Error("%v", err)
		}
	} else {
		logs.Info("web_port=0: only virtual listener is active (plain HTTP)")
	}

	err := <-errCh
	// Serve returns ErrServerClosed on an orderly Close, which is not a failure.
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *WebServer) newServer(h http.Handler) *http.Server {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: webReadHeaderTimeout,
		WriteTimeout:      webWriteTimeout,
		IdleTimeout:       webIdleTimeout,
		ErrorLog:          logs.NewStdLogger("web"),
	}
	s.servers = append(s.servers, srv)
	return srv
}

func (s *WebServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), webShutdownGrace)
	defer cancel()
	for _, srv := range s.servers {
		// Shutdown lets in-flight admin requests finish; Close is the fallback
		// once the grace period expires.
		if err := srv.Shutdown(ctx); err != nil {
			_ = srv.Close()
		}
	}
	s.servers = nil
	if s.tcpListener != nil {
		_ = s.tcpListener.Close()
		s.tcpListener = nil
	}
	if tool.WebServerListener != nil {
		_ = tool.WebServerListener.Close()
		tool.WebServerListener = nil
	}
	return nil
}

func NewWebServer(bridge *bridge.Bridge) *WebServer {
	s := new(WebServer)
	s.Bridge = bridge
	return s
}
