package api

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/crypt"
	"github.com/djylb/nps/lib/file"
	"github.com/djylb/nps/lib/logs"
	"github.com/djylb/nps/server"
)

// The login flow, carried over from the Beego controller with the session
// replaced by a JWT. Every other control is unchanged, and in the same order,
// because they are layered deliberately:
//
//  1. body size cap        — bound the work an anonymous request can cause
//  2. captcha              — cost a script cannot pay for free
//  3. proof of work        — cost that scales when an address looks abusive
//  4. RSA decryption       — the password never crosses the wire in the clear
//  5. nonce                — the ciphertext is good for exactly one request
//  6. timestamp            — bounded replay window in secure mode
//  7. credential check     — only now is the password compared
//
// A failure at any step feeds the ban tracker, and the response says as little
// as the frontend needs to recover.

// ChallengeResponse is what the SPA fetches before showing the login form. It
// carries everything the browser needs to construct a valid attempt.
type ChallengeResponse struct {
	// Nonce must be embedded in the encrypted payload.
	Nonce string `json:"nonce"`
	// PublicKey is the PEM the browser encrypts the password with.
	PublicKey string `json:"publicKey"`
	// PoWRequired says whether a proof of work is mandatory up front; the
	// server may still demand one in response to a failed attempt.
	PoWRequired bool `json:"powRequired"`
	// PoWBits is the difficulty, in leading zero bits.
	PoWBits int `json:"powBits"`
	// CaptchaOpen says whether a captcha must accompany the attempt.
	CaptchaOpen bool `json:"captchaOpen"`
	// Captcha is the challenge itself, present only when CaptchaOpen.
	Captcha *Challenge `json:"captcha,omitempty"`
	// TotpLen is how many digits a TOTP code has, so the browser can split it
	// off the end of the password field.
	TotpLen int `json:"totpLen"`
	// RegisterAllowed and VkeyLoginAllowed drive which controls to render.
	RegisterAllowed  bool `json:"registerAllowed"`
	UserLoginAllowed bool `json:"userLoginAllowed"`
	VkeyLoginAllowed bool `json:"vkeyLoginAllowed"`
	// LoginDelayMs is the minimum gap the server enforces between attempts, so
	// the UI can disable the button rather than let the user hit a 429.
	LoginDelayMs int64 `json:"loginDelayMs"`
	// ServerTime is the server's clock in Unix milliseconds. In secure mode the
	// payload timestamp is checked against it, and a browser with a skewed
	// clock would otherwise be locked out with no way to tell why.
	ServerTime int64 `json:"serverTime"`
}

// LoginRequest is the body of POST /auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	// Password is the base64 RSA ciphertext of {"n":nonce,"t":ms,"p":password}.
	Password string `json:"password"`
	// CaptchaID and CaptchaCode answer the challenge from /auth/challenge. The
	// code may have a TOTP appended to it, matching the old form's behaviour.
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
	// PoWX is the nonce that makes sha256(password||powX) start with PoWBits
	// zero bits; Bits echoes the difficulty solved for.
	PoWX string `json:"powX"`
	Bits int    `json:"bits"`
}

// LoginResponse is returned on success.
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
	User      MeInfo `json:"user"`
}

// loginFailure is the body of a rejected attempt. It always carries a fresh
// nonce so the client can retry without a second round trip, and optionally the
// extra material the client needs to satisfy whatever tripped it.
type loginFailure struct {
	Nonce     string     `json:"nonce"`
	Bits      int        `json:"bits,omitempty"`
	Cert      string     `json:"cert,omitempty"`
	Timestamp int64      `json:"timestamp,omitempty"`
	Captcha   *Challenge `json:"captcha,omitempty"`
}

