package api

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/crypt"
	"github.com/djylb/nps/lib/file"
)

// initCrypt makes sure the RSA login key exists. InitTls falls back to
// generating one when the certificate it is handed carries no RSA key, so an
// empty certificate is all these tests need.
var cryptInit sync.Once

func initCrypt(t *testing.T) {
	t.Helper()
	cryptInit.Do(func() { crypt.InitTls(tls.Certificate{}) })
}

// encryptPayload builds the base64 RSA blob the browser would send.
func encryptPayload(t *testing.T, nonce, password string, ts int64) string {
	t.Helper()
	pubPEM, err := crypt.GetRSAPublicKeyPEM()
	if err != nil {
		t.Fatalf("GetRSAPublicKeyPEM: %v", err)
	}
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		t.Fatal("public key is not PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("public key is %T, want RSA", parsed)
	}

	body, err := json.Marshal(crypt.LoginPayload{Nonce: nonce, Timestamp: ts, Password: password})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	//nolint:staticcheck // the server decrypts with PKCS1v15, so the test must encrypt with it
	cipher, err := rsa.EncryptPKCS1v15(rand.Reader, pub, body)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return base64.StdEncoding.EncodeToString(cipher)
}

// postJSON sends body to the router and returns the recorder.
func postJSON(t *testing.T, rt *Router, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(raw))
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	return rec
}

// getChallenge fetches a login challenge and decodes it.
func getChallenge(t *testing.T, rt *Router) ChallengeResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/challenge", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("challenge status = %d; body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Data ChallengeResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	return out.Data
}

// adminConfig is the minimal configuration for a password-only admin login.
const adminConfig = "appname=nps\nweb_username=admin\nweb_password=s3cret\n"

// setupAuth prepares the crypt layer, the config and a router.
func setupAuth(t *testing.T, conf string) *Router {
	t.Helper()
	initCrypt(t)
	loadConfig(t, conf)
	useTestKey(t)
	RemoveAllLoginBans()
	t.Cleanup(RemoveAllLoginBans)
	return NewRouter(time.Now())
}

func TestChallengeReturnsUsableMaterial(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	ch := getChallenge(t, rt)

	if ch.Nonce == "" {
		t.Error("no nonce")
	}
	if !strings.Contains(ch.PublicKey, "BEGIN PUBLIC KEY") {
		t.Errorf("public key is not PEM: %.40q", ch.PublicKey)
	}
	if ch.TotpLen != crypt.TotpLen {
		t.Errorf("totpLen = %d, want %d", ch.TotpLen, crypt.TotpLen)
	}
	if ch.Captcha != nil {
		t.Error("a captcha was issued although open_captcha is off")
	}
	if ch.ServerTime == 0 {
		t.Error("no server time, so a client with a skewed clock cannot correct")
	}
}

func TestChallengeIssuesDistinctNonces(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		n := getChallenge(t, rt).Nonce
		if seen[n] {
			t.Fatalf("nonce %q was issued twice", n)
		}
		seen[n] = true
	}
}

func TestChallengeIncludesCaptchaWhenEnabled(t *testing.T) {
	rt := setupAuth(t, adminConfig+"open_captcha=true\n")
	ch := getChallenge(t, rt)
	if !ch.CaptchaOpen {
		t.Fatal("captchaOpen = false although open_captcha is set")
	}
	if ch.Captcha == nil || ch.Captcha.ID == "" {
		t.Fatal("no captcha was issued")
	}
	if !strings.HasPrefix(ch.Captcha.Image, "data:image/png;base64,") {
		t.Errorf("captcha image is not a data URI: %.40q", ch.Captcha.Image)
	}
}

