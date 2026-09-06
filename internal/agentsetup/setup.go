// Package agentsetup installs Cloak guidance in existing agent instructions.
package agentsetup

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed instructions.md
var instructions string

const startMarker = "<!-- cloak:start -->"
const endMarker = "<!-- cloak:end -->"

type Options struct {
	Directory  string
	HomeDir    string
	CodexHome  string
	ClaudeHome string
	Global     bool
	Agents     []string
}

type agent struct {
	name  string
	paths []string
}

// Targets discovers only known files. Explicit agents create their default file
// when none of that agent's supported files exists.
func Targets(options Options) ([]string, error) {
	agents := []agent{
		{"codex", []string{"AGENTS.md"}},
		{"claude", []string{"CLAUDE.md", ".claude/CLAUDE.md"}},
		{"gemini", []string{"GEMINI.md"}},
		{"copilot", []string{".github/copilot-instructions.md"}},
	}
	base := options.Directory
	if options.Global {
		base = options.HomeDir
		codexHome := strings.TrimSpace(options.CodexHome)
		if codexHome == "" {
			codexHome = filepath.Join(base, ".codex")
		}
		claudeHome := strings.TrimSpace(options.ClaudeHome)
		if claudeHome == "" {
			claudeHome = filepath.Join(base, ".claude")
		}
		agents[0].paths = []string{filepath.Join(codexHome, "AGENTS.md")}
		agents[1].paths = []string{filepath.Join(claudeHome, "CLAUDE.md")}
		agents[2].paths = []string{filepath.Join(base, ".gemini", "GEMINI.md")}
		agents[3].paths = []string{filepath.Join(base, ".copilot", "copilot-instructions.md")}
	}
	if base == "" {
		return nil, fmt.Errorf("instruction directory is unavailable")
	}
	selected := map[string]bool{}
	for _, name := range options.Agents {
		if name != "codex" && name != "claude" && name != "gemini" && name != "copilot" {
			return nil, fmt.Errorf("unknown agent %q; choose codex, claude, gemini, or copilot", name)
		}
		selected[name] = true
	}
	var paths []string
	for _, agent := range agents {
		if len(selected) > 0 && !selected[agent.name] {
			continue
		}
		found := false
		for i, path := range agent.paths {
			if !options.Global {
				path = filepath.Join(base, path)
			}
			path, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			agent.paths[i] = path
			_, err = os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", path, err)
			}
			paths = append(paths, path)
			found = true
		}
		if !found && selected[agent.name] {
			paths = append(paths, agent.paths[0])
		}
	}
	return paths, nil
}

type Change struct {
	Path     string
	original []byte
	content  []byte
	mode     os.FileMode
	exists   bool
}

// Prepare validates all files before any write. Canonical paths keep symlinks
// intact and ensure two instruction names sharing a target are written once.
func Prepare(paths []string, options Options) ([]Change, error) {
	var changes []Change
	seen := map[string]bool{}
	for _, path := range paths {
		canonical, err := resolvePath(path)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", path, err)
		}
		if !options.Global {
			base, err := filepath.EvalSymlinks(options.Directory)
			if err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(base, canonical)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("instruction file %s resolves outside the current directory", path)
			}
		}
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		change := Change{Path: canonical, mode: 0o644}
		if options.Global {
			change.mode = 0o600
		}
		info, err := os.Stat(canonical)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err == nil {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("instruction file %s is not a regular file", path)
			}
			change.exists = true
			change.mode = info.Mode().Perm()
			change.original, err = os.ReadFile(canonical)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
		}
		content, err := replaceSection(string(change.original))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		change.content = []byte(content)
		changes = append(changes, change)
	}
	return changes, nil
}

// Resolve existing symlinks, including in parents of files not yet created.
// A dangling symlink fails rather than being replaced or silently followed.
func resolvePath(path string) (string, error) {
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", fmt.Errorf("cannot resolve %s", path)
	}
	resolved, err := resolvePath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

func replaceSection(content string) (string, error) {
	startCount, endCount := strings.Count(content, startMarker), strings.Count(content, endMarker)
	start, end := strings.Index(content, startMarker), strings.Index(content, endMarker)
	if startCount != endCount || startCount > 1 || (startCount == 1 && end < start) {
		return "", fmt.Errorf("incomplete or duplicate Cloak markers; repair the cloak:start and cloak:end section before rerunning cloak init")
	}
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	block := strings.ReplaceAll(strings.TrimSuffix(instructions, "\n"), "\n", newline)
	if startCount == 1 {
		return content[:start] + block + content[end+len(endMarker):], nil
	}
	separator := ""
	if content != "" && !strings.HasSuffix(content, newline+newline) {
		separator = newline
		if !strings.HasSuffix(content, newline) {
			separator += newline
		}
	}
	return content + separator + block + newline, nil
}

// Write atomically replaces one file, preserving its permissions and skipping
// identical content. A set of files is not a single filesystem transaction.
func (change Change) Write(global bool) (string, error) {
	if bytes.Equal(change.original, change.content) {
		return "unchanged", nil
	}
	dirMode := os.FileMode(0o755)
	if global {
		dirMode = 0o700
	}
	if err := os.MkdirAll(filepath.Dir(change.Path), dirMode); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(change.Path), ".cloak-init-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(change.mode); err != nil {
		return "", err
	}
	if _, err := file.Write(change.content); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(file.Name(), change.Path); err != nil {
		return "", err
	}
	if change.exists {
		return "updated", nil
	}
	return "created", nil
}
