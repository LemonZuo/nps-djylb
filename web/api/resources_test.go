package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/djylb/nps/bridge"
	"github.com/djylb/nps/lib/file"
	"github.com/djylb/nps/server"
)

// The resource handlers call into server.GetClientList / GetTunnel /
// GetHostList, all of which consult server.Bridge for liveness. In the real
// process that is wired up by StartNewServer; tests get a bare instance.
var bridgeOnce sync.Once

func setupResources(t *testing.T, extraConf string) *Router {
	t.Helper()
	bridgeOnce.Do(func() {
		server.Bridge = bridge.NewTunnel(false, &server.RunList, 60)
	})
	loadConfig(t, adminConfig+extraConf)
	useTestKey(t)
	return NewRouter(time.Now())
}

// tokenFor issues a bearer token for the given principal.
func tokenFor(t *testing.T, p Principal) string {
	t.Helper()
	token, _, err := IssueToken(p)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	return token
}

func adminToken(t *testing.T) string {
	return tokenFor(t, Principal{Username: "admin", IsAdmin: true})
}

func userToken(t *testing.T, clientID int) string {
	return tokenFor(t, Principal{Username: "user", ClientID: clientID})
}

// doJSON performs an authenticated request with an optional JSON body.
func doJSON(t *testing.T, rt *Router, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	return rec
}

// decodeData unmarshals the envelope's data field into T.
func decodeData[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode data from %q: %v", rec.Body.String(), err)
	}
	return out.Data
}

// listOf unmarshals the envelope's data as a paged list of T.
func listOf[T any](t *testing.T, rec *httptest.ResponseRecorder) (rows []T, total int64) {
	t.Helper()
	out := decodeData[struct {
		Rows  []T   `json:"rows"`
		Total int64 `json:"total"`
	}](t, rec)
	return out.Rows, out.Total
}

// storeClient inserts a ready-made client and removes it (and anything created
// under it) when the test ends.
func storeClient(t *testing.T, id int, remark string) *file.Client {
	t.Helper()
	c := &file.Client{
		Id:        id,
		VerifyKey: "test-vkey-" + remark,
		Remark:    remark,
		Status:    true,
		Cnf:       &file.Config{},
		Flow:      &file.Flow{},
	}
	file.GetDb().JsonDb.Clients.Store(c.Id, c)
	t.Cleanup(func() {
		file.GetDb().JsonDb.Clients.Delete(c.Id)
		removeOwnedRecords(c.Id)
	})
	return c
}

// removeOwnedRecords deletes tunnels and hosts belonging to a test client, so
// state cannot leak between tests through the shared JSON DB.
func removeOwnedRecords(clientID int) {
	file.GetDb().JsonDb.Tasks.Range(func(key, value any) bool {
		if tn, ok := value.(*file.Tunnel); ok && tn.Client != nil && tn.Client.Id == clientID {
			_ = file.GetDb().DelTask(tn.Id)
			server.RunList.Delete(tn.Id)
		}
		return true
	})
	file.GetDb().JsonDb.Hosts.Range(func(key, value any) bool {
		if h, ok := value.(*file.Host); ok && h.Client != nil && h.Client.Id == clientID {
			_ = file.GetDb().DelHost(h.Id)
		}
		return true
	})
}

// --- clients ---

func TestClientListIsScopedToTheCaller(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9101, "mine")
	storeClient(t, 9102, "other")

	rows, _ := listOf[ClientView](t, doJSON(t, rt, http.MethodGet, "/api/v1/clients?limit=100", userToken(t, 9101), nil))
	for _, c := range rows {
		if c.ID != 9101 {
			t.Errorf("user sees client %d", c.ID)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("user sees %d clients, want exactly their own", len(rows))
	}

	// The clientId parameter must not widen a user's scope.
	rows, _ = listOf[ClientView](t, doJSON(t, rt, http.MethodGet, "/api/v1/clients?clientId=9102", userToken(t, 9101), nil))
	if len(rows) != 1 || rows[0].ID != 9101 {
		t.Errorf("clientId query parameter changed a user's scope: %+v", rows)
	}

	rec := doJSON(t, rt, http.MethodGet, "/api/v1/clients?limit=500", adminToken(t), nil)
	rows, _ = listOf[ClientView](t, rec)
	seen := map[int]bool{}
	for _, c := range rows {
		seen[c.ID] = true
	}
	if !seen[9101] || !seen[9102] {
		t.Errorf("admin list misses test clients: %v", seen)
	}
}

