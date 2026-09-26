package app

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"context"
	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
	"path/filepath"
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

func testConnectorStore(t *testing.T) *connectors.Store {
	t.Helper()
	store := &connectors.Store{Dir: t.TempDir()}
	for _, name := range []string{"psql", "redis-cli", "mongosh"} {
		record, _, err := store.Acquire(context.Background(), filepath.Join("..", "..", "registry", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err = store.Write(record); err != nil {
			t.Fatal(err)
		}
	}
	return store
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

func TestOldControlPrefixDelegatesUnchanged(t *testing.T) {
	delegate := &recordingDelegate{}
	err := ExecuteInvocationWithOptions(InvocationOptions{Argv0: "/shims/psql", Args: []string{"cloak", "context", "list"}, Store: newMutableContextStore(), Resolver: fakeResolver{path: "/real/psql"}, Delegate: delegate})
	if err != nil {
		t.Fatal(err)
	}
	if !delegate.called || strings.Join(delegate.argv, " ") != "/real/psql cloak context list" {
		t.Fatalf("old prefix was intercepted: %#v", delegate)
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
		Connectors: testConnectorStore(t),
		Secrets:    fakeSecretStore{values: map[string]string{secretRef.Service + "/" + secretRef.User: "secret"}},
		Env:        []string{"A=B"},
	})
	if err != nil {
		t.Fatalf("ExecuteInvocation returned error: %v", err)
	}
	wantArgv := []string{"/real/psql", "-c", "select 1"}
	if strings.Join(delegate.argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("expected activated argv %#v, got %#v", wantArgv, delegate.argv)
	}
	if !contains(delegate.env, "PGPASSWORD=secret") {
		t.Fatalf("expected activated env, got %#v", delegate.env)
	}
	if stderr.String() != "cloak: activated psql context production\n" {
		t.Fatalf("expected activated Invocation Notice, got %q", stderr.String())
	}
}

func sessionContextOptions(t *testing.T, args, env []string, stderr *bytes.Buffer, delegate *recordingDelegate) InvocationOptions {
	t.Helper()
	return InvocationOptions{
		Argv0:    "/tmp/shims/psql",
		Args:     args,
		Stderr:   stderr,
		Resolver: fakeResolver{path: "/real/psql"},
		Delegate: delegate,
		Store: memoryRepository{
			config: contextstore.Config{Version: contextstore.Version, ManagedCLIs: map[string]contextstore.ManagedCLIConfig{
				"psql": {Contexts: map[string]contextstore.Context{
					"production": {Metadata: map[string]any{"host": "prod.example.com"}},
					"staging":    {Metadata: map[string]any{"host": "staging.example.com"}},
				}},
			}},
			state: contextstore.State{Version: contextstore.Version, ActiveContexts: map[string]string{"psql": "production"}},
		},
		Connectors: testConnectorStore(t),
		Secrets:    fakeSecretStore{},
		Env:        env,
	}
}

func TestExecuteInvocationSessionContextWinsOverState(t *testing.T) {
	var stderr bytes.Buffer
	delegate := &recordingDelegate{}
	err := ExecuteInvocationWithOptions(sessionContextOptions(t, []string{"-c", "select 1"}, []string{"CLOAK_PSQL_CONTEXT=staging"}, &stderr, delegate))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(delegate.env, "PGHOST=staging.example.com") {
		t.Fatalf("expected staging activation, got %#v", delegate.env)
	}
	if stderr.String() != "cloak: activated psql context staging (from CLOAK_PSQL_CONTEXT)\n" {
		t.Fatalf("expected session Invocation Notice, got %q", stderr.String())
	}
}

func TestExecuteInvocationUnknownSessionContextFailsClosed(t *testing.T) {
	var stderr bytes.Buffer
	delegate := &recordingDelegate{}
	err := ExecuteInvocationWithOptions(sessionContextOptions(t, []string{"-c", "select 1"}, []string{"CLOAK_PSQL_CONTEXT=missing"}, &stderr, delegate))
	if err == nil || !strings.Contains(err.Error(), "context missing from CLOAK_PSQL_CONTEXT does not exist for psql") {
		t.Fatalf("expected unknown session context error, got %v", err)
	}
	if delegate.called {
		t.Fatalf("expected unknown session context to fail closed without delegating")
	}
	if stderr.String() != "cloak: failed to activate psql context missing (from CLOAK_PSQL_CONTEXT): active context not found\n" {
		t.Fatalf("expected ActivationFailed notice, got %q", stderr.String())
	}
}

func TestExecuteInvocationExplicitConnectionInputPassesThroughSessionContext(t *testing.T) {
	var stderr bytes.Buffer
	delegate := &recordingDelegate{}
	env := []string{"CLOAK_PSQL_CONTEXT=staging"}
	err := ExecuteInvocationWithOptions(sessionContextOptions(t, []string{"--host", "custom", "-c", "select 1"}, env, &stderr, delegate))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(delegate.argv, " ") != "/real/psql --host custom -c select 1" || strings.Join(delegate.env, " ") != "CLOAK_PSQL_CONTEXT=staging" {
		t.Fatalf("expected unchanged pass-through, got argv %#v env %#v", delegate.argv, delegate.env)
	}
	if !strings.Contains(stderr.String(), "cloak: explicit connection input detected for psql") {
		t.Fatalf("expected explicit-input Invocation Notice, got %q", stderr.String())
	}
}

func TestSessionContextVariable(t *testing.T) {
	for _, tc := range []struct {
		cli, env, wantName, wantVariable string
	}{
		{"psql", "CLOAK_PSQL_CONTEXT=prod", "prod", "CLOAK_PSQL_CONTEXT"},
		{"redis-cli", "CLOAK_REDIS_CLI_CONTEXT=cache", "cache", "CLOAK_REDIS_CLI_CONTEXT"},
		{"psql", "CLOAK_PSQL_CONTEXT=", "", ""},
		{"psql", "CLOAK_REDIS_CLI_CONTEXT=cache", "", ""},
	} {
		name, variable := sessionContext(tc.cli, []string{tc.env})
		if name != tc.wantName || variable != tc.wantVariable {
			t.Fatalf("sessionContext(%q, %q) = %q, %q", tc.cli, tc.env, name, variable)
		}
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
		Connectors: testConnectorStore(t),
		Secrets:    fakeSecretStore{},
		Env:        []string{"A=B"},
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
		Connectors: testConnectorStore(t),
		Secrets:    fakeSecretStore{},
		Env:        []string{"A=B"},
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