// handleChallenge issues the material for one login attempt.
func handleChallenge(w http.ResponseWriter, r *http.Request) {
	nonce := loginNonces.Issue()
	if nonce == "" {
		TooManyRequests(w, r, "too many pending login attempts, please try again shortly")
		return
	}
	pub, err := crypt.GetRSAPublicKeyPEM()
	if err != nil {
		Internal(w, r, err)
		return
	}

	resp := ChallengeResponse{
		Nonce:            nonce,
		PublicKey:        pub,
		PoWRequired:      ForcePoW(),
		PoWBits:          PoWBits(),
		CaptchaOpen:      OpenCaptcha(),
		TotpLen:          crypt.TotpLen,
		RegisterAllowed:  AllowUserRegister(),
		UserLoginAllowed: AllowUserLogin(),
		VkeyLoginAllowed: AllowUserLogin() && AllowUserVkeyLogin(),
		LoginDelayMs:     LoginBanTime() * 1000,
		ServerTime:       common.TimeNow().UnixMilli(),
	}
	if resp.CaptchaOpen {
		c, err := NewCaptcha()
		if err != nil {
			Internal(w, r, err)
			return
		}
		resp.Captcha = c
	}
	Ok(w, r, resp)
}

// handleLogin authenticates and issues a token.
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > LoginMaxBody() {
		Fail(w, r, http.StatusRequestEntityTooLarge, CodeBadRequest, "payload too large")
		return
	}

	var req LoginRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	ip := ClientIP(r)
	ipBanned := IsLoginBanned(ip, LoginIPBanTime())
	userBanned := IsLoginBanned(req.Username, LoginUserBanTime())

	// Step 2: captcha. The submitted code may carry a TOTP on the end — that is
	// how the old single-field form let a user supply both. Splitting it here
	// means a user with 2FA can still log in when the captcha itself fails,
	// provided their account is not already under suspicion.
	totp := ""
	captchaOK := true
	if OpenCaptcha() {
		code := req.CaptchaCode
		if len(code) >= crypt.TotpLen {
			totp = code[len(code)-crypt.TotpLen:]
			code = code[:len(code)-crypt.TotpLen]
		}
		captchaOK = VerifyCaptcha(req.CaptchaID, code)
		if ipBanned || (!captchaOK && totp == "") || (!captchaOK && totp != "" && userBanned) {
			logs.Warn("api: captcha failed for user %s from %s", req.Username, ip)
			NoteLoginFailure(ip, true)
			rejectLogin(w, r, "the verification code is wrong, please get a new one", loginFailure{})
			return
		}
	}

	// Step 3: proof of work. Demanded when the operator forces it, or when this
	// attempt already looks suspect — a banned address, a banned account in
	// secure mode, or a captcha that only passed because of a TOTP.
	needPoW := ForcePoW() || ipBanned || (userBanned && SecureMode()) || (totp != "" && !captchaOK)
	if needPoW && PoWBits() > 0 {
		if req.Bits != PoWBits() || !common.ValidatePoW(PoWBits(), req.Password, req.PoWX) {
			logs.Warn("api: pow failed for user %s from %s", req.Username, ip)
			NoteLoginFailure(ip, true)
			if !captchaOK {
				NoteLoginFailure(req.Username, true)
			}
			rejectLogin(w, r, "proof of work verification failed", loginFailure{Bits: PoWBits()})
			return
		}
	}

	// Step 4: decrypt. Anything the browser did not encrypt with our key fails
	// here, and the response includes the certificate so a client holding a
	// stale key can recover.
	payload, err := crypt.ParseLoginPayload(req.Password)
	if err != nil {
		logs.Warn("api: decrypt error for user %s from %s: %v", req.Username, ip, err)
		NoteLoginFailure(ip, true)
		if !captchaOK {
			NoteLoginFailure(req.Username, true)
		}
		cert, _ := crypt.GetRSAPublicKeyPEM()
		rejectLogin(w, r, "decrypt error", loginFailure{Cert: cert})
		return
	}

	// Step 5: nonce. Consuming it here — after decryption, before the password
	// is looked at — is what makes a captured ciphertext single-use.
	if !loginNonces.Consume(payload.Nonce) {
		logs.Warn("api: invalid nonce for user %s from %s", req.Username, ip)
		NoteLoginFailure(ip, true)
		if !captchaOK {
			NoteLoginFailure(req.Username, true)
		}
		rejectLogin(w, r, "invalid nonce", loginFailure{})
		return
	}

	// Step 6: freshness, in secure mode only.
	if SecureMode() {
		now := common.TimeNow().UnixMilli()
		if payload.Timestamp < now-LoginMaxSkew() || payload.Timestamp > now+LoginMaxSkew() {
			logs.Warn("api: timestamp expired for user %s from %s", req.Username, ip)
			NoteLoginFailure(ip, true)
			if !captchaOK {
				NoteLoginFailure(req.Username, true)
			}
			rejectLogin(w, r, "timestamp expired", loginFailure{Timestamp: now})
			return
		}
	}

	// A short random pause blunts timing analysis of the credential comparison
	// below, which walks the client list and cannot be made constant-time.
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(20)))

	// Step 7: credentials.
	principal, ok := authenticate(req.Username, payload.Password, totp, ip, true)
	if !ok {
		logs.Warn("api: login failed for user %s from %s", req.Username, ip)
		NoteLoginFailure(req.Username, true)
		rejectLogin(w, r, "username or password incorrect", loginFailure{})
		return
	}

	token, exp, err := IssueToken(*principal)
	if err != nil {
		Internal(w, r, err)
		return
	}
	logs.Info("api: login success for user %s from %s", req.Username, ip)
	Ok(w, r, LoginResponse{
		Token:     token,
		ExpiresAt: exp.Unix(),
		User:      describe(principal),
	})
}

