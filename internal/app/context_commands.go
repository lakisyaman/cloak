package app

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"

	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
	"github.com/spf13/cobra"
)

var contextNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func newCLIContextCommand(env CommandEnv, managedCLI string) *cobra.Command {
	cmd := &cobra.Command{Use: "context", Short: "Manage Contexts for " + managedCLI}
	cmd.AddCommand(newContextConfigureCommand(env, managedCLI))
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List Contexts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return listContexts(cmd.OutOrStdout(), env, managedCLI, false)
	}})
	cmd.AddCommand(&cobra.Command{Use: "current", Short: "Show the Active Context", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return currentContext(cmd.OutOrStdout(), env, managedCLI)
	}})
	cmd.AddCommand(&cobra.Command{Use: "switch <name>", Short: "Switch the Active Context", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return switchContext(cmd.OutOrStdout(), env, managedCLI, args[0])
	}})
	cmd.AddCommand(&cobra.Command{Use: "remove <name>", Short: "Remove a Context and its secrets", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return removeContext(cmd.OutOrStdout(), cmd.ErrOrStderr(), env, managedCLI, args[0])
	}})
	cmd.AddCommand(&cobra.Command{Use: "show <name>", Short: "Show a Context with secrets redacted", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return showContext(cmd.OutOrStdout(), env, managedCLI, args[0])
	}})
	return cmd
}

func listContexts(stdout io.Writer, env CommandEnv, managedCLI string, indented bool) error {
	if err := validateManagedCLI(managedCLI); err != nil {
		return err
	}
	config, err := env.Store.ReadConfig()
	if err != nil {
		return err
	}
	state, err := env.Store.ReadState()
	if err != nil {
		return err
	}
	managedConfig := config.ManagedCLIs[managedCLI]
	if len(managedConfig.Contexts) == 0 {
		if indented {
			fmt.Fprintln(stdout, "  no contexts")
		} else {
			fmt.Fprintln(stdout, "no contexts")
		}
		return nil
	}
	names := sortedContextNames(managedConfig)
	prefix := ""
	if indented {
		prefix = "  "
	}
	active := effectiveContext(state, managedCLI)
	for _, name := range names {
		marker := " "
		if active == name {
			marker = "*"
		}
		fmt.Fprintf(stdout, "%s%s %s\n", prefix, marker, name)
	}
	return nil
}

func currentContext(stdout io.Writer, env CommandEnv, managedCLI string) error {
	if err := validateManagedCLI(managedCLI); err != nil {
		return err
	}
	if name, variable := sessionContext(managedCLI, os.Environ()); name != "" {
		fmt.Fprintf(stdout, "%s (from %s)\n", name, variable)
		return nil
	}
	state, err := env.Store.ReadState()
	if err != nil {
		return err
	}
	if active := state.ActiveContexts[managedCLI]; active != "" {
		fmt.Fprintln(stdout, active)
		return nil
	}
	fmt.Fprintf(stdout, "no active context for %s\n", managedCLI)
	return nil
}

func switchContext(stdout io.Writer, env CommandEnv, managedCLI, name string) error {
	if err := validateManagedCLI(managedCLI); err != nil {
		return err
	}
	if err := validateContextName(name); err != nil {
		return err
	}
	config, err := env.Store.ReadConfig()
	if err != nil {
		return err
	}
	if _, ok := findContext(config, managedCLI, name); !ok {
		return fmt.Errorf("context %s does not exist for %s", name, managedCLI)
	}
	state, err := env.Store.ReadState()
	if err != nil {
		return err
	}
	if state.ActiveContexts == nil {
		state.ActiveContexts = map[string]string{}
	}
	state.ActiveContexts[managedCLI] = name
	if err := env.Store.WriteState(state); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "switched %s to context %s\n", managedCLI, name)
	return nil
}

