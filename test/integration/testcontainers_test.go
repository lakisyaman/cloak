//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

func TestManagedCLIBackendsStart(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	t.Run("postgres", func(t *testing.T) {
		pg := StartPostgres(t, ctx)
		output := execInContainer(t, ctx, pg.Container, []string{"pg_isready", "-U", pg.Username, "-d", pg.Database})
		if !strings.Contains(output, "accepting connections") {
			t.Fatalf("expected pg_isready to report accepting connections, got: %s", output)
		}
		t.Logf("postgres ready at %s:%s database=%s username=%s", pg.Host, pg.Port, pg.Database, pg.Username)
	})

	t.Run("mysql", func(t *testing.T) {
		mysql := StartMySQL(t, ctx)
		cmd := append([]string{"mysql"}, mysql.Args()...)
		cmd = append(cmd, "--skip-column-names", "--silent", "--execute", "select 1")
		// The client warns about a command-line password, so read the result line.
		selected := func(output string) bool {
			for _, line := range strings.Split(output, "\n") {
				if strings.TrimSpace(line) == "1" {
					return true
				}
			}
			return false
		}
		output := eventuallyExecInContainer(t, ctx, mysql.Container, cmd, selected)
		if !selected(output) {
			t.Fatalf("expected mysql select result 1, got: %s", output)
		}
		t.Logf("mysql ready at %s:%s database=%s username=%s", mysql.Host, mysql.Port, mysql.Database, mysql.Username)
	})

	t.Run("redis", func(t *testing.T) {
		redis := StartRedis(t, ctx)
		output := execInContainer(t, ctx, redis.Container, []string{"redis-cli", "-a", redis.Password, "ping"})
		if !strings.Contains(output, "PONG") {
			t.Fatalf("expected redis PONG, got: %s", output)
		}
		t.Logf("redis ready at %s:%s username=%s", redis.Host, redis.Port, redis.Username)
	})

	t.Run("mongo", func(t *testing.T) {
		mongo := StartMongo(t, ctx)
		output := eventuallyExecInContainer(t, ctx, mongo.Container, []string{
			"mongosh",
			"--quiet",
			"--username", mongo.Username,
			"--password", mongo.Password,
			"--authenticationDatabase", "admin",
			"--eval", "db.adminCommand({ ping: 1 }).ok",
		}, func(output string) bool {
			return strings.TrimSpace(output) == "1"
		})
		if strings.TrimSpace(output) != "1" {
			t.Fatalf("expected mongo ping result 1, got: %s", output)
		}
		t.Logf("mongo ready at %s username=%s uri=%s", mongo.URI(), mongo.Username, mongo.URI())
	})
}
