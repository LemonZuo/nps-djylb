package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/djylb/nps/lib/appconfig"
	"github.com/djylb/nps/lib/jwt"
	"github.com/djylb/nps/lib/logs"
)

// Roles carried in a token's `role` claim.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// jwtKeyBytes is the resolved signing key, cached because it is read on every
// authenticated request.
var (
	jwtKeyOnce sync.Once
	jwtKeyVal  []byte
	jwtKeyErr  error
)

// jwtKeyLen is the length in bytes of a generated key: 32 bytes matches the
// output size of HMAC-SHA256, past which extra key material adds no strength.
const jwtKeyLen = 32

// InitTokenKey resolves the signing key at startup, generating and persisting
// one if nps.conf does not define api_jwt_key.
//
// Doing this eagerly rather than on first login means a broken config directory
// is reported at boot, and it also fixes the key before any request can race
// two goroutines into generating different ones.
func InitTokenKey() error {
	jwtKeyOnce.Do(func() { jwtKeyVal, jwtKeyErr = resolveJWTKey() })
	return jwtKeyErr
}

func signingKey() ([]byte, error) {
	jwtKeyOnce.Do(func() { jwtKeyVal, jwtKeyErr = resolveJWTKey() })
	return jwtKeyVal, jwtKeyErr
}

func resolveJWTKey() ([]byte, error) {
	if configured := strings.TrimSpace(JWTKey()); configured != "" {
		// The value is used as raw bytes rather than being hex/base64 decoded, so
		// that an operator can paste in any passphrase they like. Length is the
		// only requirement.
		if len(configured) < 16 {
			return nil, errors.New("api_jwt_key must be at least 16 characters")
		}
		return []byte(configured), nil
	}

	buf := make([]byte, jwtKeyLen)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	generated := hex.EncodeToString(buf)

	path := appconfig.Path()
	if path == "" {
		// No file to write to (tests, or a config that was never loaded). The key
		// still works for this process; it just will not survive a restart, which
		// only means everyone has to log in again.
		logs.Warn("api: no config file to persist api_jwt_key to; sessions will not survive a restart")
		return []byte(generated), nil
	}

	err := appconfig.AppendKeys(path, []appconfig.Entry{{
		Key:     "api_jwt_key",
		Value:   generated,
		Comment: "管理 API 的 JWT 签名密钥（首次启动自动生成）\nJWT signing key for the management API (generated on first start)\n修改后所有已登录会话立即失效 / changing it invalidates every active session",
	}})
	if err != nil {
		// Falling back to an in-memory key keeps the server usable on a read-only
		// config volume; the operator just gets logged out on every restart.
		logs.Warn("api: could not persist api_jwt_key (%v); using an in-memory key for this run", err)
		return []byte(generated), nil
	}
	logs.Info("api: generated api_jwt_key and saved it to %s", path)
	return []byte(generated), nil
}

// Principal is the authenticated identity behind a request.
type Principal struct {
	// Username is the display name; for the admin it is web_username.
	Username string
	// IsAdmin distinguishes the operator from a tunnel owner.
	IsAdmin bool
	// ClientID is the file.Client a non-admin is scoped to; 0 for an admin.
	ClientID int
	// TokenID identifies the token, for audit lines.
	TokenID string
	// ExpiresAt is when the current token stops being accepted.
	ExpiresAt time.Time
}

// IssueToken signs a token for p, valid for the configured TTL.
func IssueToken(p Principal) (token string, expiresAt time.Time, err error) {
	key, err := signingKey()
	if err != nil {
		return "", time.Time{}, err
	}

	ttl := time.Duration(JWTTTLMinutes()) * time.Minute
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	now := time.Now()
	exp := now.Add(ttl)

	role := RoleUser
	if p.IsAdmin {
		role = RoleAdmin
	}

	jti, err := newTokenID()
	if err != nil {
		return "", time.Time{}, err
	}

	token, err = jwt.Sign(key, jwt.Claims{
		Subject:   p.Username,
		Role:      role,
		ClientID:  p.ClientID,
		IssuedAt:  now.Unix(),
		ExpiresAt: exp.Unix(),
		TokenID:   jti,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

func newTokenID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// ParseToken validates a bearer token and returns the principal it names.
func ParseToken(token string) (*Principal, error) {
	key, err := signingKey()
	if err != nil {
		return nil, err
	}
	claims, err := jwt.Verify(key, token, time.Now())
	if err != nil {
		return nil, err
	}
	isAdmin := claims.Role == RoleAdmin
	if !isAdmin && claims.Role != RoleUser {
		return nil, jwt.ErrMalformed
	}
	// An admin token must not carry a client scope, and a user token must carry
	// one: either combination would mean the claims were built inconsistently,
	// and downstream authorisation reads both fields.
	if isAdmin && claims.ClientID != 0 {
		return nil, jwt.ErrMalformed
	}
	if !isAdmin && claims.ClientID <= 0 {
		return nil, jwt.ErrMalformed
	}
	return &Principal{
		Username:  claims.Subject,
		IsAdmin:   isAdmin,
		ClientID:  claims.ClientID,
		TokenID:   claims.TokenID,
		ExpiresAt: time.Unix(claims.ExpiresAt, 0),
	}, nil
}

// bearerToken extracts the credential from the Authorization header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	scheme, value, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}
