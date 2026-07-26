package api

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/djylb/nps/lib/jwt"
)

// useTestKey pins the signing key so the tests do not depend on a config file.
func useTestKey(t *testing.T) []byte {
	t.Helper()
	key := []byte("web-api-test-signing-key-32-bytes!!")
	prevVal, prevErr := jwtKeyVal, jwtKeyErr
	jwtKeyOnce.Do(func() {}) // mark as resolved so signingKey does not read the config
	jwtKeyVal, jwtKeyErr = key, nil
	t.Cleanup(func() { jwtKeyVal, jwtKeyErr = prevVal, prevErr })
	return key
}

func TestIssueAndParseAdminToken(t *testing.T) {
	useTestKey(t)

	token, exp, err := IssueToken(Principal{Username: "admin", IsAdmin: true})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Errorf("expiry %v is not in the future", exp)
	}

	p, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if p.Username != "admin" || !p.IsAdmin || p.ClientID != 0 {
		t.Errorf("principal = %+v, want the admin", p)
	}
	if p.TokenID == "" {
		t.Error("no token id was issued")
	}
}

func TestIssueAndParseUserToken(t *testing.T) {
	useTestKey(t)

	token, _, err := IssueToken(Principal{Username: "bob", ClientID: 7})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	p, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if p.IsAdmin {
		t.Error("a user token parsed as admin")
	}
	if p.ClientID != 7 || p.Username != "bob" {
		t.Errorf("principal = %+v, want bob/7", p)
	}
}

func TestTokenIDsAreUnique(t *testing.T) {
	useTestKey(t)

	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		token, _, err := IssueToken(Principal{Username: "admin", IsAdmin: true})
		if err != nil {
			t.Fatalf("IssueToken: %v", err)
		}
		p, err := ParseToken(token)
		if err != nil {
			t.Fatalf("ParseToken: %v", err)
		}
		if seen[p.TokenID] {
			t.Fatalf("token id %q was reused", p.TokenID)
		}
		seen[p.TokenID] = true
	}
}

func TestParseTokenRejectsForeignSignature(t *testing.T) {
	useTestKey(t)

	// Signed with a different key: the classic "I made my own token" attempt.
	forged, err := jwt.Sign([]byte("attacker-controlled-key-long-enough"), jwt.Claims{
		Subject: "admin", Role: RoleAdmin,
		IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
		TokenID: "x",
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := ParseToken(forged); !errors.Is(err, jwt.ErrBadSignature) {
		t.Errorf("err = %v, want ErrBadSignature", err)
	}
}

func TestParseTokenRejectsExpired(t *testing.T) {
	key := useTestKey(t)

	past := time.Now().Add(-3 * time.Hour)
	token, err := jwt.Sign(key, jwt.Claims{
		Subject: "admin", Role: RoleAdmin,
		IssuedAt: past.Unix(), ExpiresAt: past.Add(time.Hour).Unix(), TokenID: "x",
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := ParseToken(token); !errors.Is(err, jwt.ErrExpired) {
		t.Errorf("err = %v, want ErrExpired", err)
	}
}

func TestParseTokenRejectsInconsistentScope(t *testing.T) {
	key := useTestKey(t)
	now := time.Now()

	// Each of these is correctly signed; what makes them invalid is that role
	// and client scope disagree. Accepting either would let a client-scoped
	// token reach admin-only data or vice versa.
	cases := map[string]jwt.Claims{
		"admin with a client scope": {
			Subject: "admin", Role: RoleAdmin, ClientID: 5,
			IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), TokenID: "x",
		},
		"user without a client scope": {
			Subject: "bob", Role: RoleUser, ClientID: 0,
			IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), TokenID: "x",
		},
		"user with a negative client id": {
			Subject: "bob", Role: RoleUser, ClientID: -1,
			IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), TokenID: "x",
		},
		"unknown role": {
			Subject: "bob", Role: "superuser", ClientID: 1,
			IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), TokenID: "x",
		},
		"empty role": {
			Subject: "bob", Role: "", ClientID: 1,
			IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), TokenID: "x",
		},
	}
	for name, claims := range cases {
		token, err := jwt.Sign(key, claims)
		if err != nil {
			t.Fatalf("%s: Sign: %v", name, err)
		}
		if _, err := ParseToken(token); err == nil {
			t.Errorf("%s: token was accepted", name)
		}
	}
}

func TestParseTokenRejectsGarbage(t *testing.T) {
	useTestKey(t)
	for _, token := range []string{"", "not-a-token", "a.b.c", "..."} {
		if _, err := ParseToken(token); err == nil {
			t.Errorf("ParseToken accepted %q", token)
		}
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":     "abc",
		"bearer abc":     "abc", // the scheme is case-insensitive per RFC 7235
		"BEARER abc":     "abc",
		"Bearer  abc  ":  "abc",
		"Basic abc":      "",
		"abc":            "",
		"":               "",
		"Bearer":         "",
		"Token abc":      "",
		"Bearer a.b.c==": "a.b.c==",
	}
	for header, want := range cases {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		if got := bearerToken(r); got != want {
			t.Errorf("bearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}
