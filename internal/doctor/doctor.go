package doctor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
	"github.com/lakisyaman/cloak/internal/shim"

	"github.com/zalando/go-keyring"
)

type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Finding struct {
	Severity Severity
	Message  string
}

type Options struct {
	Paths           contextstore.Paths
	PathEnv         string
	CloakBinaryPath string
	Supported       SupportedCLIs
	SupportedNames  []string
	SecretChecker   SecretChecker
}

type SupportedCLIs interface {
	IsSupported(managedCLI string) bool
}

type SupportedCLIFunc func(managedCLI string) bool

func (fn SupportedCLIFunc) IsSupported(managedCLI string) bool {
	return fn(managedCLI)
}

type SecretChecker interface {
	Available() error
	Exists(ref secrets.SecretRef) (bool, error)
}

type KeyringSecretChecker struct {
	Store secrets.Store
}

func (checker KeyringSecretChecker) Available() error {
	store := checker.Store
	if store == nil {
		store = secrets.KeyringStore{}
	}

	ref := secrets.SecretRef{
		Store:   "os",
		Service: "cloak-doctor",
		User:    fmt.Sprintf("availability/%d/%d", os.Getpid(), time.Now().UnixNano()),
	}
	if err := store.Set(ref, "ok"); err != nil {
		return err
	}
	value, err := store.Get(ref)
	if err != nil {
		_ = store.Delete(ref)
		return err
	}
	if err := store.Delete(ref); err != nil {
		return fmt.Errorf("keyring cleanup failed: %w", err)
	}
	if value != "ok" {
		return fmt.Errorf("keyring round trip returned unexpected value")
	}
	return nil
}

func (checker KeyringSecretChecker) Exists(ref secrets.SecretRef) (bool, error) {
	store := checker.Store
	if store == nil {
		store = secrets.KeyringStore{}
	}
	_, err := store.Get(ref)
	if err != nil {
		return false, err
	}
	return true, nil
}

func Run(options Options) []Finding {
	var findings []Finding

	if len(options.SupportedNames) > 0 {
		findings = append(findings, Finding{Severity: SeverityOK, Message: "supported Managed CLIs: " + strings.Join(options.SupportedNames, ", ")})
	}

	findings = append(findings, checkExistingWritableDir(options.Paths.Dir, "data directory"))
	findings = append(findings, checkExistingWritableDir(options.Paths.ShimDir, "shim directory"))

	config, configFinding := readConfig(options.Paths.ConfigFile)
	findings = append(findings, configFinding)

	state, stateFinding := readState(options.Paths.StateFile)
	findings = append(findings, stateFinding)

	if options.SecretChecker != nil {
		findings = append(findings, Finding{Severity: SeverityWarning, Message: "secret store checks may access the OS keyring"})
	}

	if configFinding.Severity != SeverityError && stateFinding.Severity != SeverityError {
		findings = append(findings, CheckStateConsistency(config, state, options.Supported)...)
		findings = append(findings, checkSecretRefs(config, options.SecretChecker)...)
	}

	findings = append(findings, checkSecretStore(options.SecretChecker))
	findings = append(findings, checkInstalledShims(options)...)

	return findings
}

func CheckStateConsistency(config contextstore.Config, state contextstore.State, supported SupportedCLIs) []Finding {
	var findings []Finding

	if config.Version != contextstore.Version {
		findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("unsupported config version %d", config.Version)})
	}
	if state.Version != contextstore.Version {
		findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("unsupported state version %d", state.Version)})
	}

	for managedCLI := range config.ManagedCLIs {
		if supported != nil && !supported.IsSupported(managedCLI) {
			findings = append(findings, Finding{Severity: SeverityWarning, Message: fmt.Sprintf("unknown Managed CLI in config: %s", managedCLI)})
		}
	}

	for managedCLI, contextName := range state.ActiveContexts {
		if supported != nil && !supported.IsSupported(managedCLI) {
			findings = append(findings, Finding{Severity: SeverityWarning, Message: fmt.Sprintf("unknown Managed CLI in state: %s", managedCLI)})
		}

		managedConfig, ok := config.ManagedCLIs[managedCLI]
		if !ok {
			findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("active context references missing Managed CLI: %s", managedCLI)})
			continue
		}
		if _, ok := managedConfig.Contexts[contextName]; !ok {
			findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("active context %s/%s does not exist", managedCLI, contextName)})
		}
	}

	if len(findings) == 0 {
		findings = append(findings, Finding{Severity: SeverityOK, Message: "state is consistent"})
	}
	return findings
}

