package appconfig

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Entry is one key/value pair to be written into the config file, with an
// optional comment placed on the line above it.
type Entry struct {
	Key     string
	Value   string
	Comment string
}

// AppendKeys writes entries into the ini file at path, then updates the live
// configuration so the new values are readable immediately.
//
// The underlying config library also offers SaveConfigFile, but it re-emits the
// file from a map: comments survive only where they were attached to a key, and
// both key and section order are lost. conf/nps.conf is a hand-maintained,
// bilingually commented document that operators edit directly, so it is
// appended to as text instead. Nothing already in the file is touched.
//
// Keys already present in the file are skipped rather than duplicated: the ini
// parser keeps the last occurrence, so writing a second copy would silently
// override an operator's value.
func AppendKeys(path string, entries []Entry) error {
	if path == "" {
		return errors.New("appconfig: no config path is known")
	}
	if len(entries) == 0 {
		return nil
	}

	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("appconfig: read %s: %w", path, err)
	}

	present := existingKeys(original)
	var block bytes.Buffer
	for _, e := range entries {
		key := strings.ToLower(strings.TrimSpace(e.Key))
		if key == "" || present[key] {
			continue
		}
		block.WriteByte('\n')
		if e.Comment != "" {
			for _, line := range strings.Split(e.Comment, "\n") {
				block.WriteString("# " + line + "\n")
			}
		}
		block.WriteString(key + "=" + e.Value + "\n")
	}
	if block.Len() == 0 {
		return nil
	}

	updated := insertBeforeFirstSection(original, block.Bytes())
	if err := writeFileAtomic(path, updated); err != nil {
		return err
	}

	// Reflect the change in the running process without a reload. Set is
	// best-effort here: the file is the source of truth and has already been
	// written, so a failure to update the in-memory copy is not fatal.
	cfg := AppConfig()
	for _, e := range entries {
		key := strings.ToLower(strings.TrimSpace(e.Key))
		if key != "" && !present[key] {
			_ = cfg.Set(key, e.Value)
		}
	}
	return nil
}

// existingKeys collects the lower-cased keys already defined anywhere in the
// file, so a rewrite never shadows a value an operator set by hand.
func existingKeys(data []byte) map[string]bool {
	keys := make(map[string]bool)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") ||
			strings.HasPrefix(line, "[") {
			continue
		}
		if k, _, ok := strings.Cut(line, "="); ok {
			keys[strings.ToLower(strings.TrimSpace(k))] = true
		}
	}
	return keys
}

// insertBeforeFirstSection places block in the file's default section: before
// the first `[section]` header if there is one, at the end otherwise. Appending
// blindly would put the keys inside whichever section happens to come last,
// where the parser would not find them under their bare names.
func insertBeforeFirstSection(data, block []byte) []byte {
	var out bytes.Buffer
	inserted := false

	lines := strings.SplitAfter(string(data), "\n")
	for _, line := range lines {
		if !inserted && strings.HasPrefix(strings.TrimSpace(line), "[") {
			out.Write(block)
			out.WriteByte('\n')
			inserted = true
		}
		out.WriteString(line)
	}

	if !inserted {
		// Guarantee the appended block starts on a line of its own even if the
		// file did not end with a newline.
		if out.Len() > 0 && !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
			out.WriteByte('\n')
		}
		out.Write(block)
	}
	return out.Bytes()
}

// writeFileAtomic replaces path via a temporary file in the same directory, so
// a crash or a full disk mid-write cannot leave the operator with a truncated
// nps.conf and an unbootable server.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("appconfig: create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Harmless once the rename has succeeded, and the cleanup that matters
		// on every failure path below.
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("appconfig: write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("appconfig: sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("appconfig: close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("appconfig: chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("appconfig: replace %s: %w", path, err)
	}
	return nil
}
