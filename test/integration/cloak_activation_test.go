//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lakisyaman/cloak/internal/app"
	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
	"github.com/lakisyaman/cloak/internal/shim"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

func TestCloakActivatesRealManagedCLIsAgainstLiveBackends(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	t.Run("psql", func(t *testing.T) {
		pg := StartPostgres(t, ctx)
		env, shimPath := newCloakIntegrationEnv(t, "psql")
		runContextCommand(t, env, "psql", "context", "configure", "production",
			"--host", "localhost",
			"--port", "5432",
			"--username", pg.Username,
			"--password", pg.Password,
			"--default-database", pg.Database,
		)
		runContextCommand(t, env, "psql", "context", "switch", "production")

		delegate := &containerDelegate{t: t, ctx: ctx, container: pg.Container}
		var stderr bytes.Buffer
		if err := app.ExecuteInvocationWithOptions(app.InvocationOptions{
			Paths:    env.Paths,
			Version:  "test",
			Argv0:    shimPath,
			Args:     []string{"-Atc", "select current_database()"},
			Stderr:   &stderr,
			Resolver: env.Resolver,
			Delegate: delegate,
			Store:    env.Store,
			Secrets:  env.Secrets,
			Env:      []string{},
		}); err != nil {
			t.Fatalf("activate psql against live backend: %v", err)
		}
		if got := strings.TrimSpace(delegate.output); got != pg.Database {
			t.Fatalf("expected psql to use default database %q, got output %q", pg.Database, delegate.output)
		}
		if !strings.Contains(stderr.String(), "cloak: activated psql context production") {
			t.Fatalf("expected activation notice, got %q", stderr.String())
		}
	})

	t.Run("mysql", func(t *testing.T) {
		mysql := StartMySQL(t, ctx)
		env, shimPath := newCloakIntegrationEnv(t, "mysql")
		runContextCommand(t, env, "mysql", "context", "configure", "production",
			"--host", "127.0.0.1",
			"--port", "3306",
			"--username", mysql.Username,
			"--password", mysql.Password,
			"--default-database", mysql.Database,
			"--ssl-mode", "PREFERRED",
		)
		runContextCommand(t, env, "mysql", "context", "switch", "production")

		delegate := &containerDelegate{t: t, ctx: ctx, container: mysql.Container}
		var stderr bytes.Buffer
		if err := app.ExecuteInvocationWithOptions(app.InvocationOptions{
			Paths:    env.Paths,
			Version:  "test",
			Argv0:    shimPath,
			Args:     []string{"--skip-column-names", "--silent", "--execute", "select database(), current_user()"},
			Stderr:   &stderr,
			Resolver: env.Resolver,
			Delegate: delegate,
			Store:    env.Store,
			Secrets:  env.Secrets,
			Env:      []string{},
		}); err != nil {
			t.Fatalf("activate mysql against live backend: %v", err)
		}
		if !strings.Contains(delegate.output, mysql.Database) || !strings.Contains(delegate.output, mysql.Username+"@") {
			t.Fatalf("expected mysql to use context database and identity, got output %q", delegate.output)
		}
		// The native -p option takes no separate value, so the password must
		// reach the client through MYSQL_PWD instead of the command line.
		if strings.Contains(delegate.output, mysql.Password) {
			t.Fatal("password reached the Real Command output")
		}
		if !strings.Contains(stderr.String(), "cloak: activated mysql context production") {
			t.Fatalf("expected activation notice, got %q", stderr.String())
		}
	})

	t.Run("redis-cli", func(t *testing.T) {
		redis := StartRedis(t, ctx)
		env, shimPath := newCloakIntegrationEnv(t, "redis-cli")
		runContextCommand(t, env, "redis-cli", "context", "configure", "production",
			"--host", "localhost",
			"--port", "6379",
			"--username", redis.Username,
			"--password", redis.Password,
			"--default-database", "2",
		)
		runContextCommand(t, env, "redis-cli", "context", "switch", "production")

		delegate := &containerDelegate{t: t, ctx: ctx, container: redis.Container}
		var stderr bytes.Buffer
		if err := app.ExecuteInvocationWithOptions(app.InvocationOptions{
			Paths:    env.Paths,
			Version:  "test",
			Argv0:    shimPath,
			Args:     []string{"SET", "cloak:e2e", "ok"},
			Stderr:   &stderr,
			Resolver: env.Resolver,
			Delegate: delegate,
			Store:    env.Store,
			Secrets:  env.Secrets,
			Env:      []string{},
		}); err != nil {
			t.Fatalf("activate redis-cli against live backend: %v", err)
		}
		if !strings.Contains(delegate.output, "OK") {
			t.Fatalf("expected redis SET OK, got %q", delegate.output)
		}

		value := execInContainer(t, ctx, redis.Container, []string{"redis-cli", "-a", redis.Password, "-n", "2", "GET", "cloak:e2e"})
		if !strings.Contains(value, "ok") {
			t.Fatalf("expected key in default redis db 2, got %q", value)
		}
	})

	t.Run("mongosh", func(t *testing.T) {
		mongo := StartMongo(t, ctx)
		env, shimPath := newCloakIntegrationEnv(t, "mongosh")
		runContextCommand(t, env, "mongosh", "context", "configure", "production",
			"--uri", "mongodb://localhost:27017?authSource=admin",
			"--username", mongo.Username,
			"--password", mongo.Password,
			"--default-database", "cloak_test",
		)
		runContextCommand(t, env, "mongosh", "context", "switch", "production")

		delegate := &containerDelegate{t: t, ctx: ctx, container: mongo.Container}
		var stderr bytes.Buffer
		var lastErr error
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			lastErr = app.ExecuteInvocationWithOptions(app.InvocationOptions{
				Paths:    env.Paths,
				Version:  "test",
				Argv0:    shimPath,
				Args:     []string{"--quiet", "--eval", "db.getName()"},
				Stderr:   &stderr,
				Resolver: env.Resolver,
				Delegate: delegate,
				Store:    env.Store,
				Secrets:  env.Secrets,
				Env:      []string{},
			})
			if lastErr == nil && strings.TrimSpace(delegate.output) == "cloak_test" {
				return
			}
			time.Sleep(1 * time.Second)
		}
		if lastErr != nil {
			t.Fatalf("activate mongosh against live backend: %v", lastErr)
		}
		t.Fatalf("expected mongosh default database cloak_test, got output %q", delegate.output)
	})
}