func TestUserCannotReadAForeignClient(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9111, "mine")
	storeClient(t, 9112, "other")

	if rec := doJSON(t, rt, http.MethodGet, "/api/v1/clients/9112", userToken(t, 9111), nil); rec.Code != http.StatusNotFound {
		t.Errorf("foreign client read: status = %d, want 404", rec.Code)
	}
	// A nonexistent id answers identically, so ids cannot be enumerated.
	if rec := doJSON(t, rt, http.MethodGet, "/api/v1/clients/999999", userToken(t, 9111), nil); rec.Code != http.StatusNotFound {
		t.Errorf("nonexistent client read: status = %d, want 404", rec.Code)
	}
	if rec := doJSON(t, rt, http.MethodGet, "/api/v1/clients/9111", userToken(t, 9111), nil); rec.Code != http.StatusOK {
		t.Errorf("own client read: status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
}

func TestClientCreateRequiresAdmin(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9121, "mine")

	remark := "created-by-user"
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/clients", userToken(t, 9121), ClientRequest{Remark: &remark})
	if rec.Code != http.StatusForbidden {
		t.Errorf("user create: status = %d, want 403", rec.Code)
	}
}

func TestAdminCreatesUpdatesAndDeletesAClient(t *testing.T) {
	rt := setupResources(t, "")
	admin := adminToken(t)

	remark := "api-created"
	rateLimit := 256
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/clients", admin, ClientRequest{Remark: &remark, RateLimit: &rateLimit})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: status = %d; body = %s", rec.Code, rec.Body)
	}
	created := decodeData[ClientView](t, rec)
	if created.ID <= 0 {
		t.Fatalf("created client has id %d", created.ID)
	}
	t.Cleanup(func() { file.GetDb().JsonDb.Clients.Delete(created.ID) })
	if created.VerifyKey == "" {
		t.Error("no vkey was generated")
	}
	if created.RateLimit != 256 {
		t.Errorf("rateLimit = %d, want 256", created.RateLimit)
	}

	newRemark := "api-renamed"
	rec = doJSON(t, rt, http.MethodPut, "/api/v1/clients/"+itoa(created.ID), admin, ClientRequest{Remark: &newRemark})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d; body = %s", rec.Code, rec.Body)
	}
	updated := decodeData[ClientView](t, rec)
	if updated.Remark != "api-renamed" {
		t.Errorf("remark = %q after update", updated.Remark)
	}
	if updated.RateLimit != 256 {
		t.Errorf("update without rateLimit reset it to %d", updated.RateLimit)
	}

	if rec = doJSON(t, rt, http.MethodDelete, "/api/v1/clients/"+itoa(created.ID), admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: status = %d; body = %s", rec.Code, rec.Body)
	}
	if _, err := file.GetDb().GetClient(created.ID); err == nil {
		t.Error("client still exists after delete")
	}
}

func TestUserEditCannotTouchAdminFields(t *testing.T) {
	rt := setupResources(t, "")
	c := storeClient(t, 9131, "quota-test")
	c.RateLimit = 512

	remark := "user-remark"
	rate := 99999
	rec := doJSON(t, rt, http.MethodPut, "/api/v1/clients/9131", userToken(t, 9131), ClientRequest{Remark: &remark, RateLimit: &rate})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body)
	}
	view := decodeData[ClientView](t, rec)
	if view.Remark != "user-remark" {
		t.Errorf("remark = %q, the user's own field was not applied", view.Remark)
	}
	if view.RateLimit != 512 {
		t.Errorf("rateLimit = %d, a user changed an operator quota", view.RateLimit)
	}
}

func TestVkeyChangeRequiresAdmin(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9141, "vkey-test")

	vkey := "user-chosen-vkey"
	rec := doJSON(t, rt, http.MethodPut, "/api/v1/clients/9141", userToken(t, 9141), ClientRequest{VerifyKey: &vkey})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body = %s", rec.Code, rec.Body)
	}
}

func TestClientClearFlowCascades(t *testing.T) {
	rt := setupResources(t, "")
	c := storeClient(t, 9151, "flow-test")
	c.Flow.InletFlow = 1000
	c.Flow.ExportFlow = 2000

	tn := &file.Tunnel{
		Id:     97001,
		Mode:   "secret",
		Client: c,
		Flow:   &file.Flow{InletFlow: 500, ExportFlow: 700},
		Target: &file.Target{},
	}
	file.GetDb().JsonDb.Tasks.Store(tn.Id, tn)
	t.Cleanup(func() { file.GetDb().JsonDb.Tasks.Delete(tn.Id) })

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/clients/9151/clear", adminToken(t), ClearRequest{Mode: "flow"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body)
	}
	if c.Flow.InletFlow != 0 || c.Flow.ExportFlow != 0 {
		t.Errorf("client flow not cleared: %+v", c.Flow)
	}
	if tn.Flow.InletFlow != 0 || tn.Flow.ExportFlow != 0 {
		t.Errorf("owned tunnel flow not cleared: %+v", tn.Flow)
	}
}

