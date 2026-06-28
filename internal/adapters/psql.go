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
		switch {
		case arg == "--":
			// Remaining tokens are operands; the first is the database name.
			return i+1 < len(args)
		case arg == "-":
			// Lone "-" is psql's stdin marker, not a database name.
			continue
		case strings.HasPrefix(arg, "-"):
			if psqlFlagConsumesNextValue(arg) {
				i++
			}
			continue
		default:
			// A bare token is a positional database name (Scope Selection).
			return true
		}
	}
	return false
}

// psqlFlagConsumesNextValue reports whether arg is a psql option whose value is
// the following argument. It understands long options (--command), long options
// with an attached value (--command=...), and short-option bundles where a
// value-taking flag is the last letter (e.g. -c, -tAc). A value-taking short
// flag that is not last carries its value attached (e.g. -cSELECT) and consumes
// no following argument.
func psqlFlagConsumesNextValue(arg string) bool {
	if strings.HasPrefix(arg, "--") {
		if strings.Contains(arg, "=") {
			return false
		}
		switch arg {
		case "--command", "--dbname", "--file", "--field-separator",
			"--host", "--log-file", "--output", "--port", "--pset",
			"--record-separator", "--set", "--table-attr", "--username",
			"--variable":
			return true
		}
		return false
	}
	body := strings.TrimPrefix(arg, "-")
	for i := 0; i < len(body); i++ {
		if strings.IndexByte(psqlValueShortFlags, body[i]) >= 0 {
			return i == len(body)-1
		}
	}
	return false
}

// psqlValueShortFlags are psql's value-taking short options.
const psqlValueShortFlags = "FLPRTUcdfhopv"

func isPostgresConnectionURI(value string) bool {
	return strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://")
}
