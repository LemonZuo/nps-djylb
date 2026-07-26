package api

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/djylb/nps/lib/common"
)

// TestMain redirects the JSON database at a scratch directory.
//
// lib/file.GetDb() resolves its storage location once, from
// common.GetRunPath(), and immediately creates and reads the four JSON files
// there. Left alone that is /etc/nps on this platform, so the first test that
// touches a client would either panic on a permission error or, worse, edit the
// machine's real database. Pointing common.ConfPath somewhere writable before
// any test runs makes the choice deterministic.
func TestMain(m *testing.M) {
	code, err := runTests(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test setup:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runTests(m *testing.M) (int, error) {
	dir, err := os.MkdirTemp("", "nps-api-test")
	if err != nil {
		return 0, err
	}
	// Deferred so the directory still goes away when a test fails; os.Exit in
	// TestMain would skip it.
	defer func() { _ = os.RemoveAll(dir) }()

	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o700); err != nil {
		return 0, err
	}

	prev := common.ConfPath
	common.ConfPath = dir
	defer func() { common.ConfPath = prev }()

	return m.Run(), nil
}