func readConfig(path string) (contextstore.Config, Finding) {
	config, err := contextstore.ReadConfig(path)
	if err != nil {
		return contextstore.Config{}, Finding{Severity: SeverityError, Message: fmt.Sprintf("config file is invalid: %v", err)}
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return config, Finding{Severity: SeverityWarning, Message: "config file not found; using empty config"}
	}
	return config, Finding{Severity: SeverityOK, Message: "config file parsed"}
}

func readState(path string) (contextstore.State, Finding) {
	state, err := contextstore.ReadState(path)
	if err != nil {
		return contextstore.State{}, Finding{Severity: SeverityError, Message: fmt.Sprintf("state file is invalid: %v", err)}
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return state, Finding{Severity: SeverityWarning, Message: "state file not found; using empty state"}
	}
	return state, Finding{Severity: SeverityOK, Message: "state file parsed"}
}

func checkExistingWritableDir(path, label string) Finding {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Finding{Severity: SeverityWarning, Message: fmt.Sprintf("%s does not exist: %s", label, path)}
	}
	if err != nil {
		return Finding{Severity: SeverityError, Message: fmt.Sprintf("%s cannot be inspected: %v", label, err)}
	}
	if !info.IsDir() {
		return Finding{Severity: SeverityError, Message: fmt.Sprintf("%s is not a directory: %s", label, path)}
	}

	tmp, err := os.CreateTemp(path, ".cloak-doctor-*")
	if err != nil {
		return Finding{Severity: SeverityError, Message: fmt.Sprintf("%s is not writable: %v", label, err)}
	}
	_ = tmp.Close()
	_ = os.Remove(tmp.Name())
	return Finding{Severity: SeverityOK, Message: fmt.Sprintf("%s is writable: %s", label, path)}
}

func checkSecretStore(checker SecretChecker) Finding {
	if checker == nil {
		return Finding{Severity: SeverityWarning, Message: "secret store availability not checked"}
	}
	if err := checker.Available(); err != nil {
		return Finding{Severity: SeverityError, Message: fmt.Sprintf("secret store unavailable: %v", err)}
	}
	return Finding{Severity: SeverityOK, Message: "secret store available"}
}

func checkSecretRefs(config contextstore.Config, checker SecretChecker) []Finding {
	var findings []Finding
	if checker == nil {
		return []Finding{{Severity: SeverityWarning, Message: "Secret Material references not checked"}}
	}

	for managedCLI, managedConfig := range config.ManagedCLIs {
		for contextName, context := range managedConfig.Contexts {
			for field, ref := range context.Secrets {
				if ref.Service == "" || ref.User == "" {
					findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("secret reference %s/%s/%s is incomplete", managedCLI, contextName, field)})
					continue
				}

				exists, err := checker.Exists(ref)
				if errors.Is(err, keyring.ErrNotFound) {
					findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("missing Secret Material for %s/%s/%s", managedCLI, contextName, field)})
					continue
				}
				if err != nil {
					findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("Secret Material for %s/%s/%s is unreadable: %v", managedCLI, contextName, field, err)})
					continue
				}
				if !exists {
					findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("missing Secret Material for %s/%s/%s", managedCLI, contextName, field)})
				}
			}
		}
	}

	if len(findings) == 0 {
		findings = append(findings, Finding{Severity: SeverityOK, Message: "Secret Material references are present"})
	}
	return findings
}

