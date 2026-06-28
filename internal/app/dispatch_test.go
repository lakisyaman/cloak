package app

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lakisyaman/cloak/internal/adapters"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
)

type fakeResolver struct {
	path string
	err  error
}

func (resolver fakeResolver) Resolve(commandName string) (string, error) {
	if resolver.err != nil {
		return "", resolver.err
	}
	return resolver.path, nil
}

type recordingDelegate struct {
	realPath string
	argv     []string
	env      []string
	called   bool
}

func (delegate *recordingDelegate) Exec(realPath string, argv []string, env []string) error {
	delegate.called = true
	delegate.realPath = realPath
	delegate.argv = append([]string(nil), argv...)
	delegate.env = append([]string(nil), env...)
	return nil
}

type memoryRepository struct {
	config   contextstore.Config
	state    contextstore.State
	stateErr error
}

func (repo memoryRepository) ReadConfig() (contextstore.Config, error) { return repo.config, nil }
func (repo memoryRepository) ReadState() (contextstore.State, error) {
	if repo.stateErr != nil {
		return contextstore.State{}, repo.stateErr
	}
	return repo.state, nil
}
func (repo memoryRepository) WriteConfig(contextstore.Config) error { return nil }
func (repo memoryRepository) WriteState(contextstore.State) error   { return nil }

type fakeAdapterRegistry map[string]adapters.Adapter

func (registry fakeAdapterRegistry) Get(managedCLI string) (adapters.Adapter, bool) {
	adapter, ok := registry[managedCLI]
	return adapter, ok
}

type fakeActivationAdapter struct {
	name     string
	explicit bool
	err      error
}

func (adapter fakeActivationAdapter) Name() string { return adapter.name }
func (adapter fakeActivationAdapter) DetectExplicitConnectionInput(args []string, env []string) bool {
	return adapter.explicit
}
func (adapter fakeActivationAdapter) Activate(invocation adapters.Invocation, ctx contextstore.Context, secretValues map[string]string) (adapters.ActivatedInvocation, error) {
	if adapter.err != nil {
		return adapters.ActivatedInvocation{}, adapter.err
	}
	activatedArgs := append([]string{"--activated", secretValues["password"]}, invocation.Args...)
	activatedEnv := append(append([]string(nil), invocation.Env...), "ACTIVATED=1")
	return adapters.ActivatedInvocation{Args: activatedArgs, Env: activatedEnv}, nil
}

type fakeSecretStore struct {
	values map[string]string
	err    error
}

func (store fakeSecretStore) Set(ref secrets.SecretRef, value string) error {
	if store.values != nil {
		store.values[ref.Service+"/"+ref.User] = value
	}
	return nil
}
func (store fakeSecretStore) Delete(ref secrets.SecretRef) error {
	if store.values != nil {
		delete(store.values, ref.Service+"/"+ref.User)
	}
	return nil
}
func (store fakeSecretStore) Get(ref secrets.SecretRef) (string, error) {
	if store.err != nil {
		return "", store.err
	}
	value, ok := store.values[ref.Service+"/"+ref.User]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return value, nil
}

func TestExecuteInvocationStandaloneRoutesToRootCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version: "test",
		Argv0:   "/usr/local/bin/cloak",
		Args:    []string{"--help"},
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatalf("ExecuteInvocation returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Add Context management to supported CLIs") {
		t.Fatalf("expected standalone help output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecuteInvocationShimControlRoutesToShimCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version: "test",
		Argv0:   "/tmp/shims/psql",
		Args:    []string{"cloak", "context", "list"},
		Stdout:  &stdout,
		Stderr:  &stderr,
		Store:   memoryRepository{config: contextstore.EmptyConfig(), state: contextstore.EmptyState()},
		Secrets: fakeSecretStore{},
	})
	if err != nil {
		t.Fatalf("shim-scoped context list returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "no contexts") {
		t.Fatalf("expected shim context list output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecuteInvocationShimPassThroughResolvesAndDelegates(t *testing.T) {
	var stdout, stderr bytes.Buffer
	delegate := &recordingDelegate{}
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version:  "test",
		Argv0:    "/tmp/shims/psql",
		Args:     []string{"-c", "select 1"},
		Stdout:   &stdout,
		Stderr:   &stderr,
		Resolver: fakeResolver{path: "/real/psql"},
		Delegate: delegate,
		Store:    memoryRepository{config: contextstore.EmptyConfig(), state: contextstore.EmptyState()},
		Env:      []string{"A=B"},
	})
	if err != nil {
		t.Fatalf("ExecuteInvocation returned error: %v", err)
	}
	if delegate.realPath != "/real/psql" {
		t.Fatalf("expected delegate real path /real/psql, got %q", delegate.realPath)
	}
	wantArgv := []string{"/real/psql", "-c", "select 1"}
	if strings.Join(delegate.argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("expected argv %#v, got %#v", wantArgv, delegate.argv)
	}
	if !strings.Contains(stderr.String(), "cloak: no active context for psql; running without activation") {
		t.Fatalf("expected no-active-context Invocation Notice, got %q", stderr.String())
	}
}

