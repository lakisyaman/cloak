package connectors

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
)

type testSecrets map[secrets.SecretRef]string

func (s testSecrets) Get(ref secrets.SecretRef) (string, error) {
	v, ok := s[ref]
	if !ok {
		return "", errors.New("secret backend unavailable")
	}
	return v, nil
}
func (s testSecrets) Set(ref secrets.SecretRef, v string) error { s[ref] = v; return nil }
func (s testSecrets) Delete(ref secrets.SecretRef) error        { delete(s, ref); return nil }

func registryDefinition(t *testing.T, name string) *Definition {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "registry", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	def, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func TestRegistryDefinitions(t *testing.T) {
	files, err := filepath.Glob("../../registry/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 {
		t.Fatal("reference definitions missing")
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yaml")
		t.Run(name, func(t *testing.T) {
			if registryDefinition(t, name).Command != name {
				t.Fatal("filename and command differ")
			}
		})
	}
}

func TestPSQLScopeAndNativeParsing(t *testing.T) {
	d := registryDefinition(t, "psql")
	ctx := contextstore.Context{Metadata: map[string]any{"host": "db.internal", "port": "5432", "defaultDatabase": "saved", "sslmode": "verify-full"}}
	for _, tc := range []struct {
		args     []string
		database bool
		pass     bool
	}{
		{[]string{"-c", "select 1"}, true, false},
		{[]string{"-Atc", "select 1"}, true, false},
		{[]string{"-tAcselect 1"}, true, false},
		{[]string{"--command=select 1"}, true, false},
		{[]string{"-tAf", "queries.sql"}, true, false},
		{[]string{"--command", "postgres://sql-payload"}, true, false},
		{[]string{"-d", "analytics"}, false, false},
		{[]string{"-dpostgres://db/analytics"}, false, false},
		{[]string{"--dbname=postgres://db/analytics"}, false, false},
		{[]string{"analytics", "-Atc", "select 1"}, false, false},
		{[]string{"--", "analytics"}, false, false},
		{[]string{"-", "-c", "select 1"}, true, false},
		{[]string{"-hother"}, false, true},
		{[]string{"-Ath", "other"}, false, true},
		{[]string{"--host=other", "-d", "analytics"}, false, true},
		{[]string{"analytics", "otheruser"}, false, true},
		{[]string{"--future-native-flag"}, true, false},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			inv := Invocation{Args: tc.args, Env: []string{"PGSSLMODE=require"}}
			if d.Detect(inv).Passthrough != tc.pass {
				t.Fatal("wrong passthrough decision")
			}
			result, err := d.Activate(inv, ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Args, inv.Args) {
				t.Fatalf("changed native args: %v", result.Args)
			}
			if (envGet(result.Env, "PGDATABASE") == "saved") != tc.database {
				t.Fatalf("wrong database injection: %v", result.Env)
			}
			if envGet(result.Env, "PGSSLMODE") != "require" {
				t.Fatal("transport override lost")
			}
			if !tc.pass && envGet(result.Env, "PGHOST") != "db.internal" {
				t.Fatal("host not activated")
			}
		})
	}
	for _, entry := range []string{"PGHOST=caller", "PGPORT=123", "PGUSER=caller", "PGPASSWORD=caller", "PGSERVICE=caller", "PGPASSFILE=/tmp/pass"} {
		if !d.Detect(Invocation{Env: []string{entry}}).Passthrough {
			t.Fatalf("missed %s", entry)
		}
	}
}

