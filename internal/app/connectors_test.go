package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
)

func newConnectorEnv(t *testing.T) CommandEnv {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "cloak")
	writeTestExecutable(t, binary)
	return normalizeCommandEnv(CommandEnv{Paths: contextstore.Paths{Dir: dir, ConfigFile: filepath.Join(dir, "config.json"), StateFile: filepath.Join(dir, "state.json"), ShimDir: filepath.Join(dir, "shims")}, Secrets: fakeSecretStore{values: map[string]string{}}, CloakBinaryPath: binary, Resolver: fakeResolver{path: "/real/client"}, Interactive: func() bool { return false }})
}

func runCommand(t *testing.T, env CommandEnv, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCommandWithEnv("test", env)
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func mustRun(t *testing.T, env CommandEnv, args ...string) string {
	t.Helper()
	out, err := runCommand(t, env, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

func installTestConnector(t *testing.T, env CommandEnv, name string) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), name+".yaml")
	data, err := os.ReadFile(filepath.Join("..", "..", "registry", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, env, "connector", "add", source)
	return source
}

func TestConnectorLifecycleRetainsContextsAndValidatesAtUse(t *testing.T) {
	env := newConnectorEnv(t)
	if out := mustRun(t, env, "connector", "list"); strings.TrimSpace(out) != "no connectors" {
		t.Fatal(out)
	}
	source := installTestConnector(t, env, "redis-cli")
	if _, err := os.Stat(env.Paths.ConfigFile); !os.IsNotExist(err) {
		t.Fatal("installation touched Context config")
	}
	if _, err := os.Stat(env.Paths.StateFile); !os.IsNotExist(err) {
		t.Fatal("installation touched active state")
	}
	mustRun(t, env, "redis-cli", "context", "configure", "prod", "--host", "saved", "--password", "top-secret", "--default-database", "2")
	state, _ := env.Store.ReadState()
	if state.ActiveContexts["redis-cli"] != "" {
		t.Fatal("configure selected a Context")
	}
	mustRun(t, env, "redis-cli", "context", "switch", "prod")
	before, _ := os.ReadFile(env.Paths.ConfigFile)
	definition, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	definition = append(definition, []byte("  region:\n    type: string\n    required: true\n    inject: {env: REGION}\n")...)
	if err = os.WriteFile(source, definition, 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(delegate *recordingDelegate) error {
		return ExecuteInvocationWithOptions(InvocationOptions{Argv0: "/shims/redis-cli", Args: []string{"PING"}, Store: env.Store, Connectors: env.Connectors, Secrets: env.Secrets, Resolver: env.Resolver, Delegate: delegate, Env: []string{}, Stderr: &bytes.Buffer{}})
	}
	delegate := &recordingDelegate{}
	if err = run(delegate); err != nil || !delegate.called {
		t.Fatal("source change affected installed copy", err)
	}
	mustRun(t, env, "connector", "update", "redis-cli")
	after, _ := os.ReadFile(env.Paths.ConfigFile)
	if !bytes.Equal(before, after) {
		t.Fatal("update changed Contexts")
	}
	delegate = &recordingDelegate{}
	err = run(delegate)
	if err == nil || delegate.called || !strings.Contains(err.Error(), "cloak redis-cli context configure prod") {
		t.Fatalf("update must fail closed at use: %v called=%v", err, delegate.called)
	}
	mustRun(t, env, "redis-cli", "context", "configure", "prod", "--region", "eu")
	delegate = &recordingDelegate{}
	if err = run(delegate); err != nil || !delegate.called {
		t.Fatal("repair failed", err)
	}
	if !contains(delegate.env, "REGION=eu") {
		t.Fatal("new field was not injected")
	}
	before, _ = os.ReadFile(env.Paths.ConfigFile)
	mustRun(t, env, "connector", "remove", "redis-cli")
	after, _ = os.ReadFile(env.Paths.ConfigFile)
	if !bytes.Equal(before, after) {
		t.Fatal("remove changed Contexts")
	}
	if _, err = os.Lstat(filepath.Join(env.Paths.ShimDir, "redis-cli")); !os.IsNotExist(err) {
		t.Fatal("Shim retained")
	}
	if out := mustRun(t, env, "redis-cli", "context", "show", "prod"); strings.Contains(out, "top-secret") {
		t.Fatal("retained Context leaked secret")
	}
	config, _ := env.Store.ReadConfig()
	ctx, _ := findContext(config, "redis-cli", "prod")
	if _, err = env.Secrets.Get(ctx.Secrets["password"]); err != nil {
		t.Fatal("Connector remove deleted secret")
	}
	mustRun(t, env, "connector", "add", source)
	delegate = &recordingDelegate{}
	if err = run(delegate); err != nil || !delegate.called {
		t.Fatal("reinstall failed", err)
	}
	mustRun(t, env, "connector", "remove", "redis-cli")
	mustRun(t, env, "redis-cli", "context", "remove", "prod")
	if _, err = env.Secrets.Get(ctx.Secrets["password"]); err == nil {
		t.Fatal("Context removal did not delete secret")
	}
}

func TestConnectorInstallIndependentOfCorruptContextFiles(t *testing.T) {
	env := newConnectorEnv(t)
	if err := os.WriteFile(env.Paths.ConfigFile, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	installTestConnector(t, env, "psql")
	mustRun(t, env, "connector", "update", "psql")
	mustRun(t, env, "connector", "remove", "psql")
	data, _ := os.ReadFile(env.Paths.ConfigFile)
	if string(data) != "corrupt" {
		t.Fatal("lifecycle modified Context file")
	}
}

func TestConfigureNonInteractiveMergesAndClears(t *testing.T) {
	env := newConnectorEnv(t)
	installTestConnector(t, env, "redis-cli")
	if _, err := runCommand(t, env, "redis-cli", "context", "configure", "prod"); err == nil || !strings.Contains(err.Error(), "--host") {
		t.Fatalf("missing required value: %v", err)
	}
	mustRun(t, env, "redis-cli", "context", "configure", "prod", "--host", "saved", "--port", "6379", "--password", "first-secret", "--tls")
	mustRun(t, env, "redis-cli", "context", "configure", "prod", "--port", "6380", "--tls=false")
	config, _ := env.Store.ReadConfig()
	ctx, _ := findContext(config, "redis-cli", "prod")
	if ctx.Metadata["host"] != "saved" || ctx.Metadata["port"] != float64(6380) || ctx.Metadata["tls"] != false {
		t.Fatalf("merge lost values: %v", ctx.Metadata)
	}
	ref := ctx.Secrets["password"]
	if value, err := env.Secrets.Get(ref); err != nil || value != "first-secret" {
		t.Fatal("merge lost secret")
	}
	before, _ := os.ReadFile(env.Paths.ConfigFile)
	for _, args := range [][]string{{"--port", "hidden-secret"}, {"--clear", "host"}, {"--tls=hidden-secret"}} {
		out, err := runCommand(t, env, append([]string{"redis-cli", "context", "configure", "prod"}, args...)...)
		if err == nil || strings.Contains(err.Error()+out, "hidden-secret") {
			t.Fatalf("invalid values were accepted or echoed: %v %s", err, out)
		}
	}
	after, _ := os.ReadFile(env.Paths.ConfigFile)
	if !bytes.Equal(before, after) {
		t.Fatal("failed validation changed Context")
	}
	mustRun(t, env, "redis-cli", "context", "configure", "prod", "--clear", "password,tls")
	if _, err := env.Secrets.Get(ref); err == nil {
		t.Fatal("clear retained secret")
	}
}

func TestExplicitWizardPromptsAndKeepsValues(t *testing.T) {
	env := newConnectorEnv(t)
	installTestConnector(t, env, "psql")
	env.Interactive = func() bool { return true }
	env.ReadSecret = func() (string, error) { return "wizard-secret", nil }
	var out bytes.Buffer
	cmd := NewRootCommandWithEnv("test", env)
	cmd.SetArgs([]string{"psql", "context", "configure", "prod"})
	// Sorted fields: defaultDatabase, host, password (masked), port, sslmode, username.
	cmd.SetIn(strings.NewReader("appdb\ndb.example\n5432\nrequire\napp\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "wizard-secret") {
		t.Fatal("wizard echoed secret")
	}
	config, _ := env.Store.ReadConfig()
	ctx, _ := findContext(config, "psql", "prod")
	if ctx.Metadata["host"] != "db.example" || ctx.Metadata["username"] != "app" {
		t.Fatalf("bad wizard values %v", ctx.Metadata)
	}
	before, _ := os.ReadFile(env.Paths.ConfigFile)
	env.ReadSecret = func() (string, error) { return "", nil }
	cmd = NewRootCommandWithEnv("test", env)
	cmd.SetArgs([]string{"psql", "context", "configure", "prod"})
	cmd.SetIn(strings.NewReader("\n\n\n\n\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(env.Paths.ConfigFile)
	if !bytes.Equal(before, after) {
		t.Fatal("blank wizard entries changed values")
	}
}

type failingConfigStore struct{ ContextStore }

func (s failingConfigStore) WriteConfig(contextstore.Config) error { return errors.New("disk full") }

func TestConfigureRollsBackNewSecretsOnWriteFailure(t *testing.T) {
	env := newConnectorEnv(t)
	installTestConnector(t, env, "psql")
	mustRun(t, env, "psql", "context", "configure", "prod", "--host", "saved", "--password", "old-secret")
	config, _ := env.Store.ReadConfig()
	ctx, _ := findContext(config, "psql", "prod")
	previous := map[string]string{}
	for key, value := range env.Secrets.(fakeSecretStore).values {
		previous[key] = value
	}
	env.Store = failingConfigStore{env.Store}
	if _, err := runCommand(t, env, "psql", "context", "configure", "prod", "--password", "new-secret"); err == nil {
		t.Fatal("expected write failure")
	}
	if !reflect.DeepEqual(previous, env.Secrets.(fakeSecretStore).values) {
		t.Fatal("secret store changed after failed config write")
	}
	if value, err := env.Secrets.Get(ctx.Secrets["password"]); err != nil || value != "old-secret" {
		t.Fatal("old secret lost")
	}
}

func TestConnectorCannotReplaceUnrelatedFileOrChangeIdentity(t *testing.T) {
	env := newConnectorEnv(t)
	if err := os.MkdirAll(env.Paths.ShimDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(env.Paths.ShimDir, "psql")
	if err := os.WriteFile(link, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(t, env, "connector", "add", "../../registry/psql.yaml"); err == nil {
		t.Fatal("overwrote unrelated file")
	}
	if _, err := env.Connectors.Get("psql"); err == nil {
		t.Fatal("registered partially installed Connector")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	source := installTestConnector(t, env, "psql")
	data, _ := os.ReadFile(source)
	data = bytes.Replace(data, []byte("command: psql"), []byte("command: renamed"), 1)
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(t, env, "connector", "update", "psql"); err == nil {
		t.Fatal("accepted command identity change")
	}
	if _, err := env.Connectors.Get("psql"); err != nil {
		t.Fatal("damaged installed definition")
	}
}

func TestSecretPolicyChangeDoesNotLeakInShow(t *testing.T) {
	env := newConnectorEnv(t)
	def := "version: 1\ncommand: custom\nfields:\n  token:\n    type: string\n    secret: true\n    inject: {env: TOKEN}\n"
	if err := env.Connectors.Write(connectors.Record{Version: 1, Source: "custom.yaml", YAML: def}); err != nil {
		t.Fatal(err)
	}
	config := contextstore.EmptyConfig()
	config.ManagedCLIs["custom"] = contextstore.ManagedCLIConfig{Contexts: map[string]contextstore.Context{"prod": {Metadata: map[string]any{"token": "old-plaintext"}, Secrets: map[string]secrets.SecretRef{}}}}
	if err := env.Store.WriteConfig(config); err != nil {
		t.Fatal(err)
	}
	if out := mustRun(t, env, "custom", "context", "show", "prod"); strings.Contains(out, "old-plaintext") {
		t.Fatal("schema change leaked secret")
	}
}

func TestLegacyMongoURIRequiresExplicitSecretStorageRepair(t *testing.T) {
	env := newConnectorEnv(t)
	installTestConnector(t, env, "mongosh")
	uri := "mongodb://user:embedded-secret@localhost?authSource=admin"
	config := contextstore.EmptyConfig()
	config.ManagedCLIs["mongosh"] = contextstore.ManagedCLIConfig{Contexts: map[string]contextstore.Context{"prod": {Metadata: map[string]any{"uri": uri, "defaultDatabase": "app"}}}}
	if err := env.Store.WriteConfig(config); err != nil {
		t.Fatal(err)
	}
	mustRun(t, env, "mongosh", "context", "switch", "prod")
	if out := mustRun(t, env, "mongosh", "context", "show", "prod"); strings.Contains(out, "embedded-secret") {
		t.Fatal("legacy URI leaked")
	}
	run := func(delegate *recordingDelegate) error {
		return ExecuteInvocationWithOptions(InvocationOptions{Argv0: "/shims/mongosh", Args: []string{"--quiet"}, Store: env.Store, Connectors: env.Connectors, Secrets: env.Secrets, Resolver: env.Resolver, Delegate: delegate, Env: []string{}, Stderr: &bytes.Buffer{}})
	}
	delegate := &recordingDelegate{}
	if err := run(delegate); err == nil || delegate.called || !strings.Contains(err.Error(), "cloak mongosh context configure prod") || strings.Contains(err.Error(), "embedded-secret") {
		t.Fatalf("bad legacy Activation: %v", err)
	}
	mustRun(t, env, "mongosh", "context", "configure", "prod", "--uri", uri)
	data, err := os.ReadFile(env.Paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("embedded-secret")) {
		t.Fatal("repaired URI remained in metadata")
	}
	delegate = &recordingDelegate{}
	if err = run(delegate); err != nil {
		t.Fatal(err)
	}
	if len(delegate.argv) < 2 || delegate.argv[1] != "mongodb://user:embedded-secret@localhost/app?authSource=admin" {
		t.Fatal("embedded URI authentication or default database lost")
	}
}

func TestWizardCancellationAndValidationDoNotMutateMemoryStore(t *testing.T) {
	env := newConnectorEnv(t)
	installTestConnector(t, env, "psql")
	memory := newMutableContextStore()
	env.Store = memory
	mustRun(t, env, "psql", "context", "configure", "prod", "--host", "saved")
	before := memory.config.ManagedCLIs["psql"].Contexts["prod"].Metadata["host"]
	if _, err := runCommand(t, env, "psql", "context", "configure", "prod", "--host", "changed", "--port", "invalid"); err == nil {
		t.Fatal("expected validation error")
	}
	if memory.config.ManagedCLIs["psql"].Contexts["prod"].Metadata["host"] != before {
		t.Fatal("validation mutated in-memory Context")
	}
	env.Interactive = func() bool { return true }
	cmd := NewRootCommandWithEnv("test", env)
	cmd.SetArgs([]string{"psql", "context", "configure", "prod"})
	cmd.SetIn(strings.NewReader("changed-database\n"))
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("incomplete wizard should cancel")
	}
	if _, ok := memory.config.ManagedCLIs["psql"].Contexts["prod"].Metadata["defaultDatabase"]; ok {
		t.Fatal("cancelled wizard saved partial values")
	}
}

func TestCorruptConnectorCanBeDiagnosedAndReplaced(t *testing.T) {
	env := newConnectorEnv(t)
	source := installTestConnector(t, env, "psql")
	if err := os.WriteFile(filepath.Join(env.Connectors.Dir, "psql.json"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := mustRun(t, env, "doctor"); !strings.Contains(out, "invalid installed Connector psql") {
		t.Fatal("doctor missed corrupt definition")
	}
	mustRun(t, env, "connector", "update", "psql", "--source", source)
	if _, err := env.Connectors.Get("psql"); err != nil {
		t.Fatal(err)
	}
}

func TestConfigureRevalidatesStoredSecretUnderUpdatedSchema(t *testing.T) {
	env := newConnectorEnv(t)
	source := installTestConnector(t, env, "mongosh")
	mustRun(t, env, "mongosh", "context", "configure", "prod", "--uri", "mongodb://localhost")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("schemes: [mongodb, mongodb+srv]"), []byte("schemes: [mongodb+srv]"), 1)
	if err = os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, env, "connector", "update", "mongosh")
	if _, err = runCommand(t, env, "mongosh", "context", "configure", "prod"); err == nil || !strings.Contains(err.Error(), "--uri") {
		t.Fatal("stored URI was not validated against changed schema", err)
	}
}
