package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type payload struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

func decodeBody(t *testing.T, body, contentType string) (payload, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	var dst payload
	err := DecodeJSON(httptest.NewRecorder(), req, &dst)
	return dst, err
}

func TestDecodeJSON(t *testing.T) {
	got, err := decodeBody(t, `{"name":"web","port":8080}`, "application/json")
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.Name != "web" || got.Port != 8080 {
		t.Errorf("decoded = %+v, want {web 8080}", got)
	}
}

func TestDecodeJSONAcceptsCharsetParameter(t *testing.T) {
	if _, err := decodeBody(t, `{"name":"a"}`, "application/json; charset=utf-8"); err != nil {
		t.Errorf("DecodeJSON with a charset parameter: %v", err)
	}
}

func TestDecodeJSONRejectsWrongContentType(t *testing.T) {
	if _, err := decodeBody(t, `{"name":"a"}`, "text/plain"); err == nil {
		t.Error("DecodeJSON accepted a non-JSON Content-Type")
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	// A typo'd field silently ignored would let a user believe they configured
	// something they did not.
	if _, err := decodeBody(t, `{"name":"a","prot":80}`, "application/json"); err == nil {
		t.Error("DecodeJSON accepted an unknown field")
	}
}

func TestDecodeJSONRejectsTrailingContent(t *testing.T) {
	if _, err := decodeBody(t, `{"name":"a"}{"name":"b"}`, "application/json"); err == nil {
		t.Error("DecodeJSON accepted a second JSON document")
	}
}

func TestDecodeJSONRejectsOversizeBody(t *testing.T) {
	big := `{"name":"` + strings.Repeat("x", 2<<20) + `"}`
	_, err := decodeBody(t, big, "application/json")
	if err == nil {
		t.Fatal("DecodeJSON accepted a body over the size cap")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want a size-related message", err)
	}
}

func TestClientIPIgnoresHeadersFromUntrustedPeers(t *testing.T) {
	// allow_x_real_ip defaults to false, so a spoofed header must be ignored:
	// honouring it would let an attacker evade the login ban list at will.
	loadConfig(t, "appname=nps\n")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:5000"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := ClientIP(req); got != "203.0.113.9" {
		t.Errorf("ClientIP = %q, want the peer address %q", got, "203.0.113.9")
	}
}

func TestClientIPHonoursHeadersFromTrustedProxy(t *testing.T) {
	loadConfig(t, "appname=nps\nallow_x_real_ip=true\ntrusted_proxy_ips=127.0.0.1\n")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	if got := ClientIP(req); got != "1.2.3.4" {
		t.Errorf("ClientIP = %q, want %q", got, "1.2.3.4")
	}

	// With only X-Forwarded-For, the left-most entry is the original client.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("X-Forwarded-For", "5.6.7.8, 10.0.0.1")
	if got := ClientIP(req); got != "5.6.7.8" {
		t.Errorf("ClientIP = %q, want %q", got, "5.6.7.8")
	}
}

func TestClientIPRejectsUntrustedProxyEvenWhenEnabled(t *testing.T) {
	loadConfig(t, "appname=nps\nallow_x_real_ip=true\ntrusted_proxy_ips=127.0.0.1\n")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:5000"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	if got := ClientIP(req); got != "203.0.113.9" {
		t.Errorf("ClientIP = %q, want the peer address %q", got, "203.0.113.9")
	}
}
