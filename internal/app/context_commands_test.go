package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lakisyaman/cloak/internal/contextstore"
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
		Store:      store,
		Secrets:    secretStore,
		Resolver:   fakeResolver{path: "/real/psql"},
		Connectors: testConnectorStore(t),
	}

	var addOut bytes.Buffer
	addCmd := NewRootCommandWithEnv("test", env)
	addCmd.SetArgs([]string{"psql", "context", "configure", "production", "--host", "db.example.com", "--username", "app", "--password", "secret", "--default-database", "appdb"})
	addCmd.SetOut(&addOut)
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("context add: %v", err)
	}
	if !strings.Contains(addOut.String(), "configured context production for psql") {
		t.Fatalf("unexpected add output: %q", addOut.String())
	}

	var switchOut bytes.Buffer
	switchCmd := NewRootCommandWithEnv("test", env)
	switchCmd.SetArgs([]string{"psql", "context", "switch", "production"})
	switchCmd.SetOut(&switchOut)
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("context switch: %v", err)
	}

	var showOut bytes.Buffer
	showCmd := NewRootCommandWithEnv("test", env)
	showCmd.SetArgs([]string{"psql", "context", "show", "production"})
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
		Version:    "test",
		Argv0:      "/tmp/shims/psql",
		Args:       []string{"-c", "select 1"},
		Stderr:     &stderr,
		Resolver:   fakeResolver{path: "/real/psql"},
		Delegate:   delegate,
		Store:      store,
		Connectors: env.Connectors,
		Secrets:    secretStore,
		Env:        []string{},
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

func TestContextCurrentAndListReportSessionContext(t *testing.T) {
	store := newMutableContextStore()
	store.config.ManagedCLIs = map[string]contextstore.ManagedCLIConfig{"psql": {Contexts: map[string]contextstore.Context{"production": {}, "staging": {}}}}
	store.state.ActiveContexts = map[string]string{"psql": "production"}
	env := CommandEnv{Store: store, Secrets: fakeSecretStore{}, Resolver: fakeResolver{path: "/real/psql"}, Connectors: testConnectorStore(t)}
	run := func(args ...string) string {
		var out bytes.Buffer
		cmd := NewRootCommandWithEnv("test", env)
		cmd.SetArgs(args)
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}

	t.Setenv("CLOAK_PSQL_CONTEXT", "")
	if got := run("psql", "context", "current"); got != "production\n" {
		t.Fatalf("expected state context, got %q", got)
	}
	t.Setenv("CLOAK_PSQL_CONTEXT", "staging")
	if got := run("psql", "context", "current"); got != "staging (from CLOAK_PSQL_CONTEXT)\n" {
		t.Fatalf("expected session context, got %q", got)
	}
	if got := run("psql", "context", "list"); got != "  production\n* staging\n" {
		t.Fatalf("expected session context marked, got %q", got)
	}
}