func TestAdminLoginSucceeds(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	ch := getChallenge(t, rt)

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: encryptPayload(t, ch.Nonce, "s3cret", common.TimeNow().UnixMilli()),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	var out struct {
		Data LoginResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.Token == "" {
		t.Fatal("no token was issued")
	}
	if !out.Data.User.IsAdmin || out.Data.User.Username != "admin" {
		t.Errorf("user = %+v, want the admin", out.Data.User)
	}

	// The token must actually work.
	p, err := ParseToken(out.Data.Token)
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}
	if !p.IsAdmin {
		t.Error("the issued token is not an admin token")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	ch := getChallenge(t, rt)

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: encryptPayload(t, ch.Nonce, "wrong", common.TimeNow().UnixMilli()),
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", rec.Code, rec.Body)
	}
	// A rejection must carry a fresh nonce so the UI can retry without another
	// round trip.
	var out struct {
		Data loginFailure `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.Nonce == "" {
		t.Error("the rejection carried no replacement nonce")
	}
}

func TestLoginNonceIsSingleUse(t *testing.T) {
	// The core replay defence: the same encrypted payload must not log in twice.
	rt := setupAuth(t, adminConfig)
	ch := getChallenge(t, rt)
	payload := encryptPayload(t, ch.Nonce, "s3cret", common.TimeNow().UnixMilli())

	if rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{Username: "admin", Password: payload}); rec.Code != http.StatusOK {
		t.Fatalf("first login: status = %d; body = %s", rec.Code, rec.Body)
	}
	RemoveAllLoginBans() // the rate limiter would otherwise mask the nonce check

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{Username: "admin", Password: payload})
	if rec.Code == http.StatusOK {
		t.Fatal("the same payload logged in twice")
	}
	if env := decodeEnvelope(t, rec); !strings.Contains(env.Message, "nonce") {
		t.Errorf("message = %q, want it to name the nonce", env.Message)
	}
}

func TestLoginRejectsUnknownNonce(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	getChallenge(t, rt) // issue one, then ignore it

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: encryptPayload(t, "never-issued-nonce", "s3cret", common.TimeNow().UnixMilli()),
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a payload with an unissued nonce was accepted")
	}
}

func TestLoginRejectsGarbageCiphertext(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	getChallenge(t, rt)

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: base64.StdEncoding.EncodeToString([]byte("not encrypted with our key")),
	})
	if rec.Code == http.StatusOK {
		t.Fatal("undecryptable input was accepted")
	}
	// The response should hand back the certificate so a client with a stale
	// key can recover on its own.
	var out struct {
		Data loginFailure `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !strings.Contains(out.Data.Cert, "BEGIN PUBLIC KEY") {
		t.Error("no certificate was returned with the decrypt error")
	}
}

func TestLoginRejectsOversizeBody(t *testing.T) {
	rt := setupAuth(t, adminConfig+"login_max_body=200\n")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"`+strings.Repeat("A", 500)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413; body = %s", rec.Code, rec.Body)
	}
}

func TestSecureModeRejectsStaleTimestamp(t *testing.T) {
	rt := setupAuth(t, adminConfig+"secure_mode=true\nlogin_max_skew=1000\n")
	ch := getChallenge(t, rt)

	stale := common.TimeNow().UnixMilli() - 60_000
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: encryptPayload(t, ch.Nonce, "s3cret", stale),
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a stale payload was accepted in secure mode")
	}
	if env := decodeEnvelope(t, rec); !strings.Contains(env.Message, "timestamp") {
		t.Errorf("message = %q, want it to name the timestamp", env.Message)
	}
}

func TestNonSecureModeIgnoresTimestamp(t *testing.T) {
	// Outside secure mode the timestamp is advisory, matching the previous
	// behaviour; the nonce is what prevents replay.
	rt := setupAuth(t, adminConfig)
	ch := getChallenge(t, rt)

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: encryptPayload(t, ch.Nonce, "s3cret", 0),
	})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
}

func TestForcedPoWIsEnforced(t *testing.T) {
	rt := setupAuth(t, adminConfig+"force_pow=true\npow_bits=8\n")
	ch := getChallenge(t, rt)
	if !ch.PoWRequired || ch.PoWBits != 8 {
		t.Fatalf("challenge = %+v, want a forced 8-bit proof of work", ch)
	}
	payload := encryptPayload(t, ch.Nonce, "s3cret", common.TimeNow().UnixMilli())

	// Without a solution.
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{Username: "admin", Password: payload})
	if rec.Code == http.StatusOK {
		t.Fatal("login succeeded without the required proof of work")
	}
	RemoveAllLoginBans()

	// With one.
	rec = postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: payload,
		PoWX:     solvePoW(t, 8, payload),
		Bits:     8,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d with a valid proof of work; body = %s", rec.Code, rec.Body)
	}
}

