package api

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestNonceIssueAndConsume(t *testing.T) {
	s := newNonceStore()
	n := s.Issue()
	if n == "" {
		t.Fatal("Issue returned an empty nonce")
	}
	if len(n) != nonceLen {
		t.Errorf("nonce length = %d, want %d", len(n), nonceLen)
	}
	if !s.Consume(n) {
		t.Error("a freshly issued nonce was rejected")
	}
}

func TestNonceIsSingleUse(t *testing.T) {
	// The whole point of the nonce: a captured login ciphertext must not be
	// replayable.
	s := newNonceStore()
	n := s.Issue()
	if !s.Consume(n) {
		t.Fatal("first use failed")
	}
	if s.Consume(n) {
		t.Error("the same nonce was accepted twice")
	}
}

func TestNonceRejectsUnknownAndEmpty(t *testing.T) {
	s := newNonceStore()
	if s.Consume("never-issued") {
		t.Error("an unissued nonce was accepted")
	}
	if s.Consume("") {
		t.Error("the empty nonce was accepted")
	}
}

func TestNonceExpires(t *testing.T) {
	s := newNonceStore()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s.nowFn = func() time.Time { return now }

	n := s.Issue()
	now = now.Add(nonceTTL + time.Second)
	if s.Consume(n) {
		t.Error("an expired nonce was accepted")
	}
}

func TestNonceIsRemovedEvenWhenExpired(t *testing.T) {
	// Consume deletes on every path, so a rejected nonce cannot be retried in
	// the hope of a clock difference.
	s := newNonceStore()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s.nowFn = func() time.Time { return now }

	n := s.Issue()
	now = now.Add(nonceTTL + time.Second)
	s.Consume(n)
	if s.Len() != 0 {
		t.Errorf("store still holds %d entries after consuming an expired nonce", s.Len())
	}
}

func TestNonceSweepReclaimsExpiredEntries(t *testing.T) {
	s := newNonceStore()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s.nowFn = func() time.Time { return now }

	for i := 0; i < 50; i++ {
		s.Issue()
	}
	if s.Len() != 50 {
		t.Fatalf("Len = %d, want 50", s.Len())
	}
	// Past the TTL, the next Issue triggers a sweep (the throttle is one minute,
	// and the TTL is longer than that).
	now = now.Add(nonceTTL + time.Second)
	s.Issue()
	if s.Len() != 1 {
		t.Errorf("Len = %d after sweep, want 1 (only the new nonce)", s.Len())
	}
}

func TestNonceStoreRefusesToGrowPastCap(t *testing.T) {
	// /auth/challenge is unauthenticated; an attacker looping on it must not be
	// able to exhaust memory.
	s := newNonceStore()
	s.maxLen = 10
	for i := 0; i < 10; i++ {
		if s.Issue() == "" {
			t.Fatalf("Issue %d returned empty below the cap", i)
		}
	}
	if got := s.Issue(); got != "" {
		t.Errorf("Issue past the cap returned %q, want an empty string", got)
	}
	if s.Len() != 10 {
		t.Errorf("Len = %d, want the cap of 10", s.Len())
	}
}

func TestNonceStoreRecoversAfterCapWhenEntriesExpire(t *testing.T) {
	s := newNonceStore()
	s.maxLen = 5
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s.nowFn = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		s.Issue()
	}
	if s.Issue() != "" {
		t.Fatal("expected the store to be full")
	}
	now = now.Add(nonceTTL + time.Second)
	if s.Issue() == "" {
		t.Error("the store stayed full after every entry had expired")
	}
}

func TestNonceCollisionIsNotHandedOutTwice(t *testing.T) {
	// Force the generator to repeat itself and check the retry loop copes.
	s := newNonceStore()
	calls := 0
	s.newFn = func() string {
		calls++
		if calls <= 2 {
			return "same"
		}
		return "different"
	}
	if got := s.Issue(); got != "same" {
		t.Fatalf("first Issue = %q", got)
	}
	if got := s.Issue(); got != "different" {
		t.Errorf("second Issue = %q, want the retry to produce a new value", got)
	}
}

func TestNonceStoreIsConcurrencySafe(t *testing.T) {
	s := newNonceStore()
	const workers = 16
	const perWorker = 50

	var wg sync.WaitGroup
	accepted := make(chan string, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if n := s.Issue(); n != "" {
					accepted <- n
				}
				s.Consume("bogus-" + strconv.Itoa(id) + "-" + strconv.Itoa(i))
			}
		}(w)
	}
	wg.Wait()
	close(accepted)

	// Every issued nonce must be consumable exactly once, from any goroutine.
	seen := 0
	for n := range accepted {
		if s.Consume(n) {
			seen++
		}
	}
	if seen == 0 {
		t.Error("no issued nonce could be consumed")
	}
	if s.Len() != 0 {
		t.Errorf("Len = %d after consuming everything, want 0", s.Len())
	}
}