func removeContext(stdout, stderr io.Writer, env CommandEnv, managedCLI, name string) error {
	if err := validateManagedCLI(managedCLI); err != nil {
		return err
	}
	if err := validateContextName(name); err != nil {
		return err
	}
	config, err := env.Store.ReadConfig()
	if err != nil {
		return err
	}
	managedConfig, ok := config.ManagedCLIs[managedCLI]
	if !ok {
		return fmt.Errorf("context %s does not exist for %s", name, managedCLI)
	}
	oldContext, ok := managedConfig.Contexts[name]
	if !ok {
		return fmt.Errorf("context %s does not exist for %s", name, managedCLI)
	}
	delete(managedConfig.Contexts, name)
	config.ManagedCLIs[managedCLI] = managedConfig
	if err := env.Store.WriteConfig(config); err != nil {
		return err
	}

	state, err := env.Store.ReadState()
	if err != nil {
		return err
	}
	if state.ActiveContexts[managedCLI] == name {
		delete(state.ActiveContexts, managedCLI)
		if err := env.Store.WriteState(state); err != nil {
			return err
		}
	}
	deleteOldSecrets(stderr, env.Secrets, oldContext, contextstore.Context{})
	fmt.Fprintf(stdout, "removed context %s for %s\n", name, managedCLI)
	return nil
}

func showContext(stdout io.Writer, env CommandEnv, managedCLI, name string) error {
	if err := validateManagedCLI(managedCLI); err != nil {
		return err
	}
	if err := validateContextName(name); err != nil {
		return err
	}
	config, err := env.Store.ReadConfig()
	if err != nil {
		return err
	}
	ctx, ok := findContext(config, managedCLI, name)
	if !ok {
		return fmt.Errorf("context %s does not exist for %s", name, managedCLI)
	}
	state, err := env.Store.ReadState()
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "name: %s\n", name)
	fmt.Fprintf(stdout, "managed cli: %s\n", managedCLI)
	def, _ := env.Connectors.Get(managedCLI)
	for _, key := range sortedMetadataKeys(ctx.Metadata) {
		var value any = "<retained; Connector field unavailable>"
		if def != nil {
			if field, ok := def.Fields[key]; ok {
				if field.Secret {
					value = "<needs secret storage>"
				} else if _, err := field.Value(ctx.Metadata[key]); err != nil {
					value = "<needs configuration>"
				} else {
					value = ctx.Metadata[key]
				}
			}
		}
		fmt.Fprintf(stdout, "%s: %v\n", displayMetadataKey(key), value)
	}
	for _, key := range sortedSecretKeys(ctx.Secrets) {
		fmt.Fprintf(stdout, "%s: <stored>\n", key)
	}
	active := "no"
	if effectiveContext(state, managedCLI) == name {
		active = "yes"
	}
	fmt.Fprintf(stdout, "active: %s\n", active)
	return nil
}

func effectiveContext(state contextstore.State, managedCLI string) string {
	if name, _ := sessionContext(managedCLI, os.Environ()); name != "" {
		return name
	}
	return state.ActiveContexts[managedCLI]
}

func deleteOldSecrets(stderr io.Writer, store secrets.Store, oldContext, newContext contextstore.Context) {
	newRefs := map[secrets.SecretRef]bool{}
	for _, ref := range newContext.Secrets {
		newRefs[ref] = true
	}
	for field, ref := range oldContext.Secrets {
		if newRefs[ref] {
			continue
		}
		if err := store.Delete(ref); err != nil {
			fmt.Fprintf(stderr, "cloak: warning: failed to delete old secret %s\n", field)
		}
	}
}

func validateManagedCLI(managedCLI string) error {
	return connectors.ValidateCommand(managedCLI)
}

func validateContextName(name string) error {
	if !contextNamePattern.MatchString(name) {
		return fmt.Errorf("invalid context name %q", name)
	}
	return nil
}

func sortedContextNames(config contextstore.ManagedCLIConfig) []string {
	names := make([]string, 0, len(config.Contexts))
	for name := range config.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedMetadataKeys(metadata map[string]any) []string {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSecretKeys(secretRefs map[string]secrets.SecretRef) []string {
	keys := make([]string, 0, len(secretRefs))
	for key := range secretRefs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func displayMetadataKey(key string) string { return key }