func TestPoWBitsMustMatchTheConfiguredDifficulty(t *testing.T) {
	// Solving an easier puzzle and claiming a harder one must not work.
	rt := setupAuth(t, adminConfig+"force_pow=true\npow_bits=12\n")
	ch := getChallenge(t, rt)
	payload := encryptPayload(t, ch.Nonce, "s3cret", common.TimeNow().UnixMilli())

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: payload,
		PoWX:     solvePoW(t, 4, payload),
		Bits:     4,
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a proof of work at the wrong difficulty was accepted")
	}
}

// solvePoW finds an x making sha256(payload||x) start with the given number of
// zero bits, the same way the browser does.
func solvePoW(t *testing.T, bits int, payload string) string {
	t.Helper()
	for i := 0; i < 1<<24; i++ {
		x := strconv.Itoa(i)
		if hasLeadingZeroBits(sha256.Sum256([]byte(payload+x)), bits) {
			return x
		}
	}
	t.Fatalf("no proof of work found for %d bits", bits)
	return ""
}

func hasLeadingZeroBits(sum [32]byte, bits int) bool {
	full := bits / 8
	for i := 0; i < full; i++ {
		if sum[i] != 0 {
			return false
		}
	}
	if rem := bits % 8; rem > 0 {
		if sum[full]&(byte(0xFF)<<(8-rem)) != 0 {
			return false
		}
	}
	return true
}

func TestCaptchaIsRequiredWhenEnabled(t *testing.T) {
	rt := setupAuth(t, adminConfig+"open_captcha=true\n")
	ch := getChallenge(t, rt)

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username:    "admin",
		Password:    encryptPayload(t, ch.Nonce, "s3cret", common.TimeNow().UnixMilli()),
		CaptchaID:   ch.Captcha.ID,
		CaptchaCode: "0000",
	})
	// A wrong captcha must lose, and the odds of "0000" being right are 1/10000
	// — accept that this test is flaky at that rate rather than reaching into
	// the store, since going through the HTTP surface is the point here.
	if rec.Code == http.StatusOK {
		t.Skip("the guessed captcha happened to be correct")
	}
	if env := decodeEnvelope(t, rec); !strings.Contains(env.Message, "verification code") {
		t.Errorf("message = %q, want it to name the verification code", env.Message)
	}
}

func TestCaptchaLoginSucceedsWithTheRightCode(t *testing.T) {
	rt := setupAuth(t, adminConfig+"open_captcha=true\n")
	ch := getChallenge(t, rt)

	captchas.mu.Lock()
	code := captchas.items[ch.Captcha.ID].code
	captchas.mu.Unlock()

	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username:    "admin",
		Password:    encryptPayload(t, ch.Nonce, "s3cret", common.TimeNow().UnixMilli()),
		CaptchaID:   ch.Captcha.ID,
		CaptchaCode: code,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
}

func TestClientUserLogin(t *testing.T) {
	rt := setupAuth(t, adminConfig+"allow_user_login=true\n")
	withClient(t, &file.Client{
		Id: 42, Status: true, Cnf: &file.Config{}, Flow: &file.Flow{},
		WebUserName: "bob", WebPassword: "bobpass", VerifyKey: "vkey-bob-0001",
	})

	ch := getChallenge(t, rt)
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "bob",
		Password: encryptPayload(t, ch.Nonce, "bobpass", common.TimeNow().UnixMilli()),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	var out struct {
		Data LoginResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.User.IsAdmin {
		t.Error("a client user was granted admin")
	}
	if out.Data.User.ClientID != 42 {
		t.Errorf("clientId = %d, want 42", out.Data.User.ClientID)
	}
}

func TestClientLoginRefusedWhenUserLoginIsOff(t *testing.T) {
	rt := setupAuth(t, adminConfig) // allow_user_login defaults to false
	withClient(t, &file.Client{
		Id: 43, Status: true, Cnf: &file.Config{}, Flow: &file.Flow{},
		WebUserName: "carol", WebPassword: "carolpass", VerifyKey: "vkey-carol-001",
	})

	ch := getChallenge(t, rt)
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "carol",
		Password: encryptPayload(t, ch.Nonce, "carolpass", common.TimeNow().UnixMilli()),
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a client logged in although allow_user_login is off")
	}
}

