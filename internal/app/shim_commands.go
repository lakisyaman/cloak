package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/doctor"

	"github.com/spf13/cobra"
)

func newShimCommand(env CommandEnv) *cobra.Command {
	cmd := &cobra.Command{Use: "shim", Short: "Manage Cloak shims"}
	cmd.AddCommand(newShimInstallCommand(env))
	cmd.AddCommand(newShimUninstallCommand(env))
	cmd.AddCommand(newShimListCommand(env))
	cmd.AddCommand(newShimDirCommand(env))
	return cmd
}

func newShimInstallCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "install <cli>",
		Short: "Install a Shim for a Managed CLI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			managedCLI := args[0]
			if _, err := env.Connectors.Get(managedCLI); err != nil {
				return err
			}
			realPath, err := env.Resolver.Resolve(managedCLI)
			if err != nil {
				return fmt.Errorf("real command %s not found; install it first: %w", managedCLI, err)
			}
			if err := os.MkdirAll(env.Paths.ShimDir, 0o700); err != nil {
				return err
			}
			if _, err := ownedShimTarget(env, managedCLI); err != nil {
				return err
			}
			shimPath := filepath.Join(env.Paths.ShimDir, managedCLI)
			if err := replaceSymlink(env.CloakBinaryPath, shimPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed shim %s -> %s\n", shimPath, env.CloakBinaryPath)
			if !doctor.ShimDirOnPathAhead(env.Paths.ShimDir, realPath, env.PathEnv) {
				printShimPathHint(cmd, env.Paths.ShimDir, managedCLI)
			}
			return nil
		},
	}
}

func newShimUninstallCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <cli>",
		Short: "Uninstall a Shim for a Managed CLI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			managedCLI := args[0]
			if err := connectors.ValidateCommand(managedCLI); err != nil {
				return err
			}
			if _, err := ownedShimTarget(env, managedCLI); err != nil {
				return err
			}
			shimPath := filepath.Join(env.Paths.ShimDir, managedCLI)
			if err := os.Remove(shimPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "uninstalled shim %s\n", shimPath)
			return nil
		},
	}
}

func newShimListCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed Shims",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := os.ReadDir(env.Paths.ShimDir)
			if os.IsNotExist(err) {
				fmt.Fprintln(cmd.OutOrStdout(), "no shims")
				return nil
			}
			if err != nil {
				return err
			}
			var names []string
			for _, entry := range entries {
				if entry.Type()&os.ModeSymlink != 0 {
					names = append(names, entry.Name())
				}
			}
			sort.Strings(names)
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no shims")
				return nil
			}
			for _, name := range names {
				fmt.Fprintln(cmd.OutOrStdout(), name)
			}
			return nil
		},
	}
}

func newShimDirCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "dir",
		Short: "Print the Cloak shim directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), env.Paths.ShimDir)
			return nil
		},
	}
}

func printShimPathHint(cmd *cobra.Command, shimDir, managedCLI string) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nThe shim directory is not ahead of the real %s on your PATH, so the shim\n", managedCLI)
	fmt.Fprint(out, "will not take effect yet. Add it ahead of your system paths:\n\n")
	fmt.Fprintf(out, "    export PATH=\"%s:$PATH\"\n\n", shimDir)
	fmt.Fprint(out, "Add that line to your shell profile (e.g. ~/.zshrc) to make it permanent.\n")
}

func replaceSymlink(target, link string) error {
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to replace non-symlink %s", link)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, link)
}
