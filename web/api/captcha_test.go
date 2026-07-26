package api

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCaptchaRoundTrip(t *testing.T) {
	s := newCaptchaStore()
	ch, err := s.issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if ch.ID == "" {
		t.Error("no challenge id")
	}
	if !strings.HasPrefix(ch.Image, "data:image/png;base64,") {
		t.Errorf("image is not a PNG data URI: %.40q", ch.Image)
	}

	code := s.items[ch.ID].code
	if len(code) != captchaDigits {
		t.Fatalf("code %q has %d digits, want %d", code, len(code), captchaDigits)
	}
	if !s.verify(ch.ID, code) {
		t.Error("the correct code was rejected")
	}
}

func TestCaptchaImageDecodesAtTheRightSize(t *testing.T) {
	s := newCaptchaStore()
	ch, err := s.issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ch.Image, "data:image/png;base64,"))
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != captchaW || b.Dy() != captchaH {
		t.Errorf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), captchaW, captchaH)
	}
}

func TestCaptchaIsSingleUse(t *testing.T) {
	// A wrong guess must not be retryable against the same image, or the
	// challenge degenerates into an offline 4-digit search.
	s := newCaptchaStore()
	ch, _ := s.issue()
	code := s.items[ch.ID].code

	if s.verify(ch.ID, "0000") && code != "0000" {
		t.Fatal("a wrong code was accepted")
	}
	if s.verify(ch.ID, code) {
		t.Error("the challenge survived a failed attempt")
	}
}

func TestCaptchaRejectsWrongAndUnknown(t *testing.T) {
	s := newCaptchaStore()
	ch, _ := s.issue()
	code := s.items[ch.ID].code

	wrong := "1234"
	if wrong == code {
		wrong = "5678"
	}
	if s.verify(ch.ID, wrong) {
		t.Error("a wrong code was accepted")
	}
	if s.verify("no-such-id", code) {
		t.Error("an unknown id was accepted")
	}
	if s.verify("", "") {
		t.Error("empty input was accepted")
	}
}

func TestCaptchaTrimsWhitespace(t *testing.T) {
	s := newCaptchaStore()
	ch, _ := s.issue()
	code := s.items[ch.ID].code
	if !s.verify(ch.ID, "  "+code+" \n") {
		t.Error("a padded code was rejected")
	}
}

func TestCaptchaExpires(t *testing.T) {
	s := newCaptchaStore()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s.nowFn = func() time.Time { return now }

	ch, _ := s.issue()
	code := s.items[ch.ID].code
	now = now.Add(captchaTTL + time.Second)
	if s.verify(ch.ID, code) {
		t.Error("an expired challenge was accepted")
	}
}

func TestCaptchaStoreRefusesToGrowPastCap(t *testing.T) {
	s := newCaptchaStore()
	s.maxLen = 5
	for i := 0; i < 5; i++ {
		if _, err := s.issue(); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if _, err := s.issue(); err == nil {
		t.Error("the store grew past its cap")
	}
	if s.len() != 5 {
		t.Errorf("len = %d, want 5", s.len())
	}
}

func TestCaptchaStoreSweepsExpiredEntries(t *testing.T) {
	s := newCaptchaStore()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s.nowFn = func() time.Time { return now }

	for i := 0; i < 20; i++ {
		if _, err := s.issue(); err != nil {
			t.Fatalf("issue: %v", err)
		}
	}
	now = now.Add(captchaTTL + time.Second)
	if _, err := s.issue(); err != nil {
		t.Fatalf("issue after expiry: %v", err)
	}
	if s.len() != 1 {
		t.Errorf("len = %d after sweep, want 1", s.len())
	}
}

func TestCaptchaCodesVary(t *testing.T) {
	// A constant answer would make the control useless; this catches a broken
	// or unseeded random source.
	s := newCaptchaStore()
	seen := make(map[string]int)
	for i := 0; i < 100; i++ {
		ch, err := s.issue()
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		seen[s.items[ch.ID].code]++
	}
	if len(seen) < 50 {
		t.Errorf("only %d distinct codes in 100 challenges", len(seen))
	}
}

func TestCaptchaIDsAreUnique(t *testing.T) {
	s := newCaptchaStore()
	ids := make(map[string]bool)
	for i := 0; i < 200; i++ {
		ch, err := s.issue()
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		if ids[ch.ID] {
			t.Fatalf("id %q was reused", ch.ID)
		}
		ids[ch.ID] = true
	}
}

func TestRenderCaptchaHandlesOddInput(t *testing.T) {
	// renderCaptcha must not panic on input the store would never produce.
	for _, code := range []string{"", "1", "1234567890", "abcd"} {
		if _, err := renderCaptcha(code); err != nil {
			t.Errorf("renderCaptcha(%q): %v", code, err)
		}
	}
}

func TestCaptchaStoreIsConcurrencySafe(t *testing.T) {
	s := newCaptchaStore()
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				ch, err := s.issue()
				if err != nil {
					continue
				}
				s.verify(ch.ID, "0000")
			}
		}()
	}
	wg.Wait()
}