func checkInstalledShims(options Options) []Finding {
	entries, err := os.ReadDir(options.Paths.ShimDir)
	if errors.Is(err, os.ErrNotExist) {
		return []Finding{{Severity: SeverityWarning, Message: "shim directory not found; no installed Shims checked"}}
	}
	if err != nil {
		return []Finding{{Severity: SeverityError, Message: fmt.Sprintf("shim directory cannot be read: %v", err)}}
	}
	if len(entries) == 0 {
		return []Finding{{Severity: SeverityOK, Message: "no installed Shims"}}
	}

	var findings []Finding
	canonicalCloak, cloakErr := filepath.EvalSymlinks(options.CloakBinaryPath)
	if cloakErr != nil {
		findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("Cloak binary cannot be canonicalized: %v", cloakErr)})
	}

	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(options.Paths.ShimDir, name)
		if options.Supported != nil && !options.Supported.IsSupported(name) {
			findings = append(findings, Finding{Severity: SeverityWarning, Message: fmt.Sprintf("unknown Shim in shim directory: %s", name)})
		}
		if entry.Type()&os.ModeSymlink == 0 {
			findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("Shim is not a symlink: %s", path)})
			continue
		}
		if cloakErr == nil {
			canonicalShim, err := filepath.EvalSymlinks(path)
			if err != nil {
				findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("Shim cannot be canonicalized: %s: %v", path, err)})
			} else if canonicalShim != canonicalCloak {
				findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("Shim does not point to Cloak binary: %s", path)})
			} else {
				findings = append(findings, Finding{Severity: SeverityOK, Message: fmt.Sprintf("Shim points to Cloak binary: %s", path)})
			}
		}

		realPath, err := (shim.RealCommandResolver{PathEnv: options.PathEnv, CloakBinaryPath: options.CloakBinaryPath}).Resolve(name)
		if err != nil {
			findings = append(findings, Finding{Severity: SeverityError, Message: fmt.Sprintf("Real Command for %s not found: %v", name, err)})
			continue
		}
		findings = append(findings, Finding{Severity: SeverityOK, Message: fmt.Sprintf("Real Command for %s found: %s", name, realPath)})
		findings = append(findings, checkPathOrdering(options.Paths.ShimDir, realPath, options.PathEnv, name))
	}

	return findings
}

// ShimDirOnPathAhead reports whether shimDir appears in pathEnv ahead of the
// directory containing realPath, i.e. whether an installed shim would take
// precedence over the Real Command. It returns false when either directory is
// absent from pathEnv.
func ShimDirOnPathAhead(shimDir, realPath, pathEnv string) bool {
	shimIndex := pathIndex(pathEnv, shimDir)
	if shimIndex == -1 {
		return false
	}
	realIndex := pathIndex(pathEnv, filepath.Dir(realPath))
	if realIndex == -1 {
		return false
	}
	return shimIndex <= realIndex
}

func checkPathOrdering(shimDir, realPath, pathEnv, name string) Finding {
	shimIndex := pathIndex(pathEnv, shimDir)
	if shimIndex == -1 {
		return Finding{Severity: SeverityWarning, Message: fmt.Sprintf("shim directory is not in PATH for %s", name)}
	}

	realIndex := pathIndex(pathEnv, filepath.Dir(realPath))
	if realIndex == -1 {
		return Finding{Severity: SeverityWarning, Message: fmt.Sprintf("Real Command directory is not in PATH for %s", name)}
	}
	if shimIndex > realIndex {
		return Finding{Severity: SeverityWarning, Message: fmt.Sprintf("shim directory appears after Real Command in PATH for %s", name)}
	}
	return Finding{Severity: SeverityOK, Message: fmt.Sprintf("shim directory appears before Real Command in PATH for %s", name)}
}

func pathIndex(pathEnv, dir string) int {
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	targets := comparablePaths(dir)
	for index, pathDir := range filepath.SplitList(pathEnv) {
		for candidate := range comparablePaths(pathDir) {
			if targets[candidate] {
				return index
			}
		}
	}
	return -1
}

func comparablePaths(path string) map[string]bool {
	paths := map[string]bool{}
	if path == "" {
		return paths
	}
	cleaned := filepath.Clean(path)
	paths[cleaned] = true
	if canonical, err := filepath.EvalSymlinks(cleaned); err == nil {
		paths[canonical] = true
	}
	return paths
}
