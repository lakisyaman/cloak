package shim

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRealCommandResolverSkipsCloakSymlink(t *testing.T) {
	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shims")
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatalf("create shim dir: %v", err)
	}
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("create real dir: %v", err)
	}

	cloakBinary := filepath.Join(dir, "cloak")
	writeExecutable(t, cloakBinary)

	shimPath := filepath.Join(shimDir, "psql")
	if err := os.Symlink(cloakBinary, shimPath); err != nil {
		t.Fatalf("create shim symlink: %v", err)
	}

	realPath := filepath.Join(realDir, "psql")
	writeExecutable(t, realPath)

	resolver := RealCommandResolver{
		PathEnv:         shimDir + string(os.PathListSeparator) + realDir,
		CloakBinaryPath: cloakBinary,
	}

	got, err := resolver.Resolve("psql")
	if err != nil {
		t.Fatalf("resolve real command: %v", err)
	}
	if got != realPath {
		t.Fatalf("expected resolver to skip shim and return %q, got %q", realPath, got)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}
