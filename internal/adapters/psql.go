package adapters

import (
	"fmt"
	"strings"

	"github.com/lakisyaman/cloak/internal/contextstore"
)

type PSQL struct{}

func (PSQL) Name() string { return "psql" }

func (PSQL) DetectExplicitConnectionInput(args []string, env []string) bool {
	if envHasAny(env, "PGHOST", "PGPORT", "PGUSER", "PGPASSWORD", "PGSERVICE", "PGPASSFILE") {
		return true
	}
	if hasFlagWithValue(args, "-h", "--host", "-p", "--port", "-U", "--username") {
		return true
	}
	if value, ok := flagValue(args, "-d", "--dbname"); ok && isPostgresConnectionURI(value) {
		return true
	}
	for _, arg := range args {
		if isPostgresConnectionURI(arg) {
			return true
		}
	}
	return false
}

func (PSQL) Activate(invocation Invocation, ctx contextstore.Context, secretValues map[string]string) (ActivatedInvocation, error) {
	host := metadataString(ctx.Metadata, "host")
	if host == "" {
		return ActivatedInvocation{}, fmt.Errorf("missing metadata host")
	}

	env := append([]string(nil), invocation.Env...)
	env = envSet(env, "PGHOST", host)
	env = envSet(env, "PGPORT", metadataString(ctx.Metadata, "port"))
	env = envSet(env, "PGUSER", metadataString(ctx.Metadata, "username"))
	env = envSet(env, "PGPASSWORD", secretValues["password"])
	env = envSetIfAbsent(env, "PGSSLMODE", metadataString(ctx.Metadata, "sslmode"))

	if !psqlHasDatabaseScope(invocation.Args, invocation.Env) {
		env = envSet(env, "PGDATABASE", metadataString(ctx.Metadata, "defaultDatabase"))
	}

	return ActivatedInvocation{Args: append([]string(nil), invocation.Args...), Env: env}, nil
}

func psqlHasDatabaseScope(args []string, env []string) bool {
	if envGet(env, "PGDATABASE") != "" {
		return true
	}
	if hasFlagWithValue(args, "-d", "--dbname") {
		return true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if psqlFlagConsumesNextValue(arg) && !strings.Contains(arg, "=") {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return true
	}
	return false
}

func psqlFlagConsumesNextValue(arg string) bool {
	switch arg {
	case "-c", "--command",
		"-f", "--file",
		"-v", "--set", "--variable",
		"-o", "--output",
		"-F", "--field-separator",
		"-P", "--pset",
		"-R", "--record-separator",
		"-T", "--table-attr",
		"-L", "--log-file":
		return true
	}
	for _, prefix := range []string{
		"--command=",
		"--file=",
		"--set=",
		"--variable=",
		"--output=",
		"--field-separator=",
		"--pset=",
		"--record-separator=",
		"--table-attr=",
		"--log-file=",
	} {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func isPostgresConnectionURI(value string) bool {
	return strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://")
}
