package agentsetup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGlobalDiscoveryAndExplicitCreation(t *testing.T) {
	home := t.TempDir()
	options := Options{Global: true, HomeDir: home}
	paths, err := Targets(options)
	if err != nil || len(paths) != 0 {
		t.Fatal(paths, err)
	}
	files := []string{".codex/AGENTS.md", ".claude/CLAUDE.md", ".gemini/GEMINI.md", ".copilot/copilot-instructions.md"}
	var want []string
	for _, file := range files {
		path := filepath.Join(home, file)
		want = append(want, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err = Targets(options)
	if err != nil || !reflect.DeepEqual(paths, want) {
		t.Fatal(paths, err)
	}
	options.HomeDir = t.TempDir()
	options.Agents = []string{"gemini", "copilot"}
	paths, err = Targets(options)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := Prepare(paths, options)
	if err != nil || len(changes) != 2 {
		t.Fatal(changes, err)
	}
	for _, change := range changes {
		status, err := change.Write(true)
		if err != nil || status != "created" {
			t.Fatal(status, err)
		}
		info, err := os.Stat(filepath.Dir(change.Path))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatal(info, err)
		}
	}
}
