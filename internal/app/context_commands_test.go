package app

import (
	"bytes"
	"strings"
	"testing"

	"cloak/internal/adapters"
	"cloak/internal/contextstore"
)

type mutableContextStore struct {
	config contextstore.Config
	state  contextstore.State
}

func newMutableContextStore() *mutableContextStore {
	return &mutableContextStore{config: contextstore.EmptyConfig(), state: contextstore.EmptyState()}
}

func (store *mutableContextStore) ReadConfig() (contextstore.Config, error) { return store.config, nil }
func (store *mutableContextStore) ReadState() (contextstore.State, error)   { return store.state, nil }
func (store *mutableContextStore) WriteConfig(config contextstore.Config) error {
	store.config = config
	return nil
}
func (store *mutableContextStore) WriteState(state contextstore.State) error {
	store.state = state
	return nil
}

func TestContextAddSwitchShowAndActivationThroughCLI(t *testing.T) {
	store := newMutableContextStore()
	secretStore := fakeSecretStore{values: map[string]string{}}
	env := CommandEnv{
		Store:    store,
		Secrets:  secretStore,
		Resolver: fakeResolver{path: "/real/psql"},
		Adapters: adapterRegistryFunc(adapters.Get),
	}

	var addOut bytes.Buffer
	addCmd := NewShimControlCommandWithEnv("test", "psql", env)
	addCmd.SetArgs([]string{"cloak", "context", "add", "production", "--host", "db.example.com", "--username", "app", "--password", "secret", "--default-database", "appdb"})
	addCmd.SetOut(&addOut)
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("context add: %v", err)
	}
	if !strings.Contains(addOut.String(), "added context production for psql") {
		t.Fatalf("unexpected add output: %q", addOut.String())
	}

	var switchOut bytes.Buffer
	switchCmd := NewShimControlCommandWithEnv("test", "psql", env)
	switchCmd.SetArgs([]string{"cloak", "context", "switch", "production"})
	switchCmd.SetOut(&switchOut)
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("context switch: %v", err)
	}

	var showOut bytes.Buffer
	showCmd := NewShimControlCommandWithEnv("test", "psql", env)
	showCmd.SetArgs([]string{"cloak", "context", "show", "production"})
	showCmd.SetOut(&showOut)
	if err := showCmd.Execute(); err != nil {
		t.Fatalf("context show: %v", err)
	}
	if !strings.Contains(showOut.String(), "password: <stored>") || strings.Contains(showOut.String(), "secret") {
		t.Fatalf("context show did not redact Secret Material: %q", showOut.String())
	}

	var stderr bytes.Buffer
	delegate := &recordingDelegate{}
	if err := ExecuteInvocationWithOptions(InvocationOptions{
		Version:  "test",
		Argv0:    "/tmp/shims/psql",
		Args:     []string{"-c", "select 1"},
		Stderr:   &stderr,
		Resolver: fakeResolver{path: "/real/psql"},
		Delegate: delegate,
		Store:    store,
		Adapters: adapterRegistryFunc(adapters.Get),
		Secrets:  secretStore,
		Env:      []string{},
	}); err != nil {
		t.Fatalf("execute activated psql: %v", err)
	}

	joinedEnv := strings.Join(delegate.env, "\x00")
	for _, want := range []string{"PGHOST=db.example.com", "PGUSER=app", "PGPASSWORD=secret", "PGDATABASE=appdb"} {
		if !strings.Contains(joinedEnv, want) {
			t.Fatalf("expected env %q in activated env %#v", want, delegate.env)
		}
	}
	if !strings.Contains(stderr.String(), "cloak: activated psql context production") {
		t.Fatalf("expected activation notice, got %q", stderr.String())
	}
}
