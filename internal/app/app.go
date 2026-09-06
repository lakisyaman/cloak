package app

import (
	"fmt"
	"io"
	"runtime"
	"sort"

	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/doctor"

	"github.com/spf13/cobra"
)

func Execute(version string, args []string, stdout, stderr io.Writer) error {
	env, err := DefaultCommandEnv()
	if err != nil {
		return err
	}
	cmd := NewRootCommandWithEnv(version, env)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.Execute()
}

func NewRootCommand(version string) *cobra.Command {
	env, _ := DefaultCommandEnv()
	return NewRootCommandWithEnv(version, env)
}

func NewRootCommandWithEnv(version string, env CommandEnv) *cobra.Command {
	env = normalizeCommandEnv(env)
	root := &cobra.Command{
		Use:           "cloak",
		Short:         "Add Context management to supported CLIs",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	applyVersion(root, version)
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(newVersionCommand(version))
	root.AddCommand(newShimCommand(env))
	root.AddCommand(newConnectorCommand(env))
	root.AddCommand(newDoctorCommand(env))
	// Retained Contexts remain manageable after a Connector is removed. Loading
	// one corrupt definition must not prevent connector update/remove commands.
	names := map[string]bool{}
	installed, loadErr := env.Connectors.Names()
	for _, name := range installed {
		names[name] = true
	}
	config, configErr := env.Store.ReadConfig()
	for name := range config.ManagedCLIs {
		if connectors.ValidateCommand(name) == nil {
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		cli := &cobra.Command{Use: name, Short: "Manage Contexts for " + name}
		cli.AddCommand(newCLIContextCommand(env, name))
		root.AddCommand(cli)
	}
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Lifecycle and diagnostics must work even when Context files are corrupt.
		for ancestor := cmd; ancestor != nil; ancestor = ancestor.Parent() {
			if !names[ancestor.Name()] {
				continue
			}
			if loadErr != nil {
				return fmt.Errorf("read Connectors: %w", loadErr)
			}
			if configErr != nil {
				return fmt.Errorf("read Contexts: %w", configErr)
			}
			break
		}
		return nil
	}
	return root
}

func newDoctorCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report Cloak installation and state problems",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, finding := range doctor.Run(doctorOptionsFromEnv(env)) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", finding.Severity, finding.Message)
			}
			names, err := env.Connectors.Names()
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "error: cannot read installed Connectors: %v\n", err)
			}
			for _, name := range names {
				if _, err := env.Connectors.Get(name); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "error: %v\n", err)
				}
			}
			return nil
		},
	}
}

var (
	buildCommit string
	buildDate   string
)

// SetBuildInfo records optional VCS/build metadata surfaced by the version
// command and `--version`. main wires this from -ldflags values or the build
// info embedded by the Go toolchain.
func SetBuildInfo(commit, date string) {
	buildCommit = commit
	buildDate = date
}

func applyVersion(root *cobra.Command, version string) {
	root.Version = version
	root.SetVersionTemplate(versionString(version))
}

func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(cmd.OutOrStdout(), versionString(version))
			return nil
		},
	}
}

func versionString(version string) string {
	s := "cloak version " + version + "\n"
	if buildCommit != "" {
		s += "commit: " + buildCommit + "\n"
	}
	if buildDate != "" {
		s += "built:  " + buildDate + "\n"
	}
	s += "go:     " + runtime.Version() + "\n"
	return s
}