type integrationFileStore struct {
	configPath string
	statePath  string
}

func (store integrationFileStore) ReadConfig() (contextstore.Config, error) {
	return contextstore.ReadConfig(store.configPath)
}

func (store integrationFileStore) ReadState() (contextstore.State, error) {
	return contextstore.ReadState(store.statePath)
}

func (store integrationFileStore) WriteConfig(config contextstore.Config) error {
	return contextstore.WriteConfig(store.configPath, config)
}

func (store integrationFileStore) WriteState(state contextstore.State) error {
	return contextstore.WriteState(store.statePath, state)
}

type integrationSecretStore struct {
	values map[string]string
}

func (store integrationSecretStore) Set(ref secrets.SecretRef, value string) error {
	store.values[ref.Service+"/"+ref.User] = value
	return nil
}

func (store integrationSecretStore) Get(ref secrets.SecretRef) (string, error) {
	value, ok := store.values[ref.Service+"/"+ref.User]
	if !ok {
		return "", fmt.Errorf("secret not found")
	}
	return value, nil
}

func (store integrationSecretStore) Delete(ref secrets.SecretRef) error {
	delete(store.values, ref.Service+"/"+ref.User)
	return nil
}

type containerDelegate struct {
	t         *testing.T
	ctx       context.Context
	container testcontainers.Container
	output    string
}

func (delegate *containerDelegate) Exec(realPath string, argv []string, env []string) error {
	delegate.t.Helper()
	cmd := append([]string{filepath.Base(realPath)}, argv[1:]...)
	exitCode, outputReader, err := delegate.container.Exec(delegate.ctx, cmd, tcexec.Multiplexed(), tcexec.WithEnv(env))
	if err != nil {
		return err
	}
	output, err := io.ReadAll(outputReader)
	if err != nil {
		return err
	}
	delegate.output = string(output)
	if exitCode != 0 {
		return fmt.Errorf("exec %q failed with exit code %d: %s", redactedCommand(cmd), exitCode, delegate.output)
	}
	return nil
}

func newCloakIntegrationEnv(t *testing.T, managedCLI string) (app.CommandEnv, string) {
	t.Helper()
	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shims")
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("create real command dir: %v", err)
	}
	cloakBinary := filepath.Join(dir, "cloak")
	writeIntegrationExecutable(t, cloakBinary)
	writeIntegrationExecutable(t, filepath.Join(realDir, managedCLI))

	paths := contextstore.Paths{
		Dir:        dir,
		ConfigFile: filepath.Join(dir, "config.json"),
		StateFile:  filepath.Join(dir, "state.json"),
		ShimDir:    shimDir,
	}
	store := integrationFileStore{configPath: paths.ConfigFile, statePath: paths.StateFile}
	env := app.CommandEnv{
		Paths:           paths,
		Connectors:      &connectors.Store{Dir: filepath.Join(dir, "connectors")},
		Store:           store,
		Secrets:         integrationSecretStore{values: map[string]string{}},
		Resolver:        shim.RealCommandResolver{PathEnv: shimDir + string(os.PathListSeparator) + realDir, CloakBinaryPath: cloakBinary},
		PathEnv:         shimDir + string(os.PathListSeparator) + realDir,
		CloakBinaryPath: cloakBinary,
	}

	var output bytes.Buffer
	cmd := app.NewRootCommandWithEnv("test", env)
	cmd.SetArgs([]string{"connector", "add", filepath.Join("..", "..", "registry", managedCLI+".yaml")})
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install Connector for %s: %v", managedCLI, err)
	}
	return env, filepath.Join(shimDir, managedCLI)
}

func runContextCommand(t *testing.T, env app.CommandEnv, managedCLI string, args ...string) {
	t.Helper()
	var output bytes.Buffer
	cmd := app.NewRootCommandWithEnv("test", env)
	cmd.SetArgs(append([]string{managedCLI}, args...))
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%s cloak %s: %v", managedCLI, strings.Join(args, " "), err)
	}
}

func writeIntegrationExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}
