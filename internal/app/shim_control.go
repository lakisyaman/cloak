package app

import "github.com/spf13/cobra"

func NewShimControlCommand(version, managedCLI string) *cobra.Command {
	env, _ := DefaultCommandEnv()
	return NewShimControlCommandWithEnv(version, managedCLI, env)
}

func NewShimControlCommandWithEnv(version, managedCLI string, env CommandEnv) *cobra.Command {
	env = normalizeCommandEnv(env)
	root := &cobra.Command{
		Use:           managedCLI,
		Short:         "Shim entrypoint for " + managedCLI,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Version = version

	control := &cobra.Command{Use: "cloak", Short: "Manage Cloak Contexts for " + managedCLI}
	control.AddCommand(newShimScopedContextCommand(env, managedCLI))
	root.AddCommand(control)
	return root
}
