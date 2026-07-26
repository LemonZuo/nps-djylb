package appconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/beego/beego/config"
)

const sample = `appname=nps
runmode=pro
web_port=8888
open_captcha=true
pow_bits=20
empty_value=
allow_ports=9001-9009,10001,11000-12000

[pro]
web_port=9999
`

func writeSample(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nps.conf")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write sample config: %v", err)
	}
	return path
}

func TestLoadAppConfig(t *testing.T) {
	path := writeSample(t, sample)
	if err := LoadAppConfig("ini", path); err != nil {
		t.Fatalf("LoadAppConfig: %v", err)
	}
	c := AppConfig()

	// runmode=pro means the [pro] section shadows the default section.
	if got := c.DefaultInt("web_port", 0); got != 9999 {
		t.Errorf("web_port = %d, want 9999 (run-mode section must win)", got)
	}
	if got := RunMode(); got != "pro" {
		t.Errorf("RunMode() = %q, want %q", got, "pro")
	}
	if got := c.String("appname"); got != "nps" {
		t.Errorf("appname = %q, want %q", got, "nps")
	}
	if !c.DefaultBool("open_captcha", false) {
		t.Error("open_captcha = false, want true")
	}
	if got := c.DefaultInt("pow_bits", 0); got != 20 {
		t.Errorf("pow_bits = %d, want 20", got)
	}
	if got := Path(); got != path {
		t.Errorf("Path() = %q, want %q", got, path)
	}
}

func TestKeysAreCaseInsensitive(t *testing.T) {
	if err := LoadAppConfig("ini", writeSample(t, sample)); err != nil {
		t.Fatalf("LoadAppConfig: %v", err)
	}
	// The ini adapter lower-cases keys both at parse time and at lookup time,
	// so config keys are case-insensitive in either direction.
	for _, key := range []string{"appname", "APPNAME", "AppName"} {
		if got := AppConfig().String(key); got != "nps" {
			t.Errorf("String(%q) = %q, want %q", key, got, "nps")
		}
	}
	// Section names are folded the same way.
	if got := AppConfig().DefaultInt("PRO::web_port", 0); got != 9999 {
		t.Errorf("DefaultInt(%q) = %d, want 9999", "PRO::web_port", got)
	}
}

func TestDefaultsOnMissingAndEmpty(t *testing.T) {
	if err := LoadAppConfig("ini", writeSample(t, sample)); err != nil {
		t.Fatalf("LoadAppConfig: %v", err)
	}
	c := AppConfig()
	if got := c.DefaultString("no_such_key", "fallback"); got != "fallback" {
		t.Errorf("missing key = %q, want %q", got, "fallback")
	}
	// An empty value is indistinguishable from a missing one, matching beego.
	if got := c.DefaultString("empty_value", "fallback"); got != "fallback" {
		t.Errorf("empty value = %q, want %q", got, "fallback")
	}
	if got := c.DefaultInt64("no_such_key", 42); got != 42 {
		t.Errorf("missing int64 = %d, want 42", got)
	}
	if got := c.DefaultBool("no_such_key", true); !got {
		t.Error("missing bool = false, want true")
	}
}

func TestLoadAppConfigMissingFile(t *testing.T) {
	err := LoadAppConfig("ini", filepath.Join(t.TempDir(), "absent.conf"))
	if err == nil {
		t.Fatal("LoadAppConfig on a missing file returned nil error")
	}
}

func TestAppConfigBeforeLoadIsUsable(t *testing.T) {
	// A fresh container (as installed by init) must serve defaults rather than
	// panic, because several packages read config during their own init.
	c := &container{inner: config.NewFakeConfig(), runMode: defaultRunMode}
	if got := c.DefaultString("web_ip", "0.0.0.0"); got != "0.0.0.0" {
		t.Errorf("DefaultString on empty config = %q, want %q", got, "0.0.0.0")
	}
	if got := c.DefaultInt("web_port", 8080); got != 8080 {
		t.Errorf("DefaultInt on empty config = %d, want 8080", got)
	}
}
