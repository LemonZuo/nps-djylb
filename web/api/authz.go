package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/crypt"
	"github.com/djylb/nps/lib/jwt"
)

// principalKey carries the authenticated identity down to the handlers.
const principalKey ctxKey = iota + 1

// CurrentUser returns the principal behind r, or nil if the request did not go
// through RequireAuth. Handlers reached through RequireAuth can rely on it
// being non-nil.
func CurrentUser(r *http.Request) *Principal {
	p, _ := r.Context().Value(principalKey).(*Principal)
	return p
}

// WithPrincipal returns a copy of r carrying p. Exported for tests.
func WithPrincipal(r *http.Request, p *Principal) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey, p))
}

// RequireAuth rejects anything without a valid credential. Two are accepted:
//
//   - a bearer JWT, which is what the SPA uses;
//   - the legacy auth_key + timestamp query parameters, kept so the third-party
//     scripts written against the old web API keep working. That path always
//     grants admin, exactly as it did before.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := legacyAuthKeyPrincipal(r); p != nil {
			next.ServeHTTP(w, WithPrincipal(r, p))
			return
		}

		token := bearerToken(r)
		if token == "" {
			Unauthorized(w, r, "authentication required")
			return
		}
		p, err := ParseToken(token)
		if err != nil {
			// An expired token gets its own code so the SPA can distinguish
			// "your session ran out" from "your credentials are not accepted".
			if errors.Is(err, jwt.ErrExpired) {
				TokenExpired(w, r, "session expired, please log in again")
				return
			}
			Unauthorized(w, r, "invalid credentials")
			return
		}
		next.ServeHTTP(w, WithPrincipal(r, p))
	})
}

// RequireAdmin runs after RequireAuth and rejects non-admin principals. It is
// the guard on every endpoint that reads or changes global state.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := CurrentUser(r)
		if p == nil {
			Unauthorized(w, r, "authentication required")
			return
		}
		if !p.IsAdmin {
			Forbidden(w, r, "administrator privileges required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// legacyAuthKeyMaxSkew is how far the supplied timestamp may be from the
// server's clock, in seconds. Kept at the historical 20s: the parameter is a
// replay window, and widening it would weaken the scheme.
const legacyAuthKeyMaxSkew = 20

// legacyAuthKeyPrincipal implements the pre-existing `auth_key` API contract:
// the caller sends timestamp=<unix> and auth_key=md5(configured_key+timestamp).
// It returns nil when the parameters are absent or do not check out, so the
// caller falls through to the JWT path.
func legacyAuthKeyPrincipal(r *http.Request) *Principal {
	configured := AuthKey()
	if configured == "" {
		return nil
	}
	q := r.URL.Query()
	supplied := q.Get("auth_key")
	if supplied == "" {
		return nil
	}
	timestamp, err := strconv.ParseInt(q.Get("timestamp"), 10, 64)
	if err != nil {
		return nil
	}
	if !isValidAuthKey(configured, supplied, timestamp, common.TimeNow().Unix()) {
		return nil
	}
	return &Principal{Username: AdminUsername(), IsAdmin: true, TokenID: "legacy-auth-key"}
}

// isValidAuthKey checks the MD5 handshake. MD5 is weak, but the scheme is not
// relying on collision resistance — it is an HMAC-shaped construction over a
// shared secret and a timestamp, and it is here for compatibility with clients
// that already exist. The comparison is constant-time so the digest cannot be
// recovered one byte at a time.
func isValidAuthKey(configured, supplied string, timestamp, now int64) bool {
	if configured == "" || supplied == "" {
		return false
	}
	skew := now - timestamp
	if skew < 0 {
		skew = -skew
	}
	if skew > legacyAuthKeyMaxSkew {
		return false
	}
	expected := crypt.Md5(configured + strconv.FormatInt(timestamp, 10))
	if len(expected) != len(supplied) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

// OwnsClient reports whether p may act on the given client id. An admin may act
// on any; a user only on their own.
func OwnsClient(p *Principal, clientID int) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	return clientID != 0 && clientID == p.ClientID
}

// ScopeClientID returns the client id a listing endpoint should filter by: 0
// for an admin (meaning "no filter"), the caller's own id otherwise. Handlers
// must use this rather than a client_id from the query string, which a user
// could set to someone else's.
func ScopeClientID(p *Principal) int {
	if p == nil || p.IsAdmin {
		return 0
	}
	return p.ClientID
}
