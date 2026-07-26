package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

var (
	testKey = []byte("a-test-signing-key-of-decent-length")
	now     = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
)

func adminClaims() Claims {
	return Claims{
		Subject:   "admin",
		Role:      "admin",
		ClientID:  0,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(2 * time.Hour).Unix(),
		TokenID:   "abc123",
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	token, err := Sign(testKey, adminClaims())
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := Verify(testKey, token, now)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	want := adminClaims()
	if *got != want {
		t.Errorf("claims = %+v, want %+v", *got, want)
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	token, err := Sign(testKey, adminClaims())
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := Verify([]byte("a-different-key"), token, now); !errors.Is(err, ErrBadSignature) {
		t.Errorf("err = %v, want ErrBadSignature", err)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	token, err := Sign(testKey, Claims{
		Subject: "bob", Role: "user", ClientID: 7,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), TokenID: "x",
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	parts := strings.Split(token, ".")

	// Re-encode the payload with role escalated to admin, keeping the original
	// signature: this is the attack the signature exists to stop.
	forged := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"bob","role":"admin","cid":7,"iat":` +
			itoa(now.Unix()) + `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"jti":"x"}`))
	tampered := parts[0] + "." + forged + "." + parts[2]

	if _, err := Verify(testKey, tampered, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature for an escalated role", err)
	}
}

func TestVerifyRejectsAlgNone(t *testing.T) {
	// The canonical JWT vulnerability: a token declaring alg "none" with an
	// empty signature must never be accepted.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"admin","role":"admin","cid":0,"iat":` +
			itoa(now.Unix()) + `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"jti":"x"}`))

	for _, token := range []string{
		header + "." + payload + ".",
		header + "." + payload + ".c2ln",
	} {
		if _, err := Verify(testKey, token, now); err == nil {
			t.Errorf("Verify accepted an alg=none token: %s", token)
		}
	}
}

func TestVerifyRejectsOtherAlgorithms(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"admin","role":"admin","cid":0,"iat":` +
			itoa(now.Unix()) + `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"jti":"x"}`))

	for _, algName := range []string{"HS384", "HS512", "RS256", "ES256", "hs256", "None", "NONE"} {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"` + algName + `","typ":"JWT"}`))
		signingInput := header + "." + payload
		// Sign it correctly with HMAC-SHA256 so that only the declared
		// algorithm distinguishes it from a valid token.
		mac := hmac.New(sha256.New, testKey)
		mac.Write([]byte(signingInput))
		token := signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

		if _, err := Verify(testKey, token, now); !errors.Is(err, ErrBadAlgorithm) && !errors.Is(err, ErrMalformed) {
			t.Errorf("alg=%q: err = %v, want a rejection", algName, err)
		}
	}
}

func TestVerifyRejectsDuplicateClaims(t *testing.T) {
	// encoding/json keeps the last value for a repeated key, so a duplicated
	// role would resolve differently for a reader than for the verifier.
	raw := `{"sub":"bob","role":"user","role":"admin","cid":7,"iat":` +
		itoa(now.Unix()) + `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"jti":"x"}`
	payload := base64.RawURLEncoding.EncodeToString([]byte(raw))
	signingInput := encodedHeader + "." + payload
	mac := hmac.New(sha256.New, testKey)
	mac.Write([]byte(signingInput))
	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if _, err := Verify(testKey, token, now); !errors.Is(err, ErrMalformed) {
		t.Errorf("err = %v, want ErrMalformed for duplicate claims", err)
	}
}

func TestVerifyRejectsUnknownClaims(t *testing.T) {
	raw := `{"sub":"bob","role":"user","cid":7,"iat":` +
		itoa(now.Unix()) + `,"exp":` + itoa(now.Add(time.Hour).Unix()) +
		`,"jti":"x","admin":true}`
	payload := base64.RawURLEncoding.EncodeToString([]byte(raw))
	signingInput := encodedHeader + "." + payload
	mac := hmac.New(sha256.New, testKey)
	mac.Write([]byte(signingInput))
	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if _, err := Verify(testKey, token, now); !errors.Is(err, ErrMalformed) {
		t.Errorf("err = %v, want ErrMalformed for an unknown claim", err)
	}
}