// rejectLogin answers a failed attempt, attaching a fresh nonce (and a fresh
// captcha when one is in use) so the client can retry immediately.
func rejectLogin(w http.ResponseWriter, r *http.Request, message string, detail loginFailure) {
	detail.Nonce = loginNonces.Issue()
	if OpenCaptcha() {
		if c, err := NewCaptcha(); err == nil {
			detail.Captcha = c
		}
	}
	writeJSON(w, http.StatusUnauthorized, Envelope{
		Code:      CodeUnauthorized,
		Message:   message,
		Data:      detail,
		RequestID: RequestID(r),
	})
}

// authenticate resolves credentials to a principal. explicit distinguishes a
// real login attempt from the implicit one used to detect a no-auth install:
// only an explicit attempt counts against the ban tracker.
func authenticate(username, password, totp, ip string, explicit bool) (*Principal, bool) {
	CleanBanRecords(false)

	if explicit && IsLoginBanned(ip, LoginIPBanTime()) {
		return nil, false
	}

	if adminAuth(username, password, totp) {
		// The bridge treats a recently-authenticated address as trusted for a
		// couple of hours; this is what lets the client download page work.
		if server.Bridge != nil {
			server.Bridge.Register.Store(common.GetIpByAddr(ip), time.Now().Add(2*time.Hour))
		}
		ClearLoginFailures(ip)
		return &Principal{Username: username, IsAdmin: true}, true
	}

	if p, ok := clientAuth(username, password, totp); ok {
		ClearLoginFailures(ip)
		return p, true
	}

	NoteLoginFailure(ip, explicit)
	return nil, false
}

// adminAuth checks the credentials from nps.conf. When a TOTP secret is set the
// code is mandatory, and may arrive either as its own field or appended to the
// password.
func adminAuth(username, password, totp string) bool {
	if username == "" || username != AdminUsername() {
		return false
	}
	if secret := AdminTotpSecret(); secret != "" {
		ok := false
		if totp != "" {
			ok, _ = crypt.ValidateTOTPCode(secret, totp)
		} else {
			if len(password) < crypt.TotpLen {
				return false
			}
			code := password[len(password)-crypt.TotpLen:]
			password = password[:len(password)-crypt.TotpLen]
			ok, _ = crypt.ValidateTOTPCode(secret, code)
		}
		if !ok {
			return false
		}
	}
	// An empty configured password means the install is unauthenticated, which
	// the caller has already decided is acceptable.
	return password == AdminPassword()
}

