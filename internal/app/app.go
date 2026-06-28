package app

import (
	"fmt"
	"io"
	"runtime"

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

	root.AddCommand(newVersionCommand(version))
	root.AddCommand(newShimCommand(env))
	root.AddCommand(newContextCommand(env))
	root.AddCommand(newDoctorCommand(env))
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
