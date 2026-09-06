package app

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/notice"
	"github.com/lakisyaman/cloak/internal/secrets"
	"github.com/lakisyaman/cloak/internal/shim"
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

type InvocationOptions struct {
	Version    string
	Argv0      string
	Args       []string
	Stdout     io.Writer
	Stderr     io.Writer
	Resolver   RealCommandResolver
	Delegate   RealCommandDelegate
	Store      ContextStore
	Connectors *connectors.Store
	Paths      contextstore.Paths
	Secrets    secrets.Store
	Env        []string
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
		cmd := NewRootCommandWithEnv(options.Version, CommandEnv{Paths: options.Paths, Store: options.Store, Secrets: options.Secrets, Resolver: options.Resolver, Connectors: options.Connectors})
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

	connector, err := options.Connectors.Get(invocation.ManagedCLI)
	if err != nil {
		notice.ActivationFailed(options.Stderr, invocation.ManagedCLI, activeContextName, "Connector unavailable")
		return err
	}

	if connector.Detect(connectors.Invocation{Args: options.Args, Env: options.Env}).Passthrough {
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
		return fmt.Errorf("active context not found; run cloak %s context configure %s", invocation.ManagedCLI, activeContextName)
	}

	activated, err := connector.Activate(connectors.Invocation{Args: options.Args, Env: options.Env}, ctx, options.Secrets)
	if err != nil {
		notice.ActivationFailed(options.Stderr, invocation.ManagedCLI, activeContextName, err.Error())
		return fmt.Errorf("%w; run cloak %s context configure %s", err, invocation.ManagedCLI, activeContextName)
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
	if options.Secrets == nil {
		options.Secrets = secrets.KeyringStore{}
	}
	env := normalizeCommandEnv(CommandEnv{Paths: options.Paths, Store: options.Store, Connectors: options.Connectors})
	options.Paths, options.Store, options.Connectors = env.Paths, env.Store, env.Connectors
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

type realCommandExecDelegate struct{}

func (realCommandExecDelegate) Exec(realPath string, argv []string, env []string) error {
	return syscall.Exec(realPath, argv, env)
}