// clientAuth walks the client list looking for a matching web login. It mirrors
// the previous implementation exactly, including the "user"+vkey shortcut for
// clients that have no web credentials of their own.
func clientAuth(username, password, totp string) (*Principal, bool) {
	if !AllowUserLogin() || username == "" || password == "" {
		return nil, false
	}
	allowVkey := AllowUserVkeyLogin()

	var found *file.Client
	file.GetDb().JsonDb.Clients.Range(func(_, value any) bool {
		v := value.(*file.Client)
		if !v.Status || v.NoDisplay {
			return true
		}

		auth := false
		if v.WebUserName == "" && v.WebPassword == "" {
			// A client with no web credentials can still be reached with the
			// literal username "user" plus its vkey, when the operator allows it.
			if v.Id > 0 && username == "user" && allowVkey && v.VerifyKey == password {
				auth = true
			}
		}

		if !auth && v.WebUserName == username {
			pwd := password
			totpOK := true
			if v.WebTotpSecret != "" {
				totpOK = false
				if totp != "" {
					totpOK, _ = crypt.ValidateTOTPCode(v.WebTotpSecret, totp)
				} else if len(password) >= crypt.TotpLen {
					pwd = password[:len(password)-crypt.TotpLen]
					code := password[len(password)-crypt.TotpLen:]
					totpOK, _ = crypt.ValidateTOTPCode(v.WebTotpSecret, code)
				}
			} else if v.WebPassword == "" && v.VerifyKey == password {
				// No web password set: the vkey stands in for one.
				auth = true
			}
			if !auth && totpOK && v.WebPassword == pwd {
				auth = true
			}
		}

		if auth {
			found = v
			return false
		}
		return true
	})

	if found == nil {
		return nil, false
	}
	return &Principal{Username: found.WebUserName, ClientID: found.Id}, true
}

// MeInfo describes the logged-in principal and the switches the UI needs to
// decide what to render.
type MeInfo struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
	ClientID int    `json:"clientId"`
	// Version and Year are shown in the page footer.
	Version string `json:"version"`
	Year    int    `json:"year"`
	// Permissions mirrors the allow_* switches from nps.conf. The server
	// re-checks every one of these in the handler that acts on it; this copy
	// exists only so the UI does not offer controls that would be refused.
	Permissions Permissions `json:"permissions"`
}

// Permissions is the UI-visible subset of the configuration.
type Permissions struct {
	FlowLimit        bool `json:"flowLimit"`
	RateLimit        bool `json:"rateLimit"`
	TimeLimit        bool `json:"timeLimit"`
	ConnNumLimit     bool `json:"connNumLimit"`
	TunnelNumLimit   bool `json:"tunnelNumLimit"`
	MultiIP          bool `json:"multiIp"`
	SecretLink       bool `json:"secretLink"`
	SystemInfo       bool `json:"systemInfo"`
	LocalProxy       bool `json:"localProxy"`
	ChangeUsername   bool `json:"changeUsername"`
	UserLoginAllowed bool `json:"userLoginAllowed"`
	RegisterAllowed  bool `json:"registerAllowed"`
}

func describe(p *Principal) MeInfo {
	localProxy := AllowLocalProxy()
	if !p.IsAdmin {
		localProxy = AllowUserLocal()
	}
	return MeInfo{
		Username: p.Username,
		IsAdmin:  p.IsAdmin,
		ClientID: p.ClientID,
		Version:  server.GetVersion(),
		Year:     server.GetCurrentYear(),
		Permissions: Permissions{
			FlowLimit:        AllowFlowLimit(),
			RateLimit:        AllowRateLimit(),
			TimeLimit:        AllowTimeLimit(),
			ConnNumLimit:     AllowConnNumLimit(),
			TunnelNumLimit:   AllowTunnelNumLimit(),
			MultiIP:          AllowMultiIP(),
			SecretLink:       AllowSecretLink(),
			SystemInfo:       AllowLocalProxy() || SystemInfoDisplay(),
			LocalProxy:       localProxy,
			ChangeUsername:   AllowUserChangeUsername(),
			UserLoginAllowed: AllowUserLogin(),
			RegisterAllowed:  AllowUserRegister(),
		},
	}
}

