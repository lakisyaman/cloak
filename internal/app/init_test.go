package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeInstruction(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func readInstruction(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInitDiscoversCurrentDirectoryAndPreservesInstructions(t *testing.T) {
	env := newConnectorEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	files := []string{"AGENTS.md", "CLAUDE.md", ".claude/CLAUDE.md", "GEMINI.md", ".github/copilot-instructions.md"}
	for _, file := range files {
		writeInstruction(t, file, "# Existing instructions\n\nKeep this text.\n")
	}
	writeInstruction(t, "nested/AGENTS.md", "nested")
	writeInstruction(t, "README.md", "readme")
	out := mustRun(t, env, "init")
	if strings.Count(out, "updated ") != len(files) {
		t.Fatal(out)
	}
	for _, file := range files {
		content := readInstruction(t, file)
		if !strings.HasPrefix(content, "# Existing instructions\n\nKeep this text.\n") || strings.Count(content, "<!-- cloak:start -->") != 1 {
			t.Fatal(content)
		}
		for _, guidance := range []string{"cloak connector list", "cloak connector add @cloak/<cli>", "cloak <cli> context switch <name>", "cloak <cli> context configure <name> --help", "user-global"} {
			if !strings.Contains(content, guidance) {
				t.Fatalf("missing %q", guidance)
			}
		}
		// An unchanged run must preserve the file itself, including mtime.
		stamp := time.Unix(1234567890, 0)
		if err := os.Chtimes(file, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	out = mustRun(t, env, "init")
	if strings.Count(out, "unchanged ") != len(files) {
		t.Fatal(out)
	}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil || info.ModTime().Unix() != 1234567890 || info.Mode().Perm() != 0o640 {
			t.Fatalf("file attributes changed: %s %v %v", file, info, err)
		}
	}
	if readInstruction(t, "nested/AGENTS.md") != "nested" || readInstruction(t, "README.md") != "readme" {
		t.Fatal("modified an unrelated file")
	}
	for _, path := range []string{env.Paths.ConfigFile, env.Paths.StateFile, env.Connectors.Dir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("init touched Cloak state: %s", path)
		}
	}
}

func TestInitRefreshesOnlyMarkedSectionWithCRLF(t *testing.T) {
	env := newConnectorEnv(t)
	t.Chdir(t.TempDir())
	prefix, suffix := "# Personal\r\n\r\n", "\r\n\r\n## More\r\nKeep trailing spaces.  "
	writeInstruction(t, "AGENTS.md", prefix+"<!-- cloak:start -->\r\nold guidance\r\n<!-- cloak:end -->"+suffix)
	mustRun(t, env, "init")
	content := readInstruction(t, "AGENTS.md")
	if !strings.HasPrefix(content, prefix) || !strings.HasSuffix(content, suffix) || strings.Contains(content, "old guidance") {
		t.Fatal(content)
	}
	if strings.Contains(strings.ReplaceAll(content, "\r\n", ""), "\n") {
		t.Fatal("introduced LF into CRLF file")
	}
}

func TestInitRefusesMalformedFilesBeforeWritingAny(t *testing.T) {
	for _, bad := range []string{
		"<!-- cloak:start -->",
		"<!-- cloak:end -->",
		"<!-- cloak:end --><!-- cloak:start -->",
		strings.Repeat("<!-- cloak:start --><!-- cloak:end -->", 2),
	} {
		t.Run(bad, func(t *testing.T) {
			env := newConnectorEnv(t)
			t.Chdir(t.TempDir())
			writeInstruction(t, "AGENTS.md", "keep")
			writeInstruction(t, "CLAUDE.md", bad)
			if _, err := runCommand(t, env, "init"); err == nil || !strings.Contains(err.Error(), "markers") {
				t.Fatal(err)
			}
			if readInstruction(t, "AGENTS.md") != "keep" || readInstruction(t, "CLAUDE.md") != bad {
				t.Fatal("modified files despite failed validation")
			}
		})
	}
}

func TestInitExplicitAgentsAndNonInteractiveFallback(t *testing.T) {
	env := newConnectorEnv(t)
	t.Chdir(t.TempDir())
	if _, err := runCommand(t, env, "init"); err == nil || !strings.Contains(err.Error(), "--agent") {
		t.Fatal(err)
	}
	if _, err := runCommand(t, env, "init", "--agent", "codex,unknown"); err == nil {
		t.Fatal("accepted unknown agent")
	}
	if _, err := os.Stat("AGENTS.md"); !os.IsNotExist(err) {
		t.Fatal("created a file before validating agent selection")
	}
	writeInstruction(t, "GEMINI.md", "untouched")
	out := mustRun(t, env, "init", "--agent", "claude,codex", "--agent", "codex,copilot")
	if strings.Count(out, "created ") != 3 || readInstruction(t, "GEMINI.md") != "untouched" {
		t.Fatal(out)
	}
	if _, err := runCommand(t, env, "init", "unexpected"); err == nil {
		t.Fatal("accepted positional argument")
	}
}

func TestInitExplicitClaudeUsesExistingNestedFile(t *testing.T) {
	env := newConnectorEnv(t)
	t.Chdir(t.TempDir())
	writeInstruction(t, ".claude/CLAUDE.md", "keep")
	mustRun(t, env, "init", "--agent", "claude")
	if _, err := os.Stat("CLAUDE.md"); !os.IsNotExist(err) {
		t.Fatal("created a duplicate Claude instruction file")
	}
	if !strings.Contains(readInstruction(t, ".claude/CLAUDE.md"), "cloak connector list") {
		t.Fatal("nested instructions not updated")
	}
}

func TestInitInteractiveChoiceAndCancellation(t *testing.T) {
	for _, input := range []string{"2\n", "invalid\nclaude\n", ""} {
		t.Run(input, func(t *testing.T) {
			env := newConnectorEnv(t)
			env.Interactive = func() bool { return true }
			t.Chdir(t.TempDir())
			var out bytes.Buffer
			cmd := NewRootCommandWithEnv("test", env)
			cmd.SetArgs([]string{"init"})
			cmd.SetIn(strings.NewReader(input))
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			err := cmd.Execute()
			if input == "" {
				if err == nil || !strings.Contains(err.Error(), "interrupted") {
					t.Fatal(err)
				}
				entries, _ := os.ReadDir(".")
				if len(entries) != 0 {
					t.Fatal("cancelled selection wrote files")
				}
			} else if err != nil || !strings.Contains(readInstruction(t, "CLAUDE.md"), "<!-- cloak:start -->") {
				t.Fatal(err, out.String())
			}
		})
	}
}

func TestInitGlobalOverridesAndCorruptContextIndependence(t *testing.T) {
	env := newConnectorEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeInstruction(t, "AGENTS.md", "local")
	writeInstruction(t, env.Paths.ConfigFile, "invalid json")
	codex, claude := filepath.Join(t.TempDir(), "codex"), filepath.Join(t.TempDir(), "claude")
	t.Setenv("CODEX_HOME", codex)
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	out := mustRun(t, env, "init", "--global", "--agent", "codex,claude")
	if strings.Count(out, "created ") != 2 || readInstruction(t, "AGENTS.md") != "local" {
		t.Fatal(out)
	}
	for _, path := range []string{filepath.Join(codex, "AGENTS.md"), filepath.Join(claude, "CLAUDE.md")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal(path, info, err)
		}
	}
	if readInstruction(t, env.Paths.ConfigFile) != "invalid json" {
		t.Fatal("init modified Context data")
	}
}

func TestInitSymlinksAndInvalidFileTypes(t *testing.T) {
	t.Run("shared target", func(t *testing.T) {
		env := newConnectorEnv(t)
		t.Chdir(t.TempDir())
		writeInstruction(t, "AGENTS.md", "keep")
		if err := os.Symlink("AGENTS.md", "CLAUDE.md"); err != nil {
			t.Fatal(err)
		}
		out := mustRun(t, env, "init")
		info, err := os.Lstat("CLAUDE.md")
		if err != nil || info.Mode()&os.ModeSymlink == 0 || strings.Count(out, "updated ") != 1 {
			t.Fatal(out, err)
		}
	})
	for _, kind := range []string{"outside", "dangling", "directory"} {
		t.Run(kind, func(t *testing.T) {
			env := newConnectorEnv(t)
			outside := filepath.Join(t.TempDir(), "outside.md")
			writeInstruction(t, outside, "outside")
			t.Chdir(t.TempDir())
			writeInstruction(t, "AGENTS.md", "keep")
			var err error
			switch kind {
			case "outside":
				err = os.Symlink(outside, "CLAUDE.md")
			case "dangling":
				err = os.Symlink("missing.md", "CLAUDE.md")
			case "directory":
				err = os.Mkdir("CLAUDE.md", 0o755)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runCommand(t, env, "init"); err == nil {
				t.Fatal("accepted invalid target")
			}
			if readInstruction(t, outside) != "outside" || readInstruction(t, "AGENTS.md") != "keep" {
				t.Fatal("modified files despite invalid target")
			}
		})
	}
}