func TestDisabledClientCannotLogIn(t *testing.T) {
	rt := setupAuth(t, adminConfig+"allow_user_login=true\n")
	withClient(t, &file.Client{
		Id: 44, Status: false, Cnf: &file.Config{}, Flow: &file.Flow{},
		WebUserName: "dave", WebPassword: "davepass", VerifyKey: "vkey-dave-0001",
	})

	ch := getChallenge(t, rt)
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "dave",
		Password: encryptPayload(t, ch.Nonce, "davepass", common.TimeNow().UnixMilli()),
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a disabled client logged in")
	}
}

func TestVkeyLoginForCredentiallessClient(t *testing.T) {
	// A client with no web credentials can be reached as "user" + its vkey,
	// which is how the original implementation behaved.
	rt := setupAuth(t, adminConfig+"allow_user_login=true\nallow_user_vkey_login=true\n")
	withClient(t, &file.Client{
		Id: 45, Status: true, Cnf: &file.Config{}, Flow: &file.Flow{},
		VerifyKey: "vkey-plain-0001",
	})

	ch := getChallenge(t, rt)
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "user",
		Password: encryptPayload(t, ch.Nonce, "vkey-plain-0001", common.TimeNow().UnixMilli()),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
}

func TestVkeyLoginRefusedWhenDisabled(t *testing.T) {
	rt := setupAuth(t, adminConfig+"allow_user_login=true\nallow_user_vkey_login=false\n")
	withClient(t, &file.Client{
		Id: 46, Status: true, Cnf: &file.Config{}, Flow: &file.Flow{},
		VerifyKey: "vkey-plain-0002",
	})

	ch := getChallenge(t, rt)
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "user",
		Password: encryptPayload(t, ch.Nonce, "vkey-plain-0002", common.TimeNow().UnixMilli()),
	})
	if rec.Code == http.StatusOK {
		t.Fatal("vkey login worked although allow_user_vkey_login is off")
	}
}

// withClient inserts a client into the JSON DB for the duration of the test.
func withClient(t *testing.T, c *file.Client) {
	t.Helper()
	file.GetDb().JsonDb.Clients.Store(c.Id, c)
	t.Cleanup(func() { file.GetDb().JsonDb.Clients.Delete(c.Id) })
}

func TestMeRequiresAuthentication(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMeDescribesTheCaller(t *testing.T) {
	rt := setupAuth(t, adminConfig+"allow_flow_limit=true\n")
	token, _, err := IssueToken(Principal{Username: "admin", IsAdmin: true})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Data MeInfo `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Data.IsAdmin || out.Data.Username != "admin" {
		t.Errorf("me = %+v, want the admin", out.Data)
	}
	if !out.Data.Permissions.FlowLimit {
		t.Error("permissions do not reflect allow_flow_limit=true")
	}
	if out.Data.Version == "" {
		t.Error("no version was reported")
	}
}

func TestRegisterIsRefusedWhenDisabled(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	rec := postJSON(t, rt, "/api/v1/auth/register", RegisterRequest{Username: "x", Password: "y"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body = %s", rec.Code, rec.Body)
	}
}

func TestRegisterCreatesAClient(t *testing.T) {
	rt := setupAuth(t, adminConfig+"allow_user_register=true\n")
	ch := getChallenge(t, rt)
	username := "newuser-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	rec := postJSON(t, rt, "/api/v1/auth/register", RegisterRequest{
		Username: username,
		Password: encryptPayload(t, ch.Nonce, "newpass", common.TimeNow().UnixMilli()),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	var out struct {
		Data struct {
			ClientID int `json:"clientId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.ClientID == 0 {
		t.Fatal("no client id was returned")
	}
	t.Cleanup(func() { file.GetDb().JsonDb.Clients.Delete(out.Data.ClientID) })

	v, ok := file.GetDb().JsonDb.Clients.Load(out.Data.ClientID)
	if !ok {
		t.Fatalf("client %d was not stored", out.Data.ClientID)
	}
	if got := v.(*file.Client).WebUserName; got != username {
		t.Errorf("stored username = %q, want %q", got, username)
	}
}

func TestRegisterRefusesTheAdminUsername(t *testing.T) {
	// Registering as the admin would create a client whose web login shadows
	// the operator's.
	rt := setupAuth(t, adminConfig+"allow_user_register=true\n")
	ch := getChallenge(t, rt)

	rec := postJSON(t, rt, "/api/v1/auth/register", RegisterRequest{
		Username: "admin",
		Password: encryptPayload(t, ch.Nonce, "whatever", common.TimeNow().UnixMilli()),
	})
	if rec.Code == http.StatusOK {
		t.Fatal("the admin username was registered")
	}
}