// --- tunnels ---

func TestTunnelLifecycle(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9201, "tunnel-owner")
	admin := adminToken(t)

	mode := "secret"
	clientID := 9201
	remark := "api-tunnel"
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/tunnels", admin, TunnelRequest{Mode: &mode, ClientID: &clientID, Remark: &remark})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: status = %d; body = %s", rec.Code, rec.Body)
	}
	created := decodeData[TunnelView](t, rec)
	if created.ID <= 0 {
		t.Fatalf("created tunnel id = %d", created.ID)
	}
	if created.Password == "" {
		t.Error("secret tunnel got no generated password")
	}
	id := itoa(created.ID)

	rows, _ := listOf[TunnelView](t, doJSON(t, rt, http.MethodGet, "/api/v1/tunnels?type=secret&limit=100", admin, nil))
	found := false
	for _, v := range rows {
		if v.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Error("created tunnel missing from the list")
	}

	newRemark := "api-tunnel-renamed"
	rec = doJSON(t, rt, http.MethodPut, "/api/v1/tunnels/"+id, admin, TunnelRequest{Remark: &newRemark})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d; body = %s", rec.Code, rec.Body)
	}
	if got := decodeData[TunnelView](t, rec); got.Remark != newRemark {
		t.Errorf("remark = %q after update", got.Remark)
	}

	if rec = doJSON(t, rt, http.MethodPost, "/api/v1/tunnels/"+id+"/stop", admin, nil); rec.Code != http.StatusOK {
		t.Errorf("stop: status = %d; body = %s", rec.Code, rec.Body)
	}
	if rec = doJSON(t, rt, http.MethodPost, "/api/v1/tunnels/"+id+"/start", admin, nil); rec.Code != http.StatusOK {
		t.Errorf("start: status = %d; body = %s", rec.Code, rec.Body)
	}

	if rec = doJSON(t, rt, http.MethodDelete, "/api/v1/tunnels/"+id, admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: status = %d; body = %s", rec.Code, rec.Body)
	}
	if _, err := file.GetDb().GetTask(created.ID); err == nil {
		t.Error("tunnel still exists after delete")
	}
}

func TestUserCannotSetABridgeTarget(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9211, "bridge-test")

	mode := "secret"
	target := "bridge://5/127.0.0.1:22"
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/tunnels", userToken(t, 9211), TunnelRequest{Mode: &mode, Target: &target})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: status = %d; body = %s", rec.Code, rec.Body)
	}
	created := decodeData[TunnelView](t, rec)
	if created.Target.Target != "" {
		t.Errorf("target = %q, a user planted a bridge:// target", created.Target.Target)
	}
}

func TestUserCannotReachAForeignTunnel(t *testing.T) {
	rt := setupResources(t, "")
	owner := storeClient(t, 9221, "owner")
	storeClient(t, 9222, "intruder")

	tn := &file.Tunnel{
		Id:     97002,
		Mode:   "secret",
		Client: owner,
		Flow:   &file.Flow{},
		Target: &file.Target{},
	}
	file.GetDb().JsonDb.Tasks.Store(tn.Id, tn)
	t.Cleanup(func() { file.GetDb().JsonDb.Tasks.Delete(tn.Id) })

	if rec := doJSON(t, rt, http.MethodGet, "/api/v1/tunnels/97002", userToken(t, 9222), nil); rec.Code != http.StatusNotFound {
		t.Errorf("foreign tunnel read: status = %d, want 404", rec.Code)
	}
	if rec := doJSON(t, rt, http.MethodDelete, "/api/v1/tunnels/97002", userToken(t, 9222), nil); rec.Code != http.StatusNotFound {
		t.Errorf("foreign tunnel delete: status = %d, want 404", rec.Code)
	}
}

// --- hosts ---

