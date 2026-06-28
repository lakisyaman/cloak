package app

import (
	"fmt"
	"io"

	"cloak/internal/doctor"

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
	root.Version = version

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
