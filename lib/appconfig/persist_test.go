package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConf(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nps.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	return path
}

func TestAppendKeysPreservesExistingContent(t *testing.T) {
	body := "# leading comment\nweb_port=8888\n\n[dev]\nweb_port=9999\n"
	path := writeConf(t, body)

	if err := AppendKeys(path, []Entry{{Key: "api_jwt_key", Value: "deadbeef", Comment: "generated"}}); err != nil {
		t.Fatalf("AppendKeys: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	out := string(got)

	for _, want := range []string{"# leading comment", "web_port=8888", "[dev]", "api_jwt_key=deadbeef", "# generated"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// The new key must land in the default section, not inside [dev], or a
	// lookup of the bare name would miss it.
	if strings.Index(out, "api_jwt_key=") > strings.Index(out, "[dev]") {
		t.Errorf("key was written after the first section header:\n%s", out)
	}
}

func TestAppendKeysIsReadableByTheParser(t *testing.T) {
	path := writeConf(t, "web_port=8888\n")
	if err := AppendKeys(path, []Entry{{Key: "api_jwt_key", Value: "abc123"}}); err != nil {
		t.Fatalf("AppendKeys: %v", err)
	}
	if err := LoadAppConfig("ini", path); err != nil {
		t.Fatalf("LoadAppConfig: %v", err)
	}
	if got := AppConfig().String("api_jwt_key"); got != "abc123" {
		t.Errorf("api_jwt_key = %q, want %q", got, "abc123")
	}
}

func TestAppendKeysSkipsExistingKey(t *testing.T) {
	path := writeConf(t, "api_jwt_key=operator-chose-this\n")
	if err := AppendKeys(path, []Entry{{Key: "api_jwt_key", Value: "generated"}}); err != nil {
		t.Fatalf("AppendKeys: %v", err)
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "generated") {
		t.Errorf("an existing key was overwritten:\n%s", out)
	}
	if n := strings.Count(string(out), "api_jwt_key"); n != 1 {
		t.Errorf("api_jwt_key appears %d times, want 1:\n%s", n, out)
	}
}

func TestAppendKeysSkipsKeyDefinedInASection(t *testing.T) {
	// A key set under [dev] still counts as defined: silently adding a default
	// one would change which value that run mode sees.
	path := writeConf(t, "web_port=8888\n[dev]\napi_jwt_key=x\n")
	if err := AppendKeys(path, []Entry{{Key: "api_jwt_key", Value: "generated"}}); err != nil {
		t.Fatalf("AppendKeys: %v", err)
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "generated") {
		t.Errorf("key defined in a section was duplicated:\n%s", out)
	}
}

func TestAppendKeysHandlesFileWithoutSectionsOrTrailingNewline(t *testing.T) {
	path := writeConf(t, "web_port=8888")
	if err := AppendKeys(path, []Entry{{Key: "api_jwt_key", Value: "v"}}); err != nil {
		t.Fatalf("AppendKeys: %v", err)
	}
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), "\napi_jwt_key=v") {
		t.Errorf("key was not placed on its own line:\n%q", out)
	}
}

func TestAppendKeysPreservesPermissions(t *testing.T) {
	path := writeConf(t, "web_port=8888\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := AppendKeys(path, []Entry{{Key: "api_jwt_key", Value: "v"}}); err != nil {
		t.Fatalf("AppendKeys: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", st.Mode().Perm())
	}
}

func TestAppendKeysLeavesNoTempFileBehind(t *testing.T) {
	path := writeConf(t, "web_port=8888\n")
	if err := AppendKeys(path, []Entry{{Key: "api_jwt_key", Value: "v"}}); err != nil {
		t.Fatalf("AppendKeys: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory contains %v, want only nps.conf", names)
	}
}

func TestAppendKeysErrors(t *testing.T) {
	if err := AppendKeys("", []Entry{{Key: "k", Value: "v"}}); err == nil {
		t.Error("empty path was accepted")
	}
	if err := AppendKeys(filepath.Join(t.TempDir(), "missing.conf"), []Entry{{Key: "k", Value: "v"}}); err == nil {
		t.Error("missing file was accepted")
	}
	// Nothing to do is not an error.
	path := writeConf(t, "web_port=8888\n")
	if err := AppendKeys(path, nil); err != nil {
		t.Errorf("empty entry list: %v", err)
	}
}
