package api

import (
	"github.com/djylb/nps/lib/appconfig"
)

// Every setting the API reads from nps.conf goes through a named accessor here
// rather than being fetched inline. That keeps key spellings and defaults in
// one auditable list, and makes it obvious which knobs the admin surface obeys.

func cfg() appconfig.Configer { return appconfig.AppConfig() }

// --- deployment ---

// WebBaseURL is the raw web_base_url; callers normalise it via basepath.
func WebBaseURL() string { return cfg().String("web_base_url") }

// HeadCustomCode is operator-supplied markup injected into the page head.
func HeadCustomCode() string { return cfg().String("head_custom_code") }

// --- proxy trust ---

func allowXRealIP() bool { return cfg().DefaultBool("allow_x_real_ip", false) }

func trustedProxyIPs() string {
	return cfg().DefaultString("trusted_proxy_ips", "127.0.0.1")
}

// --- feature switches surfaced to the SPA ---
//
// These gate which controls the UI renders. They are advisory for the UI but
// authoritative on the server: every one of them is re-checked in the handler
// that acts on it, because a hidden button is not an access control.

func AllowUserLogin() bool          { return cfg().DefaultBool("allow_user_login", false) }
func AllowUserRegister() bool       { return cfg().DefaultBool("allow_user_register", false) }
func AllowUserVkeyLogin() bool      { return cfg().DefaultBool("allow_user_vkey_login", AllowUserLogin()) }
func AllowUserChangeUsername() bool { return cfg().DefaultBool("allow_user_change_username", false) }
func AllowFlowLimit() bool          { return cfg().DefaultBool("allow_flow_limit", false) }
func AllowRateLimit() bool          { return cfg().DefaultBool("allow_rate_limit", false) }
func AllowTimeLimit() bool          { return cfg().DefaultBool("allow_time_limit", false) }
func AllowConnNumLimit() bool       { return cfg().DefaultBool("allow_connection_num_limit", false) }
func AllowTunnelNumLimit() bool     { return cfg().DefaultBool("allow_tunnel_num_limit", false) }
func AllowMultiIP() bool            { return cfg().DefaultBool("allow_multi_ip", false) }
func AllowSecretLink() bool         { return cfg().DefaultBool("allow_secret_link", false) }
func SystemInfoDisplay() bool       { return cfg().DefaultBool("system_info_display", false) }

// AllowLocalProxy controls whether tunnel targets may point at the server's own
// loopback (a `bridge://` or 127.0.0.1 target).
func AllowLocalProxy() bool { return cfg().DefaultBool("allow_local_proxy", false) }

// AllowUserLocal extends AllowLocalProxy to non-admin users; it defaults to
// whatever AllowLocalProxy is, matching the previous controller behaviour.
func AllowUserLocal() bool {
	return cfg().DefaultBool("allow_user_local", AllowLocalProxy())
}

// --- credentials ---

func AdminUsername() string { return cfg().String("web_username") }
func AdminPassword() string { return cfg().String("web_password") }
func AdminTotpSecret() string {
	return cfg().String("totp_secret")
}

// AuthKey is the shared secret for the legacy `auth_key`+timestamp API used by
// third-party scripts. Empty disables that path entirely.
func AuthKey() string { return cfg().String("auth_key") }

// AuthCryptKey is the AES key used to hand AuthKey to the UI; it must be
// exactly 16 bytes for the legacy endpoint to encrypt anything.
func AuthCryptKey() string { return cfg().String("auth_crypt_key") }

// --- login hardening ---

func SecureMode() bool      { return cfg().DefaultBool("secure_mode", false) }
func OpenCaptcha() bool     { return cfg().DefaultBool("open_captcha", false) }
func ForcePoW() bool        { return cfg().DefaultBool("force_pow", false) }
func PoWBits() int          { return cfg().DefaultInt("pow_bits", 20) }
func LoginBanTime() int64   { return cfg().DefaultInt64("login_ban_time", 5) }
func LoginIPBanTime() int64 { return cfg().DefaultInt64("login_ip_ban_time", 180) }
func LoginUserBanTime() int64 {
	return cfg().DefaultInt64("login_user_ban_time", 3600)
}
func LoginMaxFailTimes() int { return cfg().DefaultInt("login_max_fail_times", 10) }
func LoginMaxBody() int64    { return cfg().DefaultInt64("login_max_body", 1024) }
func LoginMaxSkew() int64    { return cfg().DefaultInt64("login_max_skew", 5*60*1000) }

// --- JWT ---

// JWTKey is the HMAC signing secret. An empty value means the server must
// generate and persist one on first start.
func JWTKey() string { return cfg().String("api_jwt_key") }

// JWTTTLMinutes is how long an issued token stays valid.
func JWTTTLMinutes() int { return cfg().DefaultInt("api_jwt_ttl", 120) }

// --- TLS for the management listener ---

func WebOpenSSL() bool    { return cfg().String("web_open_ssl") == "true" }
func WebCertFile() string { return cfg().String("web_cert_file") }
func WebKeyFile() string  { return cfg().String("web_key_file") }
