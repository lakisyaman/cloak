package adapters

import (
	"fmt"
	"strings"

	"cloak/internal/contextstore"
)

type MongoSH struct{}

func (MongoSH) Name() string { return "mongosh" }

func (MongoSH) DetectExplicitConnectionInput(args []string, env []string) bool {
	for _, arg := range args {
		if isMongoURI(arg) {
			return true
		}
	}
	return hasFlagWithValue(args,
		"--host",
		"--port",
		"--username",
		"--password",
		"--authenticationDatabase",
		"--tlsCertificateKeyFile",
		"--tlsCAFile",
		"--tlsCertificateSelector",
	)
}

func (MongoSH) Activate(invocation Invocation, ctx contextstore.Context, secretValues map[string]string) (ActivatedInvocation, error) {
	uri := metadataString(ctx.Metadata, "uri")
	if uri == "" {
		return ActivatedInvocation{}, fmt.Errorf("missing metadata uri")
	}

	activatedURI, err := ApplyMongoDefaultDatabase(uri, metadataString(ctx.Metadata, "defaultDatabase"))
	if err != nil {
		return ActivatedInvocation{}, err
	}

	activatedArgs := []string{activatedURI}
	if username := metadataString(ctx.Metadata, "username"); username != "" {
		activatedArgs = append(activatedArgs, "--username", username)
	}
	if password := secretValues["password"]; password != "" {
		activatedArgs = append(activatedArgs, "--password", password)
	}
	if authSource := metadataString(ctx.Metadata, "authSource"); authSource != "" {
		activatedArgs = append(activatedArgs, "--authenticationDatabase", authSource)
	}
	activatedArgs = append(activatedArgs, invocation.Args...)

	return ActivatedInvocation{Args: activatedArgs, Env: append([]string(nil), invocation.Env...)}, nil
}

func isMongoURI(value string) bool {
	return strings.HasPrefix(value, "mongodb://") || strings.HasPrefix(value, "mongodb+srv://")
}
