package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lakisyaman/cloak/internal/agentsetup"
	"github.com/spf13/cobra"
)

func newInitCommand(env CommandEnv) *cobra.Command {
	var global bool
	var agents []string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Add Cloak guidance to agent instruction files",
		Long: `Add or refresh Cloak's section in all recognized agent instruction files.
By default, discover files in the current directory. Use --global for user-level
instructions. If no files exist, choose an agent interactively or pass --agent.
Existing instructions outside Cloak's marked section are preserved.`,
		Example: "  cloak init\n  cloak init --global\n  cloak init --agent claude\n  cloak init --global --agent codex,gemini",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options := agentsetup.Options{Global: global, Agents: agents}
			var err error
			if global {
				options.HomeDir, err = os.UserHomeDir()
				options.CodexHome = os.Getenv("CODEX_HOME")
				options.ClaudeHome = os.Getenv("CLAUDE_CONFIG_DIR")
			} else {
				options.Directory, err = os.Getwd()
			}
			if err != nil {
				return err
			}
			paths, err := agentsetup.Targets(options)
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				interactive := isInteractive(cmd)
				if env.Interactive != nil {
					interactive = env.Interactive()
				}
				if !interactive {
					return fmt.Errorf("no agent instruction files found; rerun with --agent codex, claude, gemini, or copilot (see cloak init --help)")
				}
				agent, err := promptInitAgent(cmd)
				if err != nil {
					return err
				}
				options.Agents = []string{agent}
				paths, err = agentsetup.Targets(options)
				if err != nil {
					return err
				}
			}
			changes, err := agentsetup.Prepare(paths, options)
			if err != nil {
				return err
			}
			for _, change := range changes {
				status, err := change.Write(global)
				if err != nil {
					return fmt.Errorf("write %s: %w", change.Path, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", status, change.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "write user-level agent instructions")
	cmd.Flags().StringSliceVar(&agents, "agent", nil, "target codex, claude, gemini, or copilot; create if missing (comma-separated or repeated)")
	return cmd
}

func promptInitAgent(cmd *cobra.Command) (string, error) {
	agents := []string{"codex", "claude", "gemini", "copilot"}
	fmt.Fprintln(cmd.ErrOrStderr(), "No agent instruction files found. Choose an agent to initialize:")
	for i, agent := range agents {
		fmt.Fprintf(cmd.ErrOrStderr(), "  %d. %s\n", i+1, agent)
	}
	for {
		fmt.Fprint(cmd.ErrOrStderr(), "Agent (name or number): ")
		answer, err := readPromptLine(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("agent selection interrupted; no files written")
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		if number, err := strconv.Atoi(answer); err == nil && number >= 1 && number <= len(agents) {
			return agents[number-1], nil
		}
		for _, agent := range agents {
			if answer == agent {
				return agent, nil
			}
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "Choose codex, claude, gemini, or copilot.")
	}
}
