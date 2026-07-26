package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/logs"
)

// Middleware wraps a handler with a cross-cutting concern.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so that the first listed runs outermost, which is
// the order they read in at the call site.
func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// WithRequestID assigns each request a correlation id, echoed back in the
// response header and in the JSON envelope.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		if id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// WithRecover turns a panic in any handler into a 500 instead of tearing down
// the whole server. The admin API sits in the same process as the data plane,
// so a panic here must not take tunnels down with it.
func WithRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// A client disconnecting mid-write surfaces as this sentinel; it is
			// not a bug and must not be logged as one.
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			logs.Error("api: panic in %s %s (request %s): %v\n%s",
				r.Method, r.URL.Path, RequestID(r), rec, debug.Stack())
			Fail(w, r, http.StatusInternalServerError, CodeInternal, "internal error")
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code so the access log can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer, keeping
// Flush and Hijack usable through the wrapper.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// WithAccessLog records one debug line per API call. It stays at debug level
// because a busy dashboard polls several endpoints per second.
func WithAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logs.Debug("api: %s %s %d %s %s",
			r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond), ClientIP(r))
	})
}

// WithNoStore marks API responses as uncacheable. Dashboard and list data is
// live state, and a proxy caching a permission-scoped response would serve one
// user's data to another.
func WithNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// maxJSONBody caps request bodies for the general API. Login has its own,
// tighter limit (login_max_body) enforced separately.
const maxJSONBody = 1 << 20 // 1 MiB

// DecodeJSON reads a JSON request body into dst with the protections the API
// needs everywhere: a size cap, rejection of unknown fields so a typo in a
// client is a loud error rather than a silently ignored setting, and rejection
// of trailing content so a body cannot smuggle a second document.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mt, _, _ := strings.Cut(ct, ";"); !strings.EqualFold(strings.TrimSpace(mt), "application/json") {
			return errors.New("content-type must be application/json")
		}
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errors.New("request body too large")
		}
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}

// ClientIP resolves the caller's address, honouring a forwarding header only
// when the immediate peer is a configured trusted proxy. Trusting the header
// unconditionally would let anyone spoof their way past the login ban list.
func ClientIP(r *http.Request) string {
	peer := common.GetIpByAddr(r.RemoteAddr)
	if !allowXRealIP() || !common.IsTrustedProxy(trustedProxyIPs(), peer) {
		return peer
	}
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
		return common.GetIpByAddr(v)
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// The left-most entry is the original client; the rest are proxies.
		if first, _, _ := strings.Cut(v, ","); strings.TrimSpace(first) != "" {
			return common.GetIpByAddr(strings.TrimSpace(first))
		}
	}
	return peer
}
