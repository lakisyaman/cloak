package adapters

import (
	"fmt"

	"cloak/internal/contextstore"
)

type RedisCLI struct{}

func (RedisCLI) Name() string { return "redis-cli" }

func (RedisCLI) DetectExplicitConnectionInput(args []string, env []string) bool {
	if envHasAny(env, "REDISCLI_AUTH", "REDISCLI_AUTH_USERNAME") {
		return true
	}
	return hasFlagWithValue(args, "-h", "--host", "-p", "--port", "-u", "--uri", "--user", "-a", "--pass")
}

func (RedisCLI) Activate(invocation Invocation, ctx contextstore.Context, secretValues map[string]string) (ActivatedInvocation, error) {
	host := metadataString(ctx.Metadata, "host")
	if host == "" {
		return ActivatedInvocation{}, fmt.Errorf("missing metadata host")
	}

	activatedArgs := []string{"-h", host}
	if port := metadataString(ctx.Metadata, "port"); port != "" {
		activatedArgs = append(activatedArgs, "-p", port)
	}
	if username := metadataString(ctx.Metadata, "username"); username != "" {
		activatedArgs = append(activatedArgs, "--user", username)
	}
	if password := secretValues["password"]; password != "" {
		activatedArgs = append(activatedArgs, "-a", password)
	}
	if defaultDatabase := metadataString(ctx.Metadata, "defaultDatabase"); defaultDatabase != "" && !redisHasDatabaseScope(invocation.Args) {
		activatedArgs = append(activatedArgs, "-n", defaultDatabase)
	}
	if metadataBool(ctx.Metadata, "tls") && !hasFlag(invocation.Args, "--tls") {
		activatedArgs = append(activatedArgs, "--tls")
	}
	activatedArgs = append(activatedArgs, invocation.Args...)

	return ActivatedInvocation{Args: activatedArgs, Env: append([]string(nil), invocation.Env...)}, nil
}

func redisHasDatabaseScope(args []string) bool {
	return hasFlagWithValue(args, "-n", "--db")
}
