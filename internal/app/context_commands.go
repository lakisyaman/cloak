package app

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"cloak/internal/adapters"
	"cloak/internal/contextstore"
	"cloak/internal/secrets"

	"github.com/spf13/cobra"
)

var contextNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func newContextCommand(env CommandEnv) *cobra.Command {
	cmd := &cobra.Command{Use: "context", Short: "Manage Contexts across Managed CLIs"}
	cmd.AddCommand(newStandaloneContextListCommand(env))
	cmd.AddCommand(newStandaloneContextCurrentCommand(env))
	cmd.AddCommand(newStandaloneContextSwitchCommand(env))
	cmd.AddCommand(newStandaloneContextRemoveCommand(env))
	cmd.AddCommand(newStandaloneContextShowCommand(env))
	return cmd
}

func newShimScopedContextCommand(env CommandEnv, managedCLI string) *cobra.Command {
	cmd := &cobra.Command{Use: "context", Short: "Manage Contexts for this Managed CLI"}
	cmd.AddCommand(newContextAddCommand(env, managedCLI))
	cmd.AddCommand(newContextListCommand(env, managedCLI))
	cmd.AddCommand(newContextCurrentCommand(env, managedCLI))
	cmd.AddCommand(newContextSwitchCommand(env, managedCLI))
	cmd.AddCommand(newContextRemoveCommand(env, managedCLI))
	cmd.AddCommand(newContextShowCommand(env, managedCLI))
	return cmd
}

func newStandaloneContextListCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "list [cli]",
		Short: "List Contexts",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return listContexts(cmd.OutOrStdout(), env, args[0], false)
			}
			config, err := env.Store.ReadConfig()
			if err != nil {
				return err
			}
			if len(config.ManagedCLIs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no contexts")
				return nil
			}
			managedCLIs := sortedManagedCLIs(config)
			for i, managedCLI := range managedCLIs {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", managedCLI)
				if err := listContexts(cmd.OutOrStdout(), env, managedCLI, true); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newStandaloneContextCurrentCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "current <cli>",
		Short: "Show the Active Context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return currentContext(cmd.OutOrStdout(), env, args[0])
		},
	}
}

func newStandaloneContextSwitchCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <cli> <name>",
		Short: "Switch the Active Context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return switchContext(cmd.OutOrStdout(), env, args[0], args[1])
		},
	}
}

func newStandaloneContextRemoveCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <cli> <name>",
		Short: "Remove a Context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeContext(cmd.OutOrStdout(), cmd.ErrOrStderr(), env, args[0], args[1])
		},
	}
}

func newStandaloneContextShowCommand(env CommandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "show <cli> <name>",
		Short: "Show a Context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showContext(cmd.OutOrStdout(), env, args[0], args[1])
		},
	}
}

func newContextAddCommand(env CommandEnv, managedCLI string) *cobra.Command {
	flags := enrollmentFlags{}
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a Context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return addContext(cmd.OutOrStdout(), cmd.ErrOrStderr(), env, managedCLI, args[0], flags, cmd)
		},
	}
	registerEnrollmentFlags(cmd, managedCLI, &flags)
	return cmd
}

func newContextListCommand(env CommandEnv, managedCLI string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listContexts(cmd.OutOrStdout(), env, managedCLI, false)
		},
	}
}

func newContextCurrentCommand(env CommandEnv, managedCLI string) *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the Active Context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return currentContext(cmd.OutOrStdout(), env, managedCLI)
		},
	}
}

func newContextSwitchCommand(env CommandEnv, managedCLI string) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name>",
		Short: "Switch the Active Context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return switchContext(cmd.OutOrStdout(), env, managedCLI, args[0])
		},
	}
}

func newContextRemoveCommand(env CommandEnv, managedCLI string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a Context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeContext(cmd.OutOrStdout(), cmd.ErrOrStderr(), env, managedCLI, args[0])
		},
	}
}

func newContextShowCommand(env CommandEnv, managedCLI string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a Context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showContext(cmd.OutOrStdout(), env, managedCLI, args[0])
		},
	}
}

type enrollmentFlags struct {
	uri             string
	host            string
	port            string
	username        string
	password        string
	authSource      string
	defaultDatabase string
	sslmode         string
	tls             bool
}

func registerEnrollmentFlags(cmd *cobra.Command, managedCLI string, flags *enrollmentFlags) {
	switch managedCLI {
	case "mongosh":
		cmd.Flags().StringVar(&flags.uri, "uri", "", "MongoDB URI")
		cmd.Flags().StringVar(&flags.username, "username", "", "username")
		cmd.Flags().StringVar(&flags.password, "password", "", "password Secret Material")
		cmd.Flags().StringVar(&flags.authSource, "auth-source", "", "authentication database")
		cmd.Flags().StringVar(&flags.defaultDatabase, "default-database", "", "default database")
	case "psql":
		cmd.Flags().StringVar(&flags.host, "host", "", "host")
		cmd.Flags().StringVar(&flags.port, "port", "", "port")
		cmd.Flags().StringVar(&flags.username, "username", "", "username")
		cmd.Flags().StringVar(&flags.password, "password", "", "password Secret Material")
		cmd.Flags().StringVar(&flags.defaultDatabase, "default-database", "", "default database")
		cmd.Flags().StringVar(&flags.sslmode, "sslmode", "", "SSL mode")
	case "redis-cli":
		cmd.Flags().StringVar(&flags.host, "host", "", "host")
		cmd.Flags().StringVar(&flags.port, "port", "", "port")
		cmd.Flags().StringVar(&flags.username, "username", "", "username")
		cmd.Flags().StringVar(&flags.password, "password", "", "password Secret Material")
		cmd.Flags().StringVar(&flags.defaultDatabase, "default-database", "", "default database index")
		cmd.Flags().BoolVar(&flags.tls, "tls", false, "enable TLS")
	}
}