func TestMySQLScopeSecretsAndNativeParsing(t *testing.T) {
	d := registryDefinition(t, "mysql")
	ref := secrets.NewRef("mysql", "prod", "password")
	ctx := contextstore.Context{
		Metadata: map[string]any{"host": "db.internal", "port": 3306, "username": "reporter", "defaultDatabase": "saved", "sslMode": "VERIFY_IDENTITY"},
		Secrets:  map[string]secrets.SecretRef{"password": ref},
	}
	store := testSecrets{ref: "s3cr3t"}
	for _, tc := range []struct {
		args     []string
		database bool
		pass     bool
	}{
		{[]string{"-e", "select 1"}, true, false},
		{[]string{"-Ne", "select 1"}, true, false},
		{[]string{"-eselect 1"}, true, false},
		{[]string{"--execute=select 1"}, true, false},
		{[]string{"--init-command", "set names utf8mb4"}, true, false},
		{[]string{"--init-command-add", "SET @cloak_probe=1", "-e", "select database()"}, true, false},
		{[]string{"--init-command-add=SET @cloak_probe=1", "-e", "select database()"}, true, false},
		{[]string{"-D", "analytics"}, false, false},
		{[]string{"-Danalytics"}, false, false},
		{[]string{"--database=analytics"}, false, false},
		{[]string{"analytics", "-e", "select 1"}, false, false},
		{[]string{"--", "analytics"}, false, false},
		{[]string{"--ssl-mode", "DISABLED"}, true, false},
		{[]string{"--ssl-ca", "/etc/ssl/ca.pem"}, true, false},
		{[]string{"--future-native-flag"}, true, false},
		{[]string{"-h", "other"}, false, true},
		{[]string{"-hother"}, false, true},
		{[]string{"-Nu", "root"}, false, true},
		{[]string{"--user=root", "-D", "analytics"}, false, true},
		{[]string{"-p"}, false, true},
		{[]string{"-ps3cr3t"}, false, true},
		{[]string{"-P", "3307"}, false, true},
		{[]string{"-S", "/tmp/mysql.sock"}, false, true},
		{[]string{"--login-path=prod"}, false, true},
		{[]string{"--no-defaults", "-e", "select 1"}, false, true},
		{[]string{"--no-login-paths", "-e", "select 1"}, false, true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			inv := Invocation{Args: tc.args}
			if d.Detect(inv).Passthrough != tc.pass {
				t.Fatal("wrong passthrough decision")
			}
			result, err := d.Activate(inv, ctx, store)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(result.Args, " ")
			if tc.pass {
				if !reflect.DeepEqual(result.Args, inv.Args) || envGet(result.Env, "MYSQL_PWD") != "" {
					t.Fatal("explicit connection input must reach the Real Command unchanged")
				}
				return
			}
			if !reflect.DeepEqual(result.Args[len(result.Args)-len(tc.args):], tc.args) {
				t.Fatalf("changed native args: %v", result.Args)
			}
			if envGet(result.Env, "MYSQL_HOST") != "db.internal" || envGet(result.Env, "MYSQL_TCP_PORT") != "3306" {
				t.Fatal("server not activated")
			}
			// The native -p option takes no separate value, so the password
			// reaches the client through the environment only.
			if envGet(result.Env, "MYSQL_PWD") != "s3cr3t" || strings.Contains(joined, "s3cr3t") {
				t.Fatalf("wrong password injection: %v", result.Args)
			}
			if strings.Count(joined, "--user reporter") != 1 {
				t.Fatalf("wrong identity injection: %v", result.Args)
			}
			if strings.Count(joined, "--ssl-mode") != 1 {
				t.Fatalf("duplicate transport option: %v", result.Args)
			}
			if (strings.Count(joined, "--database saved") == 1) != tc.database {
				t.Fatalf("wrong database injection: %v", result.Args)
			}
		})
	}
	result, err := d.Activate(Invocation{}, ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--database", "saved", "--ssl-mode", "VERIFY_IDENTITY", "--user", "reporter"}
	if !reflect.DeepEqual(result.Args, want) {
		t.Fatalf("got %v; want %v", result.Args, want)
	}
	for _, entry := range []string{"MYSQL_HOST=caller", "MYSQL_TCP_PORT=3307", "MYSQL_PWD=caller", "MYSQL_UNIX_PORT=/tmp/mysql.sock", "MYSQL_HOME=/tmp/mysql", "MYSQL_GROUP_SUFFIX=_prod", "MYSQL_TEST_LOGIN_FILE=/tmp/.mylogin.cnf"} {
		if !d.Detect(Invocation{Env: []string{entry}}).Passthrough {
			t.Fatalf("missed %s", entry)
		}
	}
	// A caller credential must skip the secret store, which is unavailable here.
	if _, err := d.Activate(Invocation{Env: []string{"MYSQL_PWD=caller"}}, ctx, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMySQLDNSSRVConnectionPassesThrough(t *testing.T) {
	d := registryDefinition(t, "mysql")
	ctx := contextstore.Context{
		Metadata: map[string]any{"host": "db.internal", "username": "reporter", "defaultDatabase": "saved"},
		Secrets: map[string]secrets.SecretRef{
			"password": secrets.NewRef("mysql", "prod", "password"),
		},
	}
	for _, args := range [][]string{
		{"--dns-srv-name=_mysql._tcp.other.example", "-e", "select 1"},
		{"--dns-srv-name", "_mysql._tcp.other.example", "-e", "select 1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			inv := Invocation{Args: args, Env: []string{"PATH=/usr/bin", "TERM=xterm"}}
			if !d.Detect(inv).Passthrough {
				t.Error("explicit DNS SRV target must bypass context activation")
			}
			// Passthrough must not read the saved password, even when the
			// secret store is unavailable.
			result, err := d.Activate(inv, ctx, nil)
			if err != nil {
				t.Fatalf("explicit DNS SRV target accessed context secrets: %v", err)
			}
			if !reflect.DeepEqual(result, inv) {
				t.Fatalf("passthrough changed the invocation: got %#v; want %#v", result, inv)
			}
		})
	}
}

func TestMySQLNoDefaultsRemainsFirst(t *testing.T) {
	d := registryDefinition(t, "mysql")
	ctx := contextstore.Context{
		Metadata: map[string]any{"host": "db.internal", "username": "reporter", "defaultDatabase": "saved"},
	}
	inv := Invocation{Args: []string{"--no-defaults", "-e", "select 1"}}
	result, err := d.Activate(inv, ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// MySQL processes this option before ordinary options. Either
	// passthrough or activation must preserve that required position.
	if len(result.Args) == 0 || result.Args[0] != "--no-defaults" {
		t.Fatalf("--no-defaults must remain the first argument: got %q", result.Args)
	}
	if len(result.Args) < 3 || !reflect.DeepEqual(result.Args[len(result.Args)-2:], inv.Args[1:]) {
		t.Fatalf("changed the native query arguments: got %q", result.Args)
	}
}

func TestRedisActivationAndCommandPayload(t *testing.T) {
	d := registryDefinition(t, "redis-cli")
	ref := secrets.NewRef("redis-cli", "prod", "password")
	ctx := contextstore.Context{Metadata: map[string]any{"host": "redis.internal", "port": 6379, "username": "default", "defaultDatabase": "2", "tls": true}, Secrets: map[string]secrets.SecretRef{"password": ref}}
	inv := Invocation{Args: []string{"--tls", "-n", "3", "SET", "key", "--host"}}
	result, err := d.Activate(inv, ctx, testSecrets{ref: "s3cr3t"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Detect(inv).Passthrough {
		t.Fatal("command payload interpreted as an option")
	}
	if strings.Count(strings.Join(result.Args, " "), "--tls") != 1 {
		t.Fatal("duplicate TLS flag")
	}
	if strings.Count(strings.Join(result.Args, " "), "-n") != 1 {
		t.Fatal("duplicate scope flag")
	}
	want := []string{"-h", "redis.internal", "-a", "s3cr3t", "-p", "6379", "--user", "default", "--tls", "-n", "3", "SET", "key", "--host"}
	if !reflect.DeepEqual(result.Args, want) {
		t.Fatalf("got %v", result.Args)
	}
	for _, args := range [][]string{{"-h", "caller", "-n", "3"}, {"--uri=redis://caller"}, {"-s", "/tmp/socket"}} {
		if !d.Detect(Invocation{Args: args}).Passthrough {
			t.Fatalf("missed explicit input: %v", args)
		}
	}
	ctx.Metadata["tls"] = false
	result, err = d.Activate(Invocation{Args: []string{"PING"}}, ctx, testSecrets{ref: "s3cr3t"})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range result.Args {
		if arg == "--tls" {
			t.Fatal("false boolean injected")
		}
	}
}

func TestMongoURIAndArgumentInjection(t *testing.T) {
	d := registryDefinition(t, "mongosh")
	for _, tc := range []struct{ uri, want string }{
		{"mongodb://localhost:27017", "mongodb://localhost:27017/app%20data"},
		{"mongodb://user:pass@localhost?authSource=admin", "mongodb://user:pass@localhost/app%20data?authSource=admin"},
		{"mongodb+srv://cluster.example.com?retryWrites=true&w=majority", "mongodb+srv://cluster.example.com/app%20data?retryWrites=true&w=majority"},
		{"mongodb://h1:27017,h2:27017/?replicaSet=rs0", "mongodb://h1:27017,h2:27017/app%20data?replicaSet=rs0"},
		{"mongodb://host/existing?authSource=admin", "mongodb://host/existing?authSource=admin"},
	} {
		ref := secrets.NewRef("mongosh", "prod", "uri")
		result, err := d.Activate(Invocation{Args: []string{"--quiet", "--eval", "db.getName()"}}, contextstore.Context{Metadata: map[string]any{"defaultDatabase": "app data", "username": "app", "authSource": "admin"}, Secrets: map[string]secrets.SecretRef{"uri": ref}}, testSecrets{ref: tc.uri})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{tc.want, "--authenticationDatabase", "admin", "--username", "app", "--quiet", "--eval", "db.getName()"}
		if !reflect.DeepEqual(result.Args, want) {
			t.Fatalf("got %v; want %v", result.Args, want)
		}
	}
	if d.Detect(Invocation{Args: []string{"--eval", "mongodb://data"}}).Passthrough {
		t.Fatal("eval payload mistaken for URI")
	}
	if !d.Detect(Invocation{Args: []string{"--quiet", "mongodb://caller"}}).Passthrough {
		t.Fatal("missed caller URI")
	}
}

const customYAML = `version: 1
command: acme-cli
parsing: {shortOptions: true}
fields:
  endpoint:
    type: string
    required: true
    onInput: passthrough
    input: {flags: ["--endpoint"]}
    inject: {env: ACME_ENDPOINT}
  token:
    type: string
    required: true
    secret: true
    onInput: override
    input: {flags: ["--token"], env: [ACME_TOKEN]}
    inject: {env: ACME_TOKEN}
  insecure:
    type: boolean
    onInput: override
    input: {flags: ["-k", "--insecure"]}
    inject: {flag: "--insecure"}
`

func TestFourthConnectorNeedsNoGoImplementation(t *testing.T) {
	d, err := Parse([]byte(customYAML))
	if err != nil {
		t.Fatal(err)
	}
	ctx := contextstore.Context{Metadata: map[string]any{"endpoint": "https://internal", "insecure": true}, Secrets: map[string]secrets.SecretRef{"token": secrets.NewRef("acme-cli", "prod", "token")}}
	// Caller override avoids reading an unavailable stored secret.
	inv := Invocation{Args: []string{"--token", "caller", "-k", "--new-native-option"}}
	result, err := d.Activate(inv, ctx, testSecrets{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Args, inv.Args) || envGet(result.Env, "ACME_ENDPOINT") != "https://internal" || envGet(result.Env, "ACME_TOKEN") != "" {
		t.Fatalf("wrong activation: %v", result)
	}
	if _, err = d.Activate(Invocation{}, ctx, testSecrets{}); err == nil {
		t.Fatal("missing secret should fail")
	}
	inv.Args = append([]string{"--endpoint", "https://caller"}, inv.Args...)
	result, err = d.Activate(inv, contextstore.Context{}, nil)
	if err != nil || !reflect.DeepEqual(result, inv) {
		t.Fatalf("passthrough must win: %v %v", result, err)
	}
}

func TestSchemaRejectsInvalidDefinitions(t *testing.T) {
	for _, data := range []string{
		strings.Replace(customYAML, "version: 1", "version: 99", 1),
		strings.Replace(customYAML, "command: acme-cli", "command: ../cloak", 1),
		strings.Replace(customYAML, "command: acme-cli", "command: connector", 1),
		strings.Replace(customYAML, "command: acme-cli", "command: init", 1),
		strings.Replace(customYAML, "onInput: override", "onInput: conditional", 1),
		strings.Replace(customYAML, "type: boolean", "type: executable", 1),
		strings.Replace(customYAML, "inject: {flag: \"--insecure\"}", "inject: {flag: \"--insecure\", takesValue: false}", 1),
		customYAML + "hooks: {before: echo}\n",
		customYAML + "---\nversion: 1\n",
		strings.Replace(customYAML, "required: true", "required: true\n    required: false", 1),
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Fatalf("accepted invalid YAML: %s", data)
		}
	}
}

func TestValidationErrorsDoNotEchoValues(t *testing.T) {
	d := registryDefinition(t, "psql")
	_, err := d.Activate(Invocation{}, contextstore.Context{Metadata: map[string]any{"host": "saved", "port": "secret-value"}}, nil)
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("bad error: %v", err)
	}
}