func TestVerifyExpiry(t *testing.T) {
	c := adminClaims()
	c.ExpiresAt = now.Add(time.Hour).Unix()
	token, err := Sign(testKey, c)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if _, err := Verify(testKey, token, now.Add(59*time.Minute)); err != nil {
		t.Errorf("token rejected before expiry: %v", err)
	}
	if _, err := Verify(testKey, token, now.Add(2*time.Hour)); !errors.Is(err, ErrExpired) {
		t.Errorf("err = %v, want ErrExpired", err)
	}
	// Within the skew allowance a just-expired token still passes.
	if _, err := Verify(testKey, token, now.Add(time.Hour+10*time.Second)); err != nil {
		t.Errorf("token rejected within the clock-skew allowance: %v", err)
	}
}

func TestVerifyRejectsFutureToken(t *testing.T) {
	c := adminClaims()
	c.IssuedAt = now.Add(time.Hour).Unix()
	c.ExpiresAt = now.Add(3 * time.Hour).Unix()
	token, err := Sign(testKey, c)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := Verify(testKey, token, now); !errors.Is(err, ErrNotYetValid) {
		t.Errorf("err = %v, want ErrNotYetValid", err)
	}
}

func TestVerifyRejectsMissingExpiry(t *testing.T) {
	// A token with no exp would be valid forever.
	raw := `{"sub":"bob","role":"user","cid":7,"iat":` + itoa(now.Unix()) + `,"jti":"x"}`
	payload := base64.RawURLEncoding.EncodeToString([]byte(raw))
	signingInput := encodedHeader + "." + payload
	mac := hmac.New(sha256.New, testKey)
	mac.Write([]byte(signingInput))
	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if _, err := Verify(testKey, token, now); !errors.Is(err, ErrMalformed) {
		t.Errorf("err = %v, want ErrMalformed for a token without exp", err)
	}
}

func TestVerifyRejectsMalformedInput(t *testing.T) {
	valid, err := Sign(testKey, adminClaims())
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	parts := strings.Split(valid, ".")

	cases := map[string]string{
		"empty":           "",
		"no dots":         "abcdef",
		"one dot":         parts[0] + "." + parts[1],
		"four parts":      valid + ".extra",
		"empty signature": parts[0] + "." + parts[1] + ".",
		"bad base64":      parts[0] + "." + "!!!not-base64!!!" + "." + parts[2],
		"empty header":    "." + parts[1] + "." + parts[2],
		"whitespace":      "   ",
		"only dots":       "..",
		"oversize":        strings.Repeat("a", maxTokenLen+1),
	}
	for name, token := range cases {
		if _, err := Verify(testKey, token, now); err == nil {
			t.Errorf("%s: Verify accepted %q", name, token)
		}
	}
}

func TestEmptyKeyIsRejected(t *testing.T) {
	if _, err := Sign(nil, adminClaims()); !errors.Is(err, ErrEmptyKey) {
		t.Errorf("Sign with an empty key: err = %v, want ErrEmptyKey", err)
	}
	if _, err := Verify(nil, "a.b.c", now); !errors.Is(err, ErrEmptyKey) {
		t.Errorf("Verify with an empty key: err = %v, want ErrEmptyKey", err)
	}
}

func TestHeaderKeyOrderIsAccepted(t *testing.T) {
	// A header that is semantically identical but ordered differently should
	// still verify, so tokens stay portable if the emitter ever changes.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"HS256"}`))
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"admin","role":"admin","cid":0,"iat":` +
			itoa(now.Unix()) + `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"jti":"x"}`))
	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, testKey)
	mac.Write([]byte(signingInput))
	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if _, err := Verify(testKey, token, now); err != nil {
		t.Errorf("Verify rejected a reordered header: %v", err)
	}
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func FuzzVerify(f *testing.F) {
	valid, _ := Sign(testKey, adminClaims())
	f.Add(valid)
	f.Add("")
	f.Add("a.b.c")
	f.Add(encodedHeader + "..")

	// Verify must never panic, whatever it is handed: it is the first thing an
	// unauthenticated request touches.
	f.Fuzz(func(t *testing.T, token string) {
		_, _ = Verify(testKey, token, now)
	})
}
