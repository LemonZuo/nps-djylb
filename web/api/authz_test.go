package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/djylb/nps/lib/crypt"
	"github.com/djylb/nps/lib/jwt"
)

// okHandler records that it ran and echoes the principal it saw.
func okHandler(seen **Principal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = CurrentUser(r)
		Ok(w, r, nil)
	})
}

func TestRequireAuthRejectsMissingCredentials(t *testing.T) {
	loadConfig(t, "appname=nps\n")
	useTestKey(t)

	var seen *Principal
	rec := httptest.NewRecorder()
	RequireAuth(okHandler(&seen)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if seen != nil {
		t.Error("the handler ran without a credential")
	}
}

func TestRequireAuthAcceptsBearerToken(t *testing.T) {
	loadConfig(t, "appname=nps\n")
	useTestKey(t)

	token, _, err := IssueToken(Principal{Username: "bob", ClientID: 3})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	var seen *Principal
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	RequireAuth(okHandler(&seen)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	if seen == nil || seen.ClientID != 3 || seen.IsAdmin {
		t.Errorf("principal = %+v, want bob/3 non-admin", seen)
	}
}

func TestRequireAuthDistinguishesExpiredTokens(t *testing.T) {
	// The SPA needs to tell "log in again" apart from "your credentials are
	// not accepted", so it can retry silently in the first case.
	loadConfig(t, "appname=nps\n")
	key := useTestKey(t)

	// Sign a token whose window is already in the past, rather than sleeping.
	past := time.Now().Add(-3 * time.Hour)
	expired, err := jwt.Sign(key, jwt.Claims{
		Subject: "admin", Role: RoleAdmin,
		IssuedAt: past.Unix(), ExpiresAt: past.Add(time.Hour).Unix(), TokenID: "x",
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	var seen *Principal
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	rec := httptest.NewRecorder()
	RequireAuth(okHandler(&seen)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if env := decodeEnvelope(t, rec); env.Code != CodeTokenExpired {
		t.Errorf("code = %d, want %d (token expired)", env.Code, CodeTokenExpired)
	}
}

func TestRequireAdmin(t *testing.T) {
	loadConfig(t, "appname=nps\n")
	useTestKey(t)

	cases := []struct {
		name  string
		p     Principal
		want  int
		token bool
	}{
		{"admin", Principal{Username: "admin", IsAdmin: true}, http.StatusOK, true},
		{"user", Principal{Username: "bob", ClientID: 3}, http.StatusForbidden, true},
		{"anonymous", Principal{}, http.StatusUnauthorized, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen *Principal
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tc.token {
				token, _, err := IssueToken(tc.p)
				if err != nil {
					t.Fatalf("IssueToken: %v", err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rec := httptest.NewRecorder()
			RequireAuth(RequireAdmin(okHandler(&seen))).ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tc.want, rec.Body)
			}
			if tc.want != http.StatusOK && seen != nil {
				t.Error("the handler ran despite the rejection")
			}
		})
	}
}

func TestRequireAdminAloneRejectsAnonymous(t *testing.T) {
	// RequireAdmin is always composed after RequireAuth, but it must fail
	// closed on its own in case that composition is ever broken.
	var seen *Principal
	rec := httptest.NewRecorder()
	RequireAdmin(okHandler(&seen)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- legacy auth_key compatibility ---

func legacyURL(key string, ts int64) string {
	return "/x?auth_key=" + crypt.Md5(key+strconv.FormatInt(ts, 10)) +
		"&timestamp=" + strconv.FormatInt(ts, 10)
}

func TestLegacyAuthKeyGrantsAdmin(t *testing.T) {
	loadConfig(t, "appname=nps\nauth_key=shared-secret\nweb_username=admin\n")
	useTestKey(t)

	var seen *Principal
	rec := httptest.NewRecorder()
	RequireAuth(RequireAdmin(okHandler(&seen))).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, legacyURL("shared-secret", time.Now().Unix()), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	if seen == nil || !seen.IsAdmin {
		t.Errorf("principal = %+v, want an admin", seen)
	}
}

func TestLegacyAuthKeyRejections(t *testing.T) {
	loadConfig(t, "appname=nps\nauth_key=shared-secret\nweb_username=admin\n")
	useTestKey(t)

	now := time.Now().Unix()
	cases := map[string]string{
		"wrong secret":     legacyURL("not-the-secret", now),
		"stale timestamp":  legacyURL("shared-secret", now-60),
		"future timestamp": legacyURL("shared-secret", now+60),
		"no timestamp":     "/x?auth_key=" + crypt.Md5("shared-secret"+strconv.FormatInt(now, 10)),
		"empty key":        "/x?auth_key=&timestamp=" + strconv.FormatInt(now, 10),
		"garbage key":      "/x?auth_key=zzz&timestamp=" + strconv.FormatInt(now, 10),
		// The digest is over key+timestamp; reusing one timestamp's digest with
		// another timestamp must fail.
		"mismatched pair": "/x?auth_key=" + crypt.Md5("shared-secret"+strconv.FormatInt(now-5, 10)) +
			"&timestamp=" + strconv.FormatInt(now, 10),
	}
	for name, url := range cases {
		t.Run(name, func(t *testing.T) {
			var seen *Principal
			rec := httptest.NewRecorder()
			RequireAuth(okHandler(&seen)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			if seen != nil {
				t.Error("the handler ran")
			}
		})
	}
}

func TestLegacyAuthKeyDisabledWhenUnconfigured(t *testing.T) {
	// With no auth_key in nps.conf the whole path must be off, or an empty
	// configured value could be matched by an empty supplied one.
	loadConfig(t, "appname=nps\n")
	useTestKey(t)

	var seen *Principal
	rec := httptest.NewRecorder()
	RequireAuth(okHandler(&seen)).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, legacyURL("", time.Now().Unix()), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestIsValidAuthKeySkewWindow(t *testing.T) {
	const key = "k"
	now := int64(1_700_000_000)
	digest := func(ts int64) string { return crypt.Md5(key + strconv.FormatInt(ts, 10)) }

	for _, offset := range []int64{0, 19, -19, 20, -20} {
		ts := now + offset
		if !isValidAuthKey(key, digest(ts), ts, now) {
			t.Errorf("offset %+d was rejected but is inside the window", offset)
		}
	}
	for _, offset := range []int64{21, -21, 3600} {
		ts := now + offset
		if isValidAuthKey(key, digest(ts), ts, now) {
			t.Errorf("offset %+d was accepted but is outside the window", offset)
		}
	}
}

// --- scope helpers ---

func TestOwnsClient(t *testing.T) {
	admin := &Principal{IsAdmin: true}
	user := &Principal{ClientID: 5}

	if !OwnsClient(admin, 99) {
		t.Error("an admin was denied another client")
	}
	if !OwnsClient(user, 5) {
		t.Error("a user was denied their own client")
	}
	if OwnsClient(user, 6) {
		t.Error("a user reached another client")
	}
	if OwnsClient(user, 0) {
		t.Error("client id 0 was treated as owned")
	}
	if OwnsClient(nil, 5) {
		t.Error("a nil principal owned a client")
	}
}

func TestScopeClientID(t *testing.T) {
	if got := ScopeClientID(&Principal{IsAdmin: true}); got != 0 {
		t.Errorf("admin scope = %d, want 0 (unfiltered)", got)
	}
	if got := ScopeClientID(&Principal{ClientID: 5}); got != 5 {
		t.Errorf("user scope = %d, want 5", got)
	}
	if got := ScopeClientID(nil); got != 0 {
		t.Errorf("nil scope = %d, want 0", got)
	}
}
