package app

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"cloak/internal/adapters"
	"cloak/internal/contextstore"
	"cloak/internal/notice"
	"cloak/internal/secrets"
	"cloak/internal/shim"
)

type RealCommandResolver interface {
	Resolve(commandName string) (string, error)
}

type RealCommandDelegate interface {
	Exec(realPath string, argv []string, env []string) error
}

type RealCommandDelegateFunc func(realPath string, argv []string, env []string) error

func (fn RealCommandDelegateFunc) Exec(realPath string, argv []string, env []string) error {
	return fn(realPath, argv, env)
}

type AdapterRegistry interface {
	Get(managedCLI string) (adapters.Adapter, bool)
}

type adapterRegistryFunc func(managedCLI string) (adapters.Adapter, bool)

func (fn adapterRegistryFunc) Get(managedCLI string) (adapters.Adapter, bool) {
	return fn(managedCLI)
}

type InvocationOptions struct {
	Version  string
	Argv0    string
	Args     []string
	Stdout   io.Writer
	Stderr   io.Writer
	Resolver RealCommandResolver
	Delegate RealCommandDelegate
	Store    ContextStore
	Adapters AdapterRegistry
	Secrets  secrets.Store
	Env      []string
}

func ExecuteInvocation(version, argv0 string, args []string, stdout, stderr io.Writer) error {
	return ExecuteInvocationWithOptions(InvocationOptions{
		Version: version,
		Argv0:   argv0,
		Args:    args,
		Stdout:  stdout,
		Stderr:  stderr,
	})
}

func ExecuteInvocationWithOptions(options InvocationOptions) error {
	prepared, err := prepareInvocationOptions(options)
	if err != nil {
		return err
	}
	options = prepared

	invocation := shim.DetectInvocation(options.Argv0)
	if invocation.Mode == shim.StandaloneMode {
		return Execute(options.Version, options.Args, options.Stdout, options.Stderr)
	}

	if shim.IsControlPrefix(options.Args) {
		cmd := NewShimControlCommandWithEnv(options.Version, invocation.ManagedCLI, CommandEnv{
			Store:           options.Store,
			Secrets:         options.Secrets,
			Resolver:        options.Resolver,
			Adapters:        options.Adapters,
			PathEnv:         os.Getenv("PATH"),
			CloakBinaryPath: options.Argv0,
		})
		cmd.SetArgs(options.Args)
		cmd.SetOut(options.Stdout)
		cmd.SetErr(options.Stderr)
		return cmd.Execute()
	}

	realPath, err := options.Resolver.Resolve(invocation.ManagedCLI)
	if err != nil {
		return err
	}

	state, err := options.Store.ReadState()
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	activeContextName := state.ActiveContexts[invocation.ManagedCLI]
	if activeContextName == "" {
		notice.NoActiveContext(options.Stderr, invocation.ManagedCLI)
		return delegateUnchanged(options, realPath)
	}

	adapter, ok := options.Adapters.Get(invocation.ManagedCLI)
	if !ok {
		notice.ActivationFailed(options.Stderr, invocation.ManagedCLI, activeContextName, "missing adapter")
		return fmt.Errorf("missing adapter for Managed CLI %s", invocation.ManagedCLI)
	}

	if adapter.DetectExplicitConnectionInput(options.Args, options.Env) {
		notice.ExplicitConnectionInput(options.Stderr, invocation.ManagedCLI)
		return delegateUnchanged(options, realPath)
	}

	config, err := options.Store.ReadConfig()
	if err != nil {
		notice.ActivationFailed(options.Stderr, invocation.ManagedCLI, activeContextName, "config could not be loaded")
		return fmt.Errorf("read config: %w", err)
	}

	ctx, ok := findContext(config, invocation.ManagedCLI, activeContextName)
	if !ok {
		notice.ActivationFailed(options.Stderr, invocation.ManagedCLI, activeContextName, "active context not found")
		return fmt.Errorf("active context %s/%s not found", invocation.ManagedCLI, activeContextName)
	}

	secretValues, err := loadSecretValues(options.Secrets, ctx)
	if err != nil {
		notice.ActivationFailed(options.Stderr, invocation.ManagedCLI, activeContextName, err.Error())
		return err
	}

	activated, err := adapter.Activate(adapters.Invocation{Args: options.Args, Env: options.Env}, ctx, secretValues)
	if err != nil {
		notice.ActivationFailed(options.Stderr, invocation.ManagedCLI, activeContextName, err.Error())
		return err
	}

	notice.Activated(options.Stderr, invocation.ManagedCLI, activeContextName)
	argv := append([]string{realPath}, activated.Args...)
	return options.Delegate.Exec(realPath, argv, activated.Env)
}

func prepareInvocationOptions(options InvocationOptions) (InvocationOptions, error) {
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	if options.Resolver == nil {
		options.Resolver = shim.RealCommandResolver{}
	}
	if options.Delegate == nil {
		options.Delegate = realCommandExecDelegate{}
	}
	if options.Env == nil {
		options.Env = os.Environ()
	}
	if options.Adapters == nil {
		options.Adapters = adapterRegistryFunc(adapters.Get)
	}
	if options.Secrets == nil {
		options.Secrets = secrets.KeyringStore{}
	}
	if options.Store == nil {
		paths, err := contextstore.DefaultPaths()
		if err != nil {
			return InvocationOptions{}, err
		}
		options.Store = fileContextStore{configPath: paths.ConfigFile, statePath: paths.StateFile}
	}
	return options, nil
}

func delegateUnchanged(options InvocationOptions, realPath string) error {
	argv := append([]string{realPath}, options.Args...)
	return options.Delegate.Exec(realPath, argv, options.Env)
}

func findContext(config contextstore.Config, managedCLI, contextName string) (contextstore.Context, bool) {
	managedConfig, ok := config.ManagedCLIs[managedCLI]
	if !ok {
		return contextstore.Context{}, false
	}
	ctx, ok := managedConfig.Contexts[contextName]
	return ctx, ok
}

func loadSecretValues(store secrets.Store, ctx contextstore.Context) (map[string]string, error) {
	values := map[string]string{}
	for field, ref := range ctx.Secrets {
		value, err := store.Get(ref)
		if err != nil {
			return nil, fmt.Errorf("missing secret %s", field)
		}
		values[field] = value
	}
	return values, nil
}

type realCommandExecDelegate struct{}

func (realCommandExecDelegate) Exec(realPath string, argv []string, env []string) error {
	return syscall.Exec(realPath, argv, env)
}
