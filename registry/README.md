# Repository Connector Registry

These files are downloaded individually by `cloak connector add @cloak/<command>`. They are not embedded in the binary or included in release archives. During development, install a local file with `cloak connector add ./registry/<command>.yaml`.

To contribute a Connector:

1. Add `<command>.yaml`, following the [version 1 schema](../docs/connector-schema.md). The filename and declared command must match.
2. Classify credential-bearing fields as secret, declare connection inputs as passthrough, and declare scope/transport inputs as field overrides.
3. Include parser hints needed to separate native option values from operands. Add meaningful behavioral tests in `internal/connectors`, using the YAML file. No CLI-specific Go implementation should be necessary.
4. Run `go test ./...`; use real-client integration tests when available. The test suite validates every YAML file in this directory.

The initial definitions preserve the supported connection, authentication, database, and transport behavior of the previous built-in implementation. psql database flags and the database operand override the default database, except when their value is a libpq connection string, which skips the Context. MongoDB's full URI is stored as Secret Material and supports both ordinary and credential-bearing URIs.

The MySQL definition stores the password as Secret Material and injects it through
`MYSQL_PWD`, because the native `-p` option accepts no separate value. `--ssl-mode`
is a Transport Option, and the remaining `--ssl-*` and `--tls-*` options are parser
hints only, so they compose with an Active Context instead of skipping it.
`--login-path`, `--defaults-file`, and the socket options carry their own connection
identity, so they are passthrough inputs.
`--dns-srv-name` also selects a connection target and bypasses Activation.
`--no-defaults` and `--no-login-paths` pass through to preserve MySQL's required
option ordering; these invocations do not use the Active Context.

Parser references: [PostgreSQL psql documentation](https://www.postgresql.org/docs/current/app-psql.html), [MySQL client options](https://dev.mysql.com/doc/refman/8.4/en/mysql-command-options.html), and [Redis CLI option parsing](https://github.com/redis/redis/blob/7.2/src/redis-cli.c). These definitions cover the declared inputs rather than validate the complete native CLI grammar; add selectors and parsing hints as native commands evolve.
