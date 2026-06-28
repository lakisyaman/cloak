package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloak/internal/contextstore"
)

func TestDoctorCommandPrintsFindings(t *testing.T) {
	dir := t.TempDir()
	paths := contextstore.Paths{
		Dir:        dir,
		ConfigFile: filepath.Join(dir, "config.json"),
		StateFile:  filepath.Join(dir, "state.json"),
		ShimDir:    filepath.Join(dir, "shims"),
	}
	if err := os.Mkdir(paths.ShimDir, 0o700); err != nil {
		t.Fatalf("create shim dir: %v", err)
	}
	if err := contextstore.WriteConfig(paths.ConfigFile, contextstore.EmptyConfig()); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := contextstore.WriteState(paths.StateFile, contextstore.EmptyState()); err != nil {
		t.Fatalf("write state: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := NewRootCommandWithEnv("test", CommandEnv{
		Paths:           paths,
		Store:           fileContextStore{configPath: paths.ConfigFile, statePath: paths.StateFile},
		Secrets:         fakeSecretStore{},
		Resolver:        fakeResolver{path: "/real/psql"},
		PathEnv:         os.Getenv("PATH"),
		CloakBinaryPath: os.Args[0],
	})
	cmd.SetArgs([]string{"doctor"})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("doctor command returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "state is consistent") {
		t.Fatalf("expected doctor findings in stdout, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRootCommandIncludesStandaloneContextShow(t *testing.T) {
	root := NewRootCommand("test")
	contextCmd, _, err := root.Find([]string{"context"})
	if err != nil {
		t.Fatalf("find context command: %v", err)
	}
	if contextCmd == nil {
		t.Fatalf("context command not found")
	}

	showCmd, _, err := root.Find([]string{"context", "show"})
	if err != nil {
		t.Fatalf("find context show command: %v", err)
	}
	if showCmd == nil {
		t.Fatalf("context show command not found")
	}
}
