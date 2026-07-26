package api

import (
	"sync"
	"time"

	"github.com/djylb/nps/lib/crypt"
)

// The login payload is RSA-encrypted by the browser and carries a nonce that
// the server issued moments earlier. Without it, a captured ciphertext could be
// replayed forever: RSA-PKCS1v15 of the same password yields a valid blob for
// any later request. Consuming the nonce on use makes each ciphertext good
// exactly once.
//
// The old Beego controller kept the nonce in the session cookie. With JWT there
// is no server session to hang it on, so it lives here instead — which also
// makes it single-use globally rather than per-browser, a slightly stronger
// property.

const (
	// nonceTTL bounds how long a challenge stays usable. Long enough for a slow
	// human plus a proof-of-work solve, short enough that the store stays small.
	nonceTTL = 5 * time.Minute

	// nonceLen is the number of characters in an issued nonce.
	nonceLen = 16

	// maxNonces caps memory use. /auth/challenge is unauthenticated, so an
	// attacker can request nonces in a loop; past this many live entries the
	// store sweeps and then refuses to grow further.
	maxNonces = 20000
)

// nonceStore holds issued-but-unused challenges.
type nonceStore struct {
	mu     sync.Mutex
	items  map[string]time.Time // nonce -> expiry
	sweep  time.Time            // when the last sweep ran
	nowFn  func() time.Time     // overridable in tests
	newFn  func() string        // overridable in tests
	maxLen int
}

func newNonceStore() *nonceStore {
	return &nonceStore{
		items:  make(map[string]time.Time),
		nowFn:  time.Now,
		newFn:  func() string { return crypt.GetRandomString(nonceLen) },
		maxLen: maxNonces,
	}
}

// loginNonces is the process-wide store used by the login and register handlers.
var loginNonces = newNonceStore()

// Issue returns a fresh nonce, or "" if the store is full. A caller that gets
// "" should surface a 429 rather than proceed: handing out a nonce that was
// never recorded would fail verification anyway.
func (s *nonceStore) Issue() string {
	now := s.nowFn()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sweepLocked(now)
	if len(s.items) >= s.maxLen {
		return ""
	}
	// A collision would silently extend an existing nonce's life; with 16 random
	// characters it will not happen, but retrying costs nothing.
	for i := 0; i < 3; i++ {
		n := s.newFn()
		if n == "" {
			return ""
		}
		if _, exists := s.items[n]; exists {
			continue
		}
		s.items[n] = now.Add(nonceTTL)
		return n
	}
	return ""
}

// Consume reports whether the nonce was live, removing it either way so that a
// replay of the same value fails even if the first attempt was rejected further
// down the chain.
func (s *nonceStore) Consume(nonce string) bool {
	if nonce == "" {
		return false
	}
	now := s.nowFn()

	s.mu.Lock()
	defer s.mu.Unlock()

	exp, ok := s.items[nonce]
	if !ok {
		return false
	}
	delete(s.items, nonce)
	return now.Before(exp)
}

// sweepLocked drops expired entries. It runs at most once a minute during
// normal operation, and unconditionally once the store is at its cap so that a
// burst of expired challenges cannot wedge it.
func (s *nonceStore) sweepLocked(now time.Time) {
	if len(s.items) < s.maxLen && now.Sub(s.sweep) < time.Minute {
		return
	}
	s.sweep = now
	for k, exp := range s.items {
		if !now.Before(exp) {
			delete(s.items, k)
		}
	}
}

// Len reports the number of live entries; used by tests and diagnostics.
func (s *nonceStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}
