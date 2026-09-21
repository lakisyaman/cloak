//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	postgresImage = "postgres:16-alpine"
	redisImage    = "redis:7-alpine"
	mongoImage    = "mongo:7"
	mysqlImage    = "mysql:8.4"
)

type PostgresContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	Database  string
	Username  string
	Password  string
}

func (c PostgresContainer) Env() map[string]string {
	return map[string]string{
		"PGHOST":     c.Host,
		"PGPORT":     c.Port,
		"PGDATABASE": c.Database,
		"PGUSER":     c.Username,
		"PGPASSWORD": c.Password,
	}
}

type RedisContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	Username  string
	Password  string
}

func (c RedisContainer) Args() []string {
	args := []string{"-h", c.Host, "-p", c.Port}
	if c.Username != "" {
		args = append(args, "--user", c.Username)
	}
	if c.Password != "" {
		args = append(args, "-a", c.Password)
	}
	return args
}

type MongoContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	Username  string
	Password  string
}

func (c MongoContainer) URI() string {
	return fmt.Sprintf("mongodb://%s:%s", c.Host, c.Port)
}

type MySQLContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	Database  string
	Username  string
	Password  string
}

// Args uses the equals form so that redactedCommand hides the password.
func (c MySQLContainer) Args() []string {
	return []string{"--host=127.0.0.1", "--user=" + c.Username, "--password=" + c.Password, "--database=" + c.Database}
}

func StartPostgres(t *testing.T, ctx context.Context) PostgresContainer {
	t.Helper()

	const (
		database = "cloak_test"
		username = "cloak"
		password = "cloak_secret"
	)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image: postgresImage,
			Env: map[string]string{
				"POSTGRES_DB":       database,
				"POSTGRES_USER":     username,
				"POSTGRES_PASSWORD": password,
			},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("5432/tcp"),
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			).WithStartupTimeout(90 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("start postgres testcontainer: %v", err)
	}
	t.Cleanup(func() { terminateContainer(t, ctx, container) })

	host, port := endpoint(t, ctx, container, "5432/tcp")
	return PostgresContainer{Container: container, Host: host, Port: port, Database: database, Username: username, Password: password}
}

func StartRedis(t *testing.T, ctx context.Context) RedisContainer {
	t.Helper()

	const password = "cloak_secret"

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        redisImage,
			Cmd:          []string{"redis-server", "--requirepass", password},
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("6379/tcp"),
				wait.ForLog("Ready to accept connections"),
			).WithStartupTimeout(60 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("start redis testcontainer: %v", err)
	}
	t.Cleanup(func() { terminateContainer(t, ctx, container) })

	host, port := endpoint(t, ctx, container, "6379/tcp")
	return RedisContainer{Container: container, Host: host, Port: port, Username: "default", Password: password}
}

func StartMongo(t *testing.T, ctx context.Context) MongoContainer {
	t.Helper()

	const (
		username = "cloak"
		password = "cloak_secret"
	)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image: mongoImage,
			Env: map[string]string{
				"MONGO_INITDB_ROOT_USERNAME": username,
				"MONGO_INITDB_ROOT_PASSWORD": password,
			},
			ExposedPorts: []string{"27017/tcp"},
			WaitingFor:   wait.ForListeningPort("27017/tcp").WithStartupTimeout(90 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("start mongo testcontainer: %v", err)
	}
	t.Cleanup(func() { terminateContainer(t, ctx, container) })

	host, port := endpoint(t, ctx, container, "27017/tcp")
	return MongoContainer{Container: container, Host: host, Port: port, Username: username, Password: password}
}

func StartMySQL(t *testing.T, ctx context.Context) MySQLContainer {
	t.Helper()

	const (
		database = "cloak_test"
		username = "cloak"
		password = "cloak_secret"
	)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image: mysqlImage,
			Env: map[string]string{
				"MYSQL_ROOT_PASSWORD": "cloak_root_secret",
				"MYSQL_DATABASE":      database,
				"MYSQL_USER":          username,
				"MYSQL_PASSWORD":      password,
			},
			ExposedPorts: []string{"3306/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("3306/tcp"),
				// The entrypoint starts a temporary server first, so the
				// second occurrence marks the real server.
				wait.ForLog("mysqld: ready for connections").WithOccurrence(2),
			).WithStartupTimeout(180 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("start mysql testcontainer: %v", err)
	}
	t.Cleanup(func() { terminateContainer(t, ctx, container) })

	host, port := endpoint(t, ctx, container, "3306/tcp")
	return MySQLContainer{Container: container, Host: host, Port: port, Database: database, Username: username, Password: password}
}

func endpoint(t *testing.T, ctx context.Context, container testcontainers.Container, exposedPort string) (string, string) {
	t.Helper()

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("resolve container host: %v", err)
	}

	mappedPort, err := container.MappedPort(ctx, exposedPort)
	if err != nil {
		t.Fatalf("resolve mapped port %s: %v", exposedPort, err)
	}

	return host, mappedPort.Port()
}

func terminateContainer(t *testing.T, ctx context.Context, container testcontainers.Container) {
	t.Helper()
	if err := container.Terminate(ctx); err != nil {
		t.Errorf("terminate testcontainer: %v", err)
	}
}

func execInContainer(t *testing.T, ctx context.Context, container testcontainers.Container, cmd []string) string {
	t.Helper()

	safeCmd := redactedCommand(cmd)
	exitCode, output, err := tryExecInContainer(ctx, container, cmd)
	if err != nil {
		t.Fatalf("exec %q in testcontainer: %v", safeCmd, err)
	}

	if exitCode != 0 {
		t.Fatalf("exec %q failed with exit code %d: %s", safeCmd, exitCode, output)
	}

	return output
}

func eventuallyExecInContainer(t *testing.T, ctx context.Context, container testcontainers.Container, cmd []string, accept func(string) bool) string {
	t.Helper()

	safeCmd := redactedCommand(cmd)
	deadline := time.Now().Add(45 * time.Second)
	var lastExitCode int
	var lastOutput string
	var lastErr error

	for time.Now().Before(deadline) {
		lastExitCode, lastOutput, lastErr = tryExecInContainer(ctx, container, cmd)
		if lastErr == nil && lastExitCode == 0 && accept(lastOutput) {
			return lastOutput
		}
		time.Sleep(1 * time.Second)
	}

	if lastErr != nil {
		t.Fatalf("exec %q did not succeed before timeout: %v", safeCmd, lastErr)
	}
	t.Fatalf("exec %q did not return accepted output before timeout; last exit code %d: %s", safeCmd, lastExitCode, lastOutput)
	return ""
}

func tryExecInContainer(ctx context.Context, container testcontainers.Container, cmd []string) (int, string, error) {
	exitCode, outputReader, err := container.Exec(ctx, cmd, tcexec.Multiplexed())
	if err != nil {
		return exitCode, "", err
	}

	output, err := io.ReadAll(outputReader)
	if err != nil {
		return exitCode, "", err
	}

	return exitCode, string(output), nil
}

func redactedCommand(cmd []string) []string {
	redacted := append([]string(nil), cmd...)
	redactNext := false

	for i, arg := range redacted {
		if redactNext {
			redacted[i] = "<redacted>"
			redactNext = false
			continue
		}

		switch arg {
		case "-a", "--pass", "--password", "--requirepass":
			redactNext = true
			continue
		}

		for _, prefix := range []string{"--pass=", "--password=", "--requirepass="} {
			if strings.HasPrefix(arg, prefix) {
				redacted[i] = prefix + "<redacted>"
			}
		}
	}

	return redacted
}
