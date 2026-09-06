# Repository Connector Registry

These files are downloaded individually by `cloak connector add @cloak/<command>`. They are not embedded in the binary or included in release archives. During development, install a local file with `cloak connector add ./registry/<command>.yaml`.

To contribute a Connector:

1. Add `<command>.yaml`, following the [version 1 schema](../docs/connector-schema.md). The filename and declared command must match.
2. Classify credential-bearing fields as secret, declare connection inputs as passthrough, and declare scope/transport inputs as field overrides.
3. Include parser hints needed to separate native option values from operands. Add meaningful behavioral tests in `internal/connectors`, using the YAML file. No CLI-specific Go implementation should be necessary.
4. Run `go test ./...`; use real-client integration tests when available. The test suite validates every YAML file in this directory.

The initial definitions preserve the supported connection, authentication, database, and transport behavior of the previous built-in implementation. psql database flags always use the agreed field override behavior. MongoDB's full URI is stored as Secret Material and supports both ordinary and credential-bearing URIs.

Parser references: [PostgreSQL psql documentation](https://www.postgresql.org/docs/current/app-psql.html) and [Redis CLI option parsing](https://github.com/redis/redis/blob/7.2/src/redis-cli.c). These definitions cover the declared inputs rather than validate the complete native CLI grammar; add selectors and parsing hints as native commands evolve.