func TestHostLifecycle(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9301, "host-owner")
	admin := adminToken(t)

	hostName := "api-test.example.com"
	clientID := 9301
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/hosts", admin, HostRequest{Host: &hostName, ClientID: &clientID})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: status = %d; body = %s", rec.Code, rec.Body)
	}
	created := decodeData[HostView](t, rec)
	if created.ID <= 0 {
		t.Fatalf("created host id = %d", created.ID)
	}
	id := itoa(created.ID)

	// The same routing triple again must be refused.
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/hosts", admin, HostRequest{Host: &hostName, ClientID: &clientID})
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate host: status = %d, want 409; body = %s", rec.Code, rec.Body)
	}

	renamed := "api-test-renamed.example.com"
	rec = doJSON(t, rt, http.MethodPut, "/api/v1/hosts/"+id, admin, HostRequest{Host: &renamed})
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: status = %d; body = %s", rec.Code, rec.Body)
	}
	if got := decodeData[HostView](t, rec); got.Host != renamed {
		t.Errorf("host = %q after rename", got.Host)
	}
	// The index must follow the rename or the proxy would keep routing the old
	// name here.
	if h, err := file.GetDb().GetInfoByHost(renamed, httptest.NewRequest(http.MethodGet, "/", nil)); err != nil || h.Id != created.ID {
		t.Errorf("renamed host not routable: %v", err)
	}

	rec = doJSON(t, rt, http.MethodPost, "/api/v1/hosts/"+id+"/toggle", admin, HostToggleRequest{Name: "auto_https", Action: "start"})
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle: status = %d; body = %s", rec.Code, rec.Body)
	}
	if h, _ := file.GetDb().GetHostById(created.ID); h == nil || !h.AutoHttps {
		t.Error("auto_https was not switched on")
	}

	if rec = doJSON(t, rt, http.MethodPost, "/api/v1/hosts/"+id+"/stop", admin, nil); rec.Code != http.StatusOK {
		t.Errorf("stop: status = %d", rec.Code)
	}
	if h, _ := file.GetDb().GetHostById(created.ID); h == nil || !h.IsClose {
		t.Error("host was not closed")
	}

	if rec = doJSON(t, rt, http.MethodDelete, "/api/v1/hosts/"+id, admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: status = %d; body = %s", rec.Code, rec.Body)
	}
	if _, err := file.GetDb().GetHostById(created.ID); err == nil {
		t.Error("host still exists after delete")
	}
}

func TestHostKeyFileIsRedactedForUsers(t *testing.T) {
	rt := setupResources(t, "")
	storeClient(t, 9311, "cert-owner")
	admin := adminToken(t)

	hostName := "cert-test.example.com"
	clientID := 9311
	keyPEM := "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----"
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/hosts", admin, HostRequest{Host: &hostName, ClientID: &clientID, KeyFile: &keyPEM})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: status = %d; body = %s", rec.Code, rec.Body)
	}
	created := decodeData[HostView](t, rec)
	if created.KeyFile == "" {
		t.Error("admin does not see the key they just stored")
	}

	rec = doJSON(t, rt, http.MethodGet, "/api/v1/hosts/"+itoa(created.ID), userToken(t, 9311), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user read: status = %d; body = %s", rec.Code, rec.Body)
	}
	if view := decodeData[HostView](t, rec); view.KeyFile != "" {
		t.Error("the private key leaked to a non-admin")
	}
}

// --- global ---

func TestGlobalBlacklistRoundTrip(t *testing.T) {
	rt := setupResources(t, "")
	admin := adminToken(t)

	if rec := doJSON(t, rt, http.MethodGet, "/api/v1/global", userToken(t, 9401), nil); rec.Code != http.StatusForbidden {
		t.Errorf("user access to global: status = %d, want 403", rec.Code)
	}

	rec := doJSON(t, rt, http.MethodPut, "/api/v1/global", admin, GlobalRequest{
		BlackIPList: []string{"10.0.0.1", "", "10.0.0.2", "10.0.0.1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d; body = %s", rec.Code, rec.Body)
	}
	saved := decodeData[GlobalView](t, rec)
	if len(saved.BlackIPList) != 2 {
		t.Errorf("blacklist = %v, want deduplicated pair", saved.BlackIPList)
	}

	rec = doJSON(t, rt, http.MethodGet, "/api/v1/global", admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status = %d; body = %s", rec.Code, rec.Body)
	}
	got := decodeData[GlobalView](t, rec)
	if len(got.BlackIPList) != 2 || got.BlackIPList[0] != "10.0.0.1" || got.BlackIPList[1] != "10.0.0.2" {
		t.Errorf("blacklist after reload = %v", got.BlackIPList)
	}
}

// --- meta ---

func TestBootstrapDescribesTheServer(t *testing.T) {
	rt := setupResources(t, "bridge_port=8883\nbridge_type=tcp\n")

	if rec := doJSON(t, rt, http.MethodGet, "/api/v1/meta/bootstrap", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous bootstrap: status = %d, want 401", rec.Code)
	}

	rec := doJSON(t, rt, http.MethodGet, "/api/v1/meta/bootstrap", adminToken(t), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body)
	}
	boot := decodeData[BootstrapResponse](t, rec)
	if boot.Version == "" {
		t.Error("no server version reported")
	}
}

func itoa(v int) string { return strconv.Itoa(v) }
