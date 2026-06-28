package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloak/internal/contextstore"
	"cloak/internal/secrets"

	"github.com/zalando/go-keyring"
)

type fakeSecretChecker struct {
	available bool
	existing  map[string]bool
	errors    map[string]error
}

func (checker fakeSecretChecker) Available() error {
	if checker.available {
		return nil
	}
	return os.ErrNotExist
}

func (checker fakeSecretChecker) Exists(ref secrets.SecretRef) (bool, error) {
	key := ref.Service + "/" + ref.User
	if err := checker.errors[key]; err != nil {
		return false, err
	}
	return checker.existing[key], nil
}

func TestRunReportsMissingSecretMaterial(t *testing.T) {
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

	if err := contextstore.WriteConfig(paths.ConfigFile, contextstore.Config{
		ManagedCLIs: map[string]contextstore.ManagedCLIConfig{
			"psql": {
				Contexts: map[string]contextstore.Context{
					"production": {
						Metadata: map[string]any{"host": "db.example.com"},
						Secrets: map[string]secrets.SecretRef{
							"password": secrets.NewRef("psql", "production", "password"),
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := contextstore.WriteState(paths.StateFile, contextstore.State{
		ActiveContexts: map[string]string{"psql": "production"},
	}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	findings := Run(Options{
		Paths:           paths,
		PathEnv:         os.Getenv("PATH"),
		CloakBinaryPath: os.Args[0],
		Supported:       SupportedCLIFunc(func(managedCLI string) bool { return managedCLI == "psql" }),
		SupportedNames:  []string{"psql"},
		SecretChecker:   fakeSecretChecker{available: true, existing: map[string]bool{}},
	})

	if !hasFinding(findings, SeverityError, "missing Secret Material for psql/production/password") {
		t.Fatalf("expected missing Secret Material finding, got %#v", findings)
	}
}

func TestRunReportsUnreadableSecretMaterialSeparatelyFromMissingSecretMaterial(t *testing.T) {
	ref := secrets.NewRef("psql", "production", "password")
	config := contextstore.Config{Version: contextstore.Version, ManagedCLIs: map[string]contextstore.ManagedCLIConfig{
		"psql": {Contexts: map[string]contextstore.Context{
			"production": {Secrets: map[string]secrets.SecretRef{"password": ref}},
		}},
	}}

	missing := checkSecretRefs(config, fakeSecretChecker{errors: map[string]error{ref.Service + "/" + ref.User: keyring.ErrNotFound}})
	if !hasFinding(missing, SeverityError, "missing Secret Material for psql/production/password") {
		t.Fatalf("expected missing Secret Material finding, got %#v", missing)
	}

	unreadable := checkSecretRefs(config, fakeSecretChecker{errors: map[string]error{ref.Service + "/" + ref.User: errors.New("permission denied")}})
	if !hasFinding(unreadable, SeverityError, "Secret Material for psql/production/password is unreadable") {
		t.Fatalf("expected unreadable Secret Material finding, got %#v", unreadable)
	}
}

func TestRunReportsShimSymlinkAndPathOrdering(t *testing.T) {
	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shims")
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(shimDir, 0o700); err != nil {
		t.Fatalf("create shim dir: %v", err)
	}
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("create real dir: %v", err)
	}

	cloakBinary := filepath.Join(dir, "cloak")
	writeExecutable(t, cloakBinary)
	if err := os.Symlink(cloakBinary, filepath.Join(shimDir, "psql")); err != nil {
		t.Fatalf("create shim symlink: %v", err)
	}
	writeExecutable(t, filepath.Join(realDir, "psql"))

	paths := contextstore.Paths{
		Dir:        dir,
		ConfigFile: filepath.Join(dir, "config.json"),
		StateFile:  filepath.Join(dir, "state.json"),
		ShimDir:    shimDir,
	}
	if err := contextstore.WriteConfig(paths.ConfigFile, contextstore.EmptyConfig()); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := contextstore.WriteState(paths.StateFile, contextstore.EmptyState()); err != nil {
		t.Fatalf("write state: %v", err)
	}

	findings := Run(Options{
		Paths:           paths,
		PathEnv:         shimDir + string(os.PathListSeparator) + realDir,
		CloakBinaryPath: cloakBinary,
		Supported:       SupportedCLIFunc(func(managedCLI string) bool { return managedCLI == "psql" }),
		SecretChecker:   fakeSecretChecker{available: true, existing: map[string]bool{}},
	})

	if !hasFinding(findings, SeverityOK, "Shim points to Cloak binary") {
		t.Fatalf("expected Shim symlink ok finding, got %#v", findings)
	}
	if !hasFinding(findings, SeverityOK, "shim directory appears before Real Command in PATH for psql") {
		t.Fatalf("expected PATH ordering ok finding, got %#v", findings)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func TestPathIndexNormalizesTrailingSlashes(t *testing.T) {
	dir := t.TempDir()
	pathEnv := dir + string(os.PathSeparator)
	if got := pathIndex(pathEnv, dir); got != 0 {
		t.Fatalf("expected normalized path index 0, got %d", got)
	}
}

func TestDoctorFindingTextIsPrintable(t *testing.T) {
	finding := Finding{Severity: SeverityWarning, Message: "example"}
	if !strings.Contains(string(finding.Severity)+": "+finding.Message, "warning: example") {
		t.Fatalf("unexpected printable finding: %#v", finding)
	}
}