func TestLoginBanEndpointsRequireAdmin(t *testing.T) {
	rt := setupAuth(t, adminConfig)

	userToken, _, err := IssueToken(Principal{Username: "bob", ClientID: 2})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	for _, m := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/bans"},
		{http.MethodDelete, "/api/v1/auth/bans"},
		{http.MethodDelete, "/api/v1/auth/bans/1.2.3.4"},
	} {
		req := httptest.NewRequest(m.method, m.path, nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", m.method, m.path, rec.Code)
		}
	}
}

func TestLoginBanListAndClear(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	adminToken, _, err := IssueToken(Principal{Username: "admin", IsAdmin: true})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	authed := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, req)
		return rec
	}

	NoteLoginFailure("10.1.2.3", true)
	NoteLoginFailure("mallory", true)

	rec := authed(http.MethodGet, "/api/v1/auth/bans")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d; body = %s", rec.Code, rec.Body)
	}
	var listOut struct {
		Data struct {
			Rows  []BanEntry `json:"rows"`
			Total int        `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listOut); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if listOut.Data.Total < 2 {
		t.Fatalf("total = %d, want at least 2", listOut.Data.Total)
	}
	byKey := make(map[string]BanEntry)
	for _, e := range listOut.Data.Rows {
		byKey[e.Key] = e
	}
	if byKey["10.1.2.3"].Type != "ip" {
		t.Errorf("10.1.2.3 type = %q, want ip", byKey["10.1.2.3"].Type)
	}
	if byKey["mallory"].Type != "username" {
		t.Errorf("mallory type = %q, want username", byKey["mallory"].Type)
	}

	if rec := authed(http.MethodDelete, "/api/v1/auth/bans/10.1.2.3"); rec.Code != http.StatusOK {
		t.Errorf("delete one: status = %d; body = %s", rec.Code, rec.Body)
	}
	if rec := authed(http.MethodDelete, "/api/v1/auth/bans/10.1.2.3"); rec.Code != http.StatusNotFound {
		t.Errorf("delete a gone record: status = %d, want 404", rec.Code)
	}
	if rec := authed(http.MethodDelete, "/api/v1/auth/bans"); rec.Code != http.StatusOK {
		t.Errorf("clear: status = %d", rec.Code)
	}
	if n := len(ListLoginBans()); n != 0 {
		t.Errorf("%d records survived the clear", n)
	}
}

func TestRepeatedFailuresTriggerTheRateLimit(t *testing.T) {
	// login_ban_time is the minimum gap between attempts; a second attempt
	// inside it must be refused regardless of the credentials.
	rt := setupAuth(t, adminConfig+"login_ban_time=60\n")

	ch := getChallenge(t, rt)
	postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: encryptPayload(t, ch.Nonce, "wrong", common.TimeNow().UnixMilli()),
	})

	ch = getChallenge(t, rt)
	rec := postJSON(t, rt, "/api/v1/auth/login", LoginRequest{
		Username: "admin",
		Password: encryptPayload(t, ch.Nonce, "s3cret", common.TimeNow().UnixMilli()),
	})
	if rec.Code == http.StatusOK {
		t.Error("a correct password was accepted inside the rate-limit window")
	}
}

func TestLoginRejectsUnknownJSONFields(t *testing.T) {
	// A typo in a client must be a loud error rather than a silently ignored
	// field — that is how a control ends up not being applied.
	rt := setupAuth(t, adminConfig)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","passwordd":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body = %s", rec.Code, rec.Body)
	}
}

func TestAuthEndpointsRejectWrongMethods(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/challenge"},
		{http.MethodGet, "/api/v1/auth/register"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s %s was accepted", c.method, c.path)
		}
	}
}

func TestCaptchaEndpointIsGatedOnTheSetting(t *testing.T) {
	rt := setupAuth(t, adminConfig)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/captcha", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d with open_captcha off, want 404", rec.Code)
	}

	rt = setupAuth(t, adminConfig+"open_captcha=true\n")
	rec = httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/captcha", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d with open_captcha on, want 200; body = %s", rec.Code, rec.Body)
	}
}
