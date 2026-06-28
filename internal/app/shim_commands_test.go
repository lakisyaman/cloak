package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lakisyaman/cloak/internal/contextstore"
)

func TestShimInstallListAndUninstall(t *testing.T) {
	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shims")
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("create real dir: %v", err)
	}
	cloakBinary := filepath.Join(dir, "cloak")
	writeTestExecutable(t, cloakBinary)
	writeTestExecutable(t, filepath.Join(realDir, "psql"))

	paths := contextstore.Paths{
		Dir:        dir,
		ConfigFile: filepath.Join(dir, "config.json"),
		StateFile:  filepath.Join(dir, "state.json"),
		ShimDir:    shimDir,
	}
	store := fileContextStore{configPath: paths.ConfigFile, statePath: paths.StateFile}
	if err := store.WriteConfig(contextstore.EmptyConfig()); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := store.WriteState(contextstore.EmptyState()); err != nil {
		t.Fatalf("write state: %v", err)
	}
	env := CommandEnv{
		Paths:           paths,
		Store:           store,
		Secrets:         fakeSecretStore{},
		Resolver:        fakeResolver{path: filepath.Join(realDir, "psql")},
		PathEnv:         shimDir + string(os.PathListSeparator) + realDir,
		CloakBinaryPath: cloakBinary,
	}

	var installOut bytes.Buffer
	install := NewRootCommandWithEnv("test", env)
	install.SetArgs([]string{"shim", "install", "psql"})
	install.SetOut(&installOut)
	if err := install.Execute(); err != nil {
		t.Fatalf("shim install: %v", err)
	}
	if !strings.Contains(installOut.String(), "installed shim") {
		t.Fatalf("unexpected install output: %q", installOut.String())
	}
	if target, err := os.Readlink(filepath.Join(shimDir, "psql")); err != nil || target != cloakBinary {
		t.Fatalf("expected psql shim symlink to %q, got target=%q err=%v", cloakBinary, target, err)
	}

	var listOut bytes.Buffer
	list := NewRootCommandWithEnv("test", env)
	list.SetArgs([]string{"shim", "list"})
	list.SetOut(&listOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("shim list: %v", err)
	}
	if strings.TrimSpace(listOut.String()) != "psql" {
		t.Fatalf("expected psql shim list, got %q", listOut.String())
	}

	uninstall := NewRootCommandWithEnv("test", env)
	uninstall.SetArgs([]string{"shim", "uninstall", "psql"})
	if err := uninstall.Execute(); err != nil {
		t.Fatalf("shim uninstall: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(shimDir, "psql")); !os.IsNotExist(err) {
		t.Fatalf("expected psql shim to be removed, err=%v", err)
	}
}

func writeTestExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func TestShimDirCommandPrintsShimDirectory(t *testing.T) {
	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shims")
	env := CommandEnv{
		Paths: contextstore.Paths{
			Dir:        dir,
			ConfigFile: filepath.Join(dir, "config.json"),
			StateFile:  filepath.Join(dir, "state.json"),
			ShimDir:    shimDir,
		},
	}

	var out bytes.Buffer
	cmd := NewRootCommandWithEnv("test", env)
	cmd.SetArgs([]string{"shim", "dir"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("shim dir: %v", err)
	}
	if strings.TrimSpace(out.String()) != shimDir {
		t.Fatalf("expected shim dir %q, got %q", shimDir, out.String())
	}
}

func TestShimInstallPrintsPathHintWhenShimDirNotAhead(t *testing.T) {
	env, shimDir, realDir := newShimInstallTestEnv(t)
	env.PathEnv = realDir + string(os.PathListSeparator) + shimDir // shim after real

	var out bytes.Buffer
	cmd := NewRootCommandWithEnv("test", env)
	cmd.SetArgs([]string{"shim", "install", "psql"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("shim install: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "export PATH=") || !strings.Contains(got, shimDir) {
		t.Fatalf("expected PATH hint mentioning %q, got %q", shimDir, got)
	}
}

func TestShimInstallOmitsPathHintWhenShimDirAhead(t *testing.T) {
	env, shimDir, realDir := newShimInstallTestEnv(t)
	env.PathEnv = shimDir + string(os.PathListSeparator) + realDir // shim ahead of real

	var out bytes.Buffer
	cmd := NewRootCommandWithEnv("test", env)
	cmd.SetArgs([]string{"shim", "install", "psql"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("shim install: %v", err)
	}
	if strings.Contains(out.String(), "export PATH=") {
		t.Fatalf("did not expect PATH hint, got %q", out.String())
	}
}

func newShimInstallTestEnv(t *testing.T) (CommandEnv, string, string) {
	t.Helper()
	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shims")
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("create real dir: %v", err)
	}
	cloakBinary := filepath.Join(dir, "cloak")
	writeTestExecutable(t, cloakBinary)
	writeTestExecutable(t, filepath.Join(realDir, "psql"))
	paths := contextstore.Paths{
		Dir:        dir,
		ConfigFile: filepath.Join(dir, "config.json"),
		StateFile:  filepath.Join(dir, "state.json"),
		ShimDir:    shimDir,
	}
	store := fileContextStore{configPath: paths.ConfigFile, statePath: paths.StateFile}
	if err := store.WriteConfig(contextstore.EmptyConfig()); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := store.WriteState(contextstore.EmptyState()); err != nil {
		t.Fatalf("write state: %v", err)
	}
	return CommandEnv{
		Paths:           paths,
		Store:           store,
		Secrets:         fakeSecretStore{},
		Resolver:        fakeResolver{path: filepath.Join(realDir, "psql")},
		CloakBinaryPath: cloakBinary,
	}, shimDir, realDir
}
