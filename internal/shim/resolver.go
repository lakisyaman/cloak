package shim

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RealCommandResolver struct {
	PathEnv         string
	CloakBinaryPath string
}

func (r RealCommandResolver) Resolve(commandName string) (string, error) {
	if commandName == "" || strings.ContainsRune(commandName, filepath.Separator) {
		return "", fmt.Errorf("invalid command name %q", commandName)
	}

	pathEnv := r.PathEnv
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}

	cloakPath := r.CloakBinaryPath
	if cloakPath == "" {
		var err error
		cloakPath, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve cloak binary: %w", err)
		}
	}

	canonicalCloak, err := filepath.EvalSymlinks(cloakPath)
	if err != nil {
		return "", fmt.Errorf("canonicalize cloak binary %q: %w", cloakPath, err)
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, commandName)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}

		canonicalCandidate, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		if canonicalCandidate == canonicalCloak {
			continue
		}
		return candidate, nil
	}

	return "", fmt.Errorf("real command %q not found in PATH", commandName)
}
