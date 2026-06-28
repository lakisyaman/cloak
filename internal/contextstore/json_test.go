package contextstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStateWritesVersionedJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "state.json")

	if err := WriteState(path, State{ActiveContexts: map[string]string{"psql": "production"}}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat private data directory: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected private data directory mode 0700, got %o", got)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !strings.Contains(string(contents), `"version": 1`) {
		t.Fatalf("expected version in state json, got: %s", contents)
	}

	state, err := ReadState(path)
	if err != nil {
		t.Fatalf("read state through store: %v", err)
	}
	if state.ActiveContexts["psql"] != "production" {
		t.Fatalf("expected psql active context production, got %q", state.ActiveContexts["psql"])
	}
}