func addContext(stdout, stderr io.Writer, env CommandEnv, managedCLI, name string, flags enrollmentFlags, cmd *cobra.Command) error {
	if err := validateManagedCLI(managedCLI); err != nil {
		return err
	}
	if err := validateContextName(name); err != nil {
		return err
	}
	ctx, secretValues, err := contextFromFlags(managedCLI, name, flags, cmd)
	if err != nil {
		return err
	}

	config, err := env.Store.ReadConfig()
	if err != nil {
		return err
	}
	if config.ManagedCLIs == nil {
		config.ManagedCLIs = map[string]contextstore.ManagedCLIConfig{}
	}
	managedConfig := config.ManagedCLIs[managedCLI]
	if managedConfig.Contexts == nil {
		managedConfig.Contexts = map[string]contextstore.Context{}
	}
	oldContext, hadOldContext := managedConfig.Contexts[name]

	for field, value := range secretValues {
		if err := env.Secrets.Set(ctx.Secrets[field], value); err != nil {
			return fmt.Errorf("store secret %s: %w", field, err)
		}
	}

	managedConfig.Contexts[name] = ctx
	config.ManagedCLIs[managedCLI] = managedConfig
	if err := env.Store.WriteConfig(config); err != nil {
		return err
	}

	if hadOldContext {
		deleteOldSecrets(stderr, env.Secrets, oldContext, ctx)
	}
	fmt.Fprintf(stdout, "added context %s for %s\n", name, managedCLI)
	return nil
}

func contextFromFlags(managedCLI, name string, flags enrollmentFlags, cmd *cobra.Command) (contextstore.Context, map[string]string, error) {
	metadata := map[string]any{}
	secretRefs := map[string]secrets.SecretRef{}
	secretValues := map[string]string{}

	setString := func(key, value string) {
		if value != "" {
			metadata[key] = value
		}
	}

	switch managedCLI {
	case "mongosh":
		if flags.uri == "" {
			return contextstore.Context{}, nil, fmt.Errorf("--uri is required for mongosh")
		}
		setString("uri", flags.uri)
		setString("username", flags.username)
		setString("authSource", flags.authSource)
		setString("defaultDatabase", flags.defaultDatabase)
	case "psql":
		if flags.host == "" {
			return contextstore.Context{}, nil, fmt.Errorf("--host is required for psql")
		}
		setString("host", flags.host)
		setString("port", flags.port)
		setString("username", flags.username)
		setString("defaultDatabase", flags.defaultDatabase)
		setString("sslmode", flags.sslmode)
	case "redis-cli":
		if flags.host == "" {
			return contextstore.Context{}, nil, fmt.Errorf("--host is required for redis-cli")
		}
		setString("host", flags.host)
		setString("port", flags.port)
		setString("username", flags.username)
		setString("defaultDatabase", flags.defaultDatabase)
		if flags.tls {
			metadata["tls"] = true
		}
	default:
		return contextstore.Context{}, nil, fmt.Errorf("unsupported Managed CLI %s", managedCLI)
	}

	if cmd.Flags().Changed("password") && flags.password != "" {
		secretRefs["password"] = secrets.NewRef(managedCLI, name, "password")
		secretValues["password"] = flags.password
	}

	return contextstore.Context{Metadata: metadata, Secrets: secretRefs}, secretValues, nil
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
	for _, name := range names {
		marker := " "
		if state.ActiveContexts[managedCLI] == name {
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
	for _, key := range sortedMetadataKeys(ctx.Metadata) {
		fmt.Fprintf(stdout, "%s: %v\n", displayMetadataKey(key), ctx.Metadata[key])
	}
	for _, key := range sortedSecretKeys(ctx.Secrets) {
		fmt.Fprintf(stdout, "%s: <stored>\n", key)
	}
	active := "no"
	if state.ActiveContexts[managedCLI] == name {
		active = "yes"
	}
	fmt.Fprintf(stdout, "active: %s\n", active)
	return nil
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
			fmt.Fprintf(stderr, "cloak: warning: failed to delete old secret %s: %v\n", field, err)
		}
	}
}

func validateManagedCLI(managedCLI string) error {
	if !adapters.IsSupported(managedCLI) {
		return fmt.Errorf("unsupported Managed CLI %s", managedCLI)
	}
	return nil
}

func validateContextName(name string) error {
	if !contextNamePattern.MatchString(name) {
		return fmt.Errorf("invalid context name %q", name)
	}
	return nil
}

func sortedManagedCLIs(config contextstore.Config) []string {
	names := make([]string, 0, len(config.ManagedCLIs))
	for name := range config.ManagedCLIs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

func displayMetadataKey(key string) string {
	labels := map[string]string{
		"authSource":      "auth source",
		"defaultDatabase": "default database",
		"sslmode":         "sslmode",
		"tls":             "tls",
		"uri":             "uri",
		"host":            "host",
		"port":            "port",
		"username":        "username",
	}
	if label, ok := labels[key]; ok {
		return label
	}
	return strings.ToLower(key[:1]) + key[1:]
}
