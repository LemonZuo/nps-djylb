// Package jwt implements the minimum JWT needed by the NPS admin API: HS256
// signing and verification of a fixed claim set.
//
// A dedicated implementation is used instead of a general-purpose library
// because the flexibility of a general library is precisely what makes JWT
// error-prone. The historical vulnerabilities all come from accepting more than
// the application needs — an `alg` of "none", an algorithm substituted from
// RSA to HMAC, a duplicate claim shadowing an earlier one. Here exactly one
// algorithm is compiled in and everything else is rejected before any claim is
// read.
package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors returned by Verify. They are distinguished so the API can answer an
// expired token differently from a forged one: the first means "log in again",
// the second is worth logging as an attack.
var (
	ErrMalformed    = errors.New("jwt: malformed token")
	ErrBadAlgorithm = errors.New("jwt: unsupported algorithm")
	ErrBadSignature = errors.New("jwt: signature mismatch")
	ErrExpired      = errors.New("jwt: token expired")
	ErrNotYetValid  = errors.New("jwt: token not yet valid")
	ErrEmptyKey     = errors.New("jwt: signing key is empty")
)

// alg is the only algorithm this package will produce or accept.
const alg = "HS256"

// maxTokenLen bounds work done before any parsing. A legitimate NPS token is
// a few hundred bytes; anything far larger is not worth base64-decoding.
const maxTokenLen = 4096

// clockSkew tolerates small clock differences between the issuing and
// verifying side, which matters when nps runs in a container whose clock
// drifts from the host's.
const clockSkew = 30 * time.Second

// Claims is the fixed payload of an NPS session token. There are no arbitrary
// extra fields: everything the API authorises on is named here.
type Claims struct {
	// Subject is the display name of the logged-in principal.
	Subject string `json:"sub"`
	// Role is "admin" or "user".
	Role string `json:"role"`
	// ClientID is the file.Client this token is scoped to; 0 for an admin.
	ClientID int `json:"cid"`
	// IssuedAt and ExpiresAt are Unix seconds.
	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
	// TokenID is a random per-token value, for correlating audit log lines.
	TokenID string `json:"jti"`
}

// header is the fixed JOSE header. It is emitted verbatim rather than being
// marshalled from a struct, so the produced token is byte-stable.
var encodedHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// Sign returns a signed token for the given claims.
func Sign(key []byte, c Claims) (string, error) {
	if len(key) == 0 {
		return "", ErrEmptyKey
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("jwt: marshal claims: %w", err)
	}
	signingInput := encodedHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(key, signingInput)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify checks a token's signature and validity window and returns its claims.
//
// The order matters: the signature is checked before any claim is interpreted,
// so unauthenticated input never reaches the time comparisons or the role.
func Verify(key []byte, token string, now time.Time) (*Claims, error) {
	if len(key) == 0 {
		return nil, ErrEmptyKey
	}
	if token == "" || len(token) > maxTokenLen {
		return nil, ErrMalformed
	}

	// A JWS has exactly three parts. Rejecting anything else here also rules
	// out the five-part JWE shape, which this package does not implement.
	firstDot := strings.IndexByte(token, '.')
	lastDot := strings.LastIndexByte(token, '.')
	if firstDot <= 0 || lastDot <= firstDot || lastDot == len(token)-1 {
		return nil, ErrMalformed
	}
	if strings.IndexByte(token[firstDot+1:lastDot], '.') != -1 {
		return nil, ErrMalformed
	}

	headerPart, payloadPart, sigPart := token[:firstDot], token[firstDot+1:lastDot], token[lastDot+1:]

	if err := checkHeader(headerPart); err != nil {
		return nil, err
	}

	sig, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil {
		return nil, ErrMalformed
	}
	expected := sign(key, token[:lastDot])
	// Constant-time comparison: a byte-wise early exit would leak, over many
	// attempts, how much of a guessed signature was correct.
	if !hmac.Equal(sig, expected) {
		return nil, ErrBadSignature
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return nil, ErrMalformed
	}
	c, err := decodeClaims(payload)
	if err != nil {
		return nil, err
	}

	if c.ExpiresAt <= 0 || c.IssuedAt <= 0 {
		// A token without a validity window would never expire.
		return nil, ErrMalformed
	}
	if now.After(time.Unix(c.ExpiresAt, 0).Add(clockSkew)) {
		return nil, ErrExpired
	}
	if now.Add(clockSkew).Before(time.Unix(c.IssuedAt, 0)) {
		return nil, ErrNotYetValid
	}
	return c, nil
}

// checkHeader accepts only the exact header this package emits. Parsing the
// header as free-form JSON and then consulting its `alg` is the pattern behind
// the classic algorithm-confusion attacks; comparing against a whitelist
// avoids the question entirely.
func checkHeader(part string) error {
	if part == encodedHeader {
		return nil
	}
	// Fall back to a strict decode so that a semantically identical header with
	// different key order still works, while anything declaring another
	// algorithm — including "none" — is refused.
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return ErrMalformed
	}
	var h struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&h); err != nil {
		return ErrMalformed
	}
	if h.Alg != alg {
		return ErrBadAlgorithm
	}
	if h.Typ != "" && !strings.EqualFold(h.Typ, "JWT") {
		return ErrMalformed
	}
	return nil
}

// decodeClaims parses the payload, rejecting unknown and duplicate fields.
//
// Duplicates matter: encoding/json keeps the last occurrence, so a payload of
// {"role":"admin","role":"user"} would verify as a user while a naive reader
// of the token text sees admin — or the reverse, depending on which end is
// looked at. Refusing duplicates removes the ambiguity.
func decodeClaims(payload []byte) (*Claims, error) {
	if err := rejectDuplicateKeys(payload); err != nil {
		return nil, err
	}
	var c Claims
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, ErrMalformed
	}
	// Reject trailing content after the object.
	if dec.More() {
		return nil, ErrMalformed
	}
	return &c, nil
}

// rejectDuplicateKeys walks the top-level object and fails on a repeated key.
func rejectDuplicateKeys(payload []byte) error {
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	tok, err := dec.Token()
	if err != nil {
		return ErrMalformed
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return ErrMalformed
	}
	seen := make(map[string]struct{})
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return ErrMalformed
		}
		key, ok := tok.(string)
		if !ok {
			return ErrMalformed
		}
		if _, dup := seen[key]; dup {
			return ErrMalformed
		}
		seen[key] = struct{}{}
		// Consume the value, descending through any nested structure.
		if err := skipValue(dec); err != nil {
			return ErrMalformed
		}
	}
	return nil
}

// skipValue consumes exactly one JSON value from dec.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil // a scalar; already consumed
	}
	if delim != '{' && delim != '[' {
		return errors.New("unexpected delimiter")
	}
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}

func sign(key []byte, signingInput string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}