// handleMe returns the caller's identity, used by the SPA on load to decide
// whether a stored token is still good.
func handleMe(w http.ResponseWriter, r *http.Request) {
	p := CurrentUser(r)
	if p == nil {
		Unauthorized(w, r, "authentication required")
		return
	}
	Ok(w, r, describe(p))
}

// handleLogout exists so the client has an explicit endpoint to call, and so
// the event is logged. A stateless token cannot be revoked server-side without
// a blacklist, which would reintroduce the session store this design removes;
// the client discards the token and it expires on its own shortly after.
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if p := CurrentUser(r); p != nil {
		logs.Info("api: logout for user %s from %s", p.Username, ClientIP(r))
	}
	Ok(w, r, nil)
}

// RegisterRequest is the body of POST /auth/register.
type RegisterRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"` // RSA ciphertext, as for login
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
}

// handleRegister creates a client account when the operator has enabled
// self-registration.
func handleRegister(w http.ResponseWriter, r *http.Request) {
	if !AllowUserRegister() {
		Forbidden(w, r, "registration is not allowed")
		return
	}
	if r.ContentLength > LoginMaxBody() {
		Fail(w, r, http.StatusRequestEntityTooLarge, CodeBadRequest, "payload too large")
		return
	}

	var req RegisterRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	// The admin username is reserved: allowing it would create a client whose
	// web login shadows the operator's.
	if req.Username == "" || req.Password == "" || req.Username == AdminUsername() {
		rejectLogin(w, r, "please check your input", loginFailure{})
		return
	}

	if OpenCaptcha() && !VerifyCaptcha(req.CaptchaID, req.CaptchaCode) {
		rejectLogin(w, r, "the verification code is wrong, please get a new one", loginFailure{})
		return
	}

	payload, err := crypt.ParseLoginPayload(req.Password)
	if err != nil {
		cert, _ := crypt.GetRSAPublicKeyPEM()
		rejectLogin(w, r, "decrypt error", loginFailure{Cert: cert})
		return
	}
	if !loginNonces.Consume(payload.Nonce) {
		rejectLogin(w, r, "invalid nonce", loginFailure{})
		return
	}
	if SecureMode() {
		now := common.TimeNow().UnixMilli()
		if payload.Timestamp < now-LoginMaxSkew() || payload.Timestamp > now+LoginMaxSkew() {
			rejectLogin(w, r, "timestamp expired", loginFailure{Timestamp: now})
			return
		}
	}

	client := &file.Client{
		Id:          int(file.GetDb().JsonDb.GetClientId()),
		Status:      true,
		Cnf:         &file.Config{},
		WebUserName: req.Username,
		WebPassword: payload.Password,
		Flow:        &file.Flow{},
	}
	if err := file.GetDb().NewClient(client); err != nil {
		// The only expected failure is a duplicate username, which is the
		// caller's problem to fix rather than a server error.
		Conflict(w, r, err.Error())
		return
	}
	logs.Info("api: registered user %s from %s", req.Username, ClientIP(r))
	Ok(w, r, map[string]any{"clientId": client.Id})
}

// handleCaptcha issues a standalone captcha, for the register form and for a
// UI that wants to refresh the image without restarting the whole challenge.
func handleCaptcha(w http.ResponseWriter, r *http.Request) {
	if !OpenCaptcha() {
		NotFound(w, r, "captcha is disabled")
		return
	}
	c, err := NewCaptcha()
	if err != nil {
		Internal(w, r, err)
		return
	}
	Ok(w, r, c)
}
