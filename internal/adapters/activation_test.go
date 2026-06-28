package adapters

import (
	"strings"
	"testing"

	"github.com/lakisyaman/cloak/internal/contextstore"
)

func TestPSQLActivationInjectsDefaultDatabaseForCommandFlag(t *testing.T) {
	activated, err := (PSQL{}).Activate(Invocation{Args: []string{"-c", "select 1"}}, contextstore.Context{
		Metadata: map[string]any{
			"host":            "db.example.com",
			"defaultDatabase": "defaultdb",
		},
	}, nil)
	if err != nil {
		t.Fatalf("activate psql: %v", err)
	}
	if !strings.Contains(strings.Join(activated.Env, "\x00"), "PGDATABASE=defaultdb") {
		t.Fatalf("expected PGDATABASE injection for -c command, got %#v", activated.Env)
	}
}

func TestPSQLActivationSuppressesDefaultDatabaseForTruePositionalDatabase(t *testing.T) {
	activated, err := (PSQL{}).Activate(Invocation{Args: []string{"analytics"}}, contextstore.Context{
		Metadata: map[string]any{
			"host":            "db.example.com",
			"defaultDatabase": "defaultdb",
		},
	}, nil)
	if err != nil {
		t.Fatalf("activate psql: %v", err)
	}
	if strings.Contains(strings.Join(activated.Env, "\x00"), "PGDATABASE=defaultdb") {
		t.Fatalf("did not expect PGDATABASE injection for positional database, got %#v", activated.Env)
	}
}

func TestPSQLActivationUsesEnvAndComposesTransportOption(t *testing.T) {
	adapter := PSQL{}
	if adapter.DetectExplicitConnectionInput(nil, []string{"PGSSLMODE=require"}) {
		t.Fatalf("PGSSLMODE should be a Transport Option, not Explicit Connection Input")
	}

	activated, err := adapter.Activate(Invocation{Args: []string{"analytics"}, Env: []string{"PGSSLMODE=require"}}, contextstore.Context{
		Metadata: map[string]any{
			"host":            "db.example.com",
			"port":            5432,
			"username":        "app",
			"defaultDatabase": "defaultdb",
			"sslmode":         "verify-full",
		},
	}, map[string]string{"password": "secret"})
	if err != nil {
		t.Fatalf("activate psql: %v", err)
	}

	joinedEnv := strings.Join(activated.Env, "\x00")
	for _, want := range []string{"PGHOST=db.example.com", "PGPORT=5432", "PGUSER=app", "PGPASSWORD=secret", "PGSSLMODE=require"} {
		if !strings.Contains(joinedEnv, want) {
			t.Fatalf("expected env %q in %#v", want, activated.Env)
		}
	}
	if strings.Contains(joinedEnv, "PGDATABASE=defaultdb") {
		t.Fatalf("positional database Scope Selection should suppress PGDATABASE injection, got %#v", activated.Env)
	}
}

func TestRedisActivationUsesArgsAndComposesTransportOption(t *testing.T) {
	adapter := RedisCLI{}
	if adapter.DetectExplicitConnectionInput([]string{"--tls", "ping"}, nil) {
		t.Fatalf("--tls should be a Transport Option, not Explicit Connection Input")
	}

	activated, err := adapter.Activate(Invocation{Args: []string{"--tls", "ping"}}, contextstore.Context{
		Metadata: map[string]any{
			"host":            "redis.example.com",
			"port":            6379,
			"username":        "default",
			"defaultDatabase": 2,
			"tls":             true,
		},
	}, map[string]string{"password": "secret"})
	if err != nil {
		t.Fatalf("activate redis-cli: %v", err)
	}

	want := []string{"-h", "redis.example.com", "-p", "6379", "--user", "default", "-a", "secret", "-n", "2", "--tls", "ping"}
	if strings.Join(activated.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("expected args %#v, got %#v", want, activated.Args)
	}
}

func TestMongoActivationUsesURIAndDefaultDatabase(t *testing.T) {
	activated, err := (MongoSH{}).Activate(Invocation{Args: []string{"--eval", "db.runCommand({ ping: 1 })"}}, contextstore.Context{
		Metadata: map[string]any{
			"uri":             "mongodb://localhost:27017?retryWrites=true",
			"username":        "app",
			"authSource":      "admin",
			"defaultDatabase": "analytics",
		},
	}, map[string]string{"password": "secret"})
	if err != nil {
		t.Fatalf("activate mongosh: %v", err)
	}

	want := []string{"mongodb://localhost:27017/analytics?retryWrites=true", "--username", "app", "--password", "secret", "--authenticationDatabase", "admin", "--eval", "db.runCommand({ ping: 1 })"}
	if strings.Join(activated.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("expected args %#v, got %#v", want, activated.Args)
	}
}
