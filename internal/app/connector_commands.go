package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/doctor"
	"github.com/spf13/cobra"
)

func newConnectorCommand(env CommandEnv) *cobra.Command {
	cmd := &cobra.Command{Use: "connector", Short: "Install and manage declarative Connectors"}
	cmd.AddCommand(&cobra.Command{Use: "add <@cloak/cli | path.yaml>", Short: "Copy a Connector into Cloak and install its Shim", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		record, def, err := env.Connectors.Acquire(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		names, err := env.Connectors.Names()
		if err != nil {
			return err
		}
		for _, name := range names {
			if name == def.Command {
				return fmt.Errorf("Connector %s is already installed; run cloak connector update %s", name, name)
			}
		}
		link := filepath.Join(env.Paths.ShimDir, def.Command)
		oldTarget, err := ownedShimTarget(env, def.Command)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(env.Paths.ShimDir, 0o700); err != nil {
			return err
		}
		if err = replaceSymlink(env.CloakBinaryPath, link); err != nil {
			return err
		}
		if err = env.Connectors.Write(record); err != nil {
			if oldTarget != "" {
				_ = replaceSymlink(oldTarget, link)
			} else {
				_ = os.Remove(link)
			}
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "installed Connector %s from %s\n", def.Command, record.Source)
		printConnectorPathHint(cmd, env, def.Command)
		return nil
	}})
	var source string
	update := &cobra.Command{Use: "update <cli>", Short: "Refresh the installed copy from its saved source", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := connectors.ValidateCommand(name); err != nil {
			return err
		}
		from := source
		if from == "" {
			record, _, err := env.Connectors.Read(name)
			if err != nil {
				return err
			}
			from = record.Source
		}
		record, def, err := env.Connectors.Acquire(cmd.Context(), from)
		if err != nil {
			return err
		}
		if def.Command != name {
			return fmt.Errorf("updated Connector must keep command %s", name)
		}
		// An explicit replacement source can recover a corrupt installed record,
		// but update never implicitly installs a missing Connector or Shim.
		if _, err = os.Stat(filepath.Join(env.Connectors.Dir, name+".json")); err != nil {
			return fmt.Errorf("Connector %s is not installed; use connector add", name)
		}
		if err = env.Connectors.Write(record); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "updated Connector %s from %s\n", name, record.Source)
		return nil
	}}
	update.Flags().StringVar(&source, "source", "", "replace the saved source with @cloak/<cli> or a YAML path")
	cmd.AddCommand(update)
	cmd.AddCommand(&cobra.Command{Use: "remove <cli>", Short: "Remove a Connector and its Shim; retain Contexts and secrets", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := connectors.ValidateCommand(name); err != nil {
			return err
		}
		target, err := ownedShimTarget(env, name)
		if err != nil {
			return err
		}
		link := filepath.Join(env.Paths.ShimDir, name)
		if target != "" {
			if err = os.Remove(link); err != nil {
				return err
			}
		}
		if err = env.Connectors.Remove(name); err != nil {
			if target != "" {
				_ = os.Symlink(target, link)
			}
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed Connector %s and its Shim; Contexts and secrets retained\n", name)
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List installed Connectors and their sources", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		names, err := env.Connectors.Names()
		if err != nil {
			return err
		}
		if len(names) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no connectors")
		}
		for _, name := range names {
			record, _, err := env.Connectors.Read(name)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t<invalid; update --source or remove>\n", name)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", name, record.Source)
			}
		}
		return nil
	}})
	return cmd
}

func printConnectorPathHint(cmd *cobra.Command, env CommandEnv, name string) {
	real, err := env.Resolver.Resolve(name)
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "install the native %s command before using its Shim\n", name)
	}
	if err != nil || !doctor.ShimDirOnPathAhead(env.Paths.ShimDir, real, env.PathEnv) {
		printShimPathHint(cmd, env.Paths.ShimDir, name)
	}
}

// Only modify a symlink owned by Cloak, never an unrelated file or link.
func ownedShimTarget(env CommandEnv, name string) (string, error) {
	if err := connectors.ValidateCommand(name); err != nil {
		return "", err
	}
	link := filepath.Join(env.Paths.ShimDir, name)
	info, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("refusing to replace non-symlink %s", link)
	}
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(env.Paths.ShimDir, abs)
	}
	canonical, canonicalErr := filepath.EvalSymlinks(abs)
	cloak, cloakErr := filepath.EvalSymlinks(env.CloakBinaryPath)
	if filepath.Clean(abs) != filepath.Clean(env.CloakBinaryPath) && (canonicalErr != nil || cloakErr != nil || canonical != cloak) {
		return "", fmt.Errorf("refusing to modify a Shim not owned by this Cloak installation: %s", link)
	}
	return target, nil
}