func TestExecuteInvocationActivatesActiveContext(t *testing.T) {
	var stderr bytes.Buffer
	delegate := &recordingDelegate{}
	secretRef := secrets.NewRef("psql", "production", "password")
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version:  "test",
		Argv0:    "/tmp/shims/psql",
		Args:     []string{"-c", "select 1"},
		Stderr:   &stderr,
		Resolver: fakeResolver{path: "/real/psql"},
		Delegate: delegate,
		Store: memoryRepository{
			config: contextstore.Config{Version: contextstore.Version, ManagedCLIs: map[string]contextstore.ManagedCLIConfig{
				"psql": {Contexts: map[string]contextstore.Context{
					"production": {Metadata: map[string]any{"host": "db.example.com"}, Secrets: map[string]secrets.SecretRef{"password": secretRef}},
				}},
			}},
			state: contextstore.State{Version: contextstore.Version, ActiveContexts: map[string]string{"psql": "production"}},
		},
		Adapters: fakeAdapterRegistry{"psql": fakeActivationAdapter{name: "psql"}},
		Secrets:  fakeSecretStore{values: map[string]string{secretRef.Service + "/" + secretRef.User: "secret"}},
		Env:      []string{"A=B"},
	})
	if err != nil {
		t.Fatalf("ExecuteInvocation returned error: %v", err)
	}
	wantArgv := []string{"/real/psql", "--activated", "secret", "-c", "select 1"}
	if strings.Join(delegate.argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("expected activated argv %#v, got %#v", wantArgv, delegate.argv)
	}
	if !contains(delegate.env, "ACTIVATED=1") {
		t.Fatalf("expected activated env, got %#v", delegate.env)
	}
	if !strings.Contains(stderr.String(), "cloak: activated psql context production") {
		t.Fatalf("expected activated Invocation Notice, got %q", stderr.String())
	}
}

func TestExecuteInvocationExplicitConnectionInputPassesThroughActiveContext(t *testing.T) {
	var stderr bytes.Buffer
	delegate := &recordingDelegate{}
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version:  "test",
		Argv0:    "/tmp/shims/psql",
		Args:     []string{"--host", "custom", "-c", "select 1"},
		Stderr:   &stderr,
		Resolver: fakeResolver{path: "/real/psql"},
		Delegate: delegate,
		Store: memoryRepository{
			config: contextstore.EmptyConfig(),
			state:  contextstore.State{Version: contextstore.Version, ActiveContexts: map[string]string{"psql": "production"}},
		},
		Adapters: fakeAdapterRegistry{"psql": fakeActivationAdapter{name: "psql", explicit: true}},
		Secrets:  fakeSecretStore{},
		Env:      []string{"A=B"},
	})
	if err != nil {
		t.Fatalf("ExecuteInvocation returned error: %v", err)
	}
	wantArgv := []string{"/real/psql", "--host", "custom", "-c", "select 1"}
	if strings.Join(delegate.argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("expected pass-through argv %#v, got %#v", wantArgv, delegate.argv)
	}
	if !strings.Contains(stderr.String(), "cloak: explicit connection input detected for psql; running without activation") {
		t.Fatalf("expected explicit-input Invocation Notice, got %q", stderr.String())
	}
}

func TestExecuteInvocationMissingSecretFailsClosed(t *testing.T) {
	var stderr bytes.Buffer
	delegate := &recordingDelegate{}
	secretRef := secrets.NewRef("psql", "production", "password")
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version:  "test",
		Argv0:    "/tmp/shims/psql",
		Args:     []string{"-c", "select 1"},
		Stderr:   &stderr,
		Resolver: fakeResolver{path: "/real/psql"},
		Delegate: delegate,
		Store: memoryRepository{
			config: contextstore.Config{Version: contextstore.Version, ManagedCLIs: map[string]contextstore.ManagedCLIConfig{
				"psql": {Contexts: map[string]contextstore.Context{
					"production": {Metadata: map[string]any{"host": "db.example.com"}, Secrets: map[string]secrets.SecretRef{"password": secretRef}},
				}},
			}},
			state: contextstore.State{Version: contextstore.Version, ActiveContexts: map[string]string{"psql": "production"}},
		},
		Adapters: fakeAdapterRegistry{"psql": fakeActivationAdapter{name: "psql"}},
		Secrets:  fakeSecretStore{},
		Env:      []string{"A=B"},
	})
	if err == nil {
		t.Fatalf("expected missing secret error")
	}
	if delegate.called {
		t.Fatalf("expected missing secret to fail closed without delegating")
	}
	if !strings.Contains(stderr.String(), "cloak: failed to activate psql context production: missing secret password") {
		t.Fatalf("expected ActivationFailed notice, got %q", stderr.String())
	}
}

func TestExecuteInvocationCorruptStateFailsLoudWithoutDelegating(t *testing.T) {
	delegate := &recordingDelegate{}
	errCorrupt := errors.New("invalid json")
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version:  "test",
		Argv0:    "/tmp/shims/psql",
		Args:     []string{"-c", "select 1"},
		Resolver: fakeResolver{path: "/real/psql"},
		Delegate: delegate,
		Store:    memoryRepository{stateErr: errCorrupt},
	})
	if !errors.Is(err, errCorrupt) {
		t.Fatalf("expected corrupt state error, got %v", err)
	}
	if delegate.called {
		t.Fatalf("expected corrupt state to fail loudly without delegating")
	}
}

func TestExecuteInvocationShimPassThroughReturnsResolutionErrors(t *testing.T) {
	errBoom := errors.New("boom")
	err := ExecuteInvocationWithOptions(InvocationOptions{
		Version:  "test",
		Argv0:    "/tmp/shims/psql",
		Args:     []string{"-c", "select 1"},
		Resolver: fakeResolver{err: errBoom},
		Delegate: &recordingDelegate{},
		Store:    memoryRepository{config: contextstore.EmptyConfig(), state: contextstore.EmptyState()},
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected resolver error, got %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
