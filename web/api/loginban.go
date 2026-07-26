package api

import (
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/djylb/nps/lib/logs"
)

// Login throttling, carried over from the Beego controller unchanged in
// behaviour. Two independent counters are kept: one keyed by client IP and one
// keyed by the attempted username. The IP counter stops a single host brute
// forcing many accounts; the username counter stops a botnet brute forcing one
// account. Both also enforce a short fixed delay between consecutive attempts.

// banRecord counts consecutive failures for one key.
type banRecord struct {
	mu        sync.Mutex
	failTimes int
	lastTry   time.Time
}

// BanEntry is one row of the ban list shown to an administrator.
type BanEntry struct {
	Key       string `json:"key"`
	FailTimes int    `json:"failTimes"`
	LastTry   string `json:"lastTry"`
	Banned    bool   `json:"banned"`
	Type      string `json:"type"` // "ip" or "username"
}

var (
	loginRecords sync.Map // key -> *banRecord
	banCleanerOn sync.Once
)

// StartLoginBanCleaner launches the periodic sweep of expired records. It is
// safe to call more than once; only the first call starts the goroutine.
func StartLoginBanCleaner() {
	banCleanerOn.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				CleanBanRecords(true)
			}
		}()
	})
}

// NoteLoginFailure records a failed attempt for key. Passing explicit=false is
// how implicit attempts (an anonymous probe of a no-auth install) avoid
// poisoning the counter.
func NoteLoginFailure(key string, explicit bool) {
	if !explicit || key == "" {
		return
	}
	now := time.Now()
	v, loaded := loginRecords.LoadOrStore(key, &banRecord{failTimes: 1, lastTry: now})
	if loaded {
		r := v.(*banRecord)
		r.mu.Lock()
		r.lastTry = now
		r.failTimes++
		r.mu.Unlock()
	}
}

// ClearLoginFailures forgets a key, called after a successful login.
func ClearLoginFailures(key string) {
	if key != "" {
		loginRecords.Delete(key)
	}
}

// IsLoginBanned reports whether key is currently blocked. banWindow is how long
// the failure counter stays hot: once that much time has passed since the last
// attempt the counter resets, so a ban lifts on its own.
func IsLoginBanned(key string, banWindow int64) bool {
	if key == "" {
		return false
	}
	v, ok := loginRecords.Load(key)
	if !ok {
		return false
	}
	r := v.(*banRecord)
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsed := time.Now().Unix() - r.lastTry.Unix()

	// A minimum gap between attempts, applied even to the first failure. This is
	// what makes an online guessing attack impractical regardless of the counter.
	if elapsed < LoginBanTime() {
		logs.Warn("api: %s request rate too high, login blocked", key)
		return true
	}
	if elapsed >= banWindow {
		r.failTimes = 0
	}
	if r.failTimes >= LoginMaxFailTimes() {
		logs.Warn("api: %s has reached maximum failed attempts, login blocked", key)
		return true
	}
	return false
}

// ListLoginBans returns every tracked key with its current state.
func ListLoginBans() []BanEntry {
	list := make([]BanEntry, 0)
	now := time.Now()
	minGap := LoginBanTime()
	maxFail := LoginMaxFailTimes()

	loginRecords.Range(func(key, value any) bool {
		k := key.(string)
		r := value.(*banRecord)

		entryType, window := "username", LoginUserBanTime()
		if net.ParseIP(k) != nil {
			entryType, window = "ip", LoginIPBanTime()
		}

		r.mu.Lock()
		fail, last := r.failTimes, r.lastTry
		r.mu.Unlock()

		elapsed := now.Unix() - last.Unix()
		list = append(list, BanEntry{
			Key:       k,
			FailTimes: fail,
			LastTry:   last.Format("2006-01-02 15:04:05"),
			Banned:    elapsed < minGap || (fail >= maxFail && elapsed < window),
			Type:      entryType,
		})
		return true
	})
	return list
}

// RemoveLoginBan lifts the ban on one key, reporting whether it existed.
func RemoveLoginBan(key string) bool {
	if key == "" {
		return false
	}
	if _, ok := loginRecords.Load(key); ok {
		loginRecords.Delete(key)
		return true
	}
	return false
}

// RemoveAllLoginBans clears every tracked key.
func RemoveAllLoginBans() {
	loginRecords.Range(func(key, _ any) bool {
		loginRecords.Delete(key)
		return true
	})
}

// CleanBanRecords drops records whose window has elapsed. force runs the sweep
// unconditionally; otherwise it runs on roughly 1% of calls, which keeps the
// cost off the login hot path while still bounding the map on a busy server.
func CleanBanRecords(force bool) {
	if !force && rand.Intn(100) != 1 {
		return
	}
	now := time.Now()
	userWindow, ipWindow := LoginUserBanTime(), LoginIPBanTime()

	loginRecords.Range(func(key, value any) bool {
		k := key.(string)
		r := value.(*banRecord)

		window := userWindow
		if net.ParseIP(k) != nil {
			window = ipWindow
		}

		r.mu.Lock()
		last := r.lastTry
		r.mu.Unlock()

		if now.Unix()-last.Unix() >= window {
			loginRecords.Delete(k)
		}
		return true
	})
}
