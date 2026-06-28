# Cloak Architecture

Cloak gives third-party CLIs an AWS/kubectl-like Context layer through PATH-based shims. The core goal is to let humans and AI agents use credential-bearing CLIs without repeatedly passing raw connection details on every command.

See also:

- [Domain language](../CONTEXT.md)
- [Architecture decisions](./adr/)

## Goals

- Preserve the normal command shape of existing CLIs.
- Add named Context management to supported third-party CLIs.
- Store Secret Material outside normal config files.
- Apply an Active Context ephemerally for a single command invocation.
- Make behavior explicit through stderr Invocation Notices.
- Keep v1 small enough to implement quickly while validating multiple CLI shapes.

## Non-goals for v1

- Arbitrary CLI support without a built-in Adapter.
- Declarative or plugin-based Adapters.
- Project-local Active Contexts.
- One-shot context selection for a single command.
- Shell completions.
- Stable public JSON output for commands.
- Disk logging.
- Windows support.
- Automatic repair in `cloak doctor`.
- Automatic credential refresh for expiring Authentication Flows.

## Supported platforms

V1 officially supports macOS and Linux.

Windows compatibility should not be intentionally blocked, but Windows is not a v1 target because shim mechanics and credential storage differ materially.

## Core model

### Managed CLI

A Managed CLI is a third-party executable for which Cloak has a built-in Adapter. V1 supports only:

- `mongosh`
- `psql`
- `redis-cli`

Unknown Managed CLI names are rejected by user-facing commands. `cloak doctor` should report unknown config entries if they appear in state files.

### Context

A Context is a named connection target for one Managed CLI.

Context names are scoped per Managed CLI, not globally. For example, `psql` can have a `production` Context and `mongosh` can have a separate `production` Context with unrelated metadata and secrets.

Valid Context names match:

```text
^[A-Za-z0-9._-]+$
```

Names with spaces, slashes, quotes, or control characters are rejected.

### Active Context

V1 supports exactly one User-Global Active Context per Managed CLI.

There is no project-local Context selection in v1.

State shape:

```json
{
  "version": 1,
  "activeContexts": {
    "psql": "production",
    "mongosh": "production",
    "redis-cli": "local"
  }
}
```

### Context Metadata and Secret Material

Context Metadata is non-secret data stored in Cloak's private JSON config.

Secret Material is credential-bearing data stored in the OS secret store through `github.com/zalando/go-keyring`.

A Context may have zero Secret Material entries.

A Context may have an optional Default Database, but database selection is not required and does not define the Context identity.

Example config shape:

```json
{
  "version": 1,
  "managedClis": {
    "psql": {
      "contexts": {
        "production": {
          "metadata": {
            "host": "db.example.com",
            "port": 5432,
            "username": "app",
            "defaultDatabase": null,
            "sslmode": "require"
          },
          "secrets": {
            "password": {
              "store": "os",
              "service": "cloak",
              "user": "psql/production/password"
            }
          }
        }
      }
    }
  }
}
```

Internal JSON files are human-readable but not a stable public API in v1. Users and agents should mutate state through Cloak commands.

## Command surfaces

Cloak has two command surfaces:

1. The standalone `cloak` command.
2. Per-CLI shim control commands using the `cloak` Control Prefix.

### Standalone commands

```bash
cloak shim install <cli>
cloak shim uninstall <cli>
cloak shim list
cloak context list [cli]
cloak context current <cli>
cloak context switch <cli> <name>
cloak context remove <cli> <name>
cloak context show <cli> <name>
cloak doctor
```

### Shim-scoped commands

```bash
<cli> cloak context add <name> [adapter flags...]
<cli> cloak context list
<cli> cloak context current
<cli> cloak context switch <name>
<cli> cloak context remove <name>
<cli> cloak context show <name>
```

Examples:

```bash
mongosh cloak context add production --uri "mongodb+srv://cluster.example.com" --username app --password secret
mongosh cloak context switch production
mongosh

psql cloak context add production --host db.example.com --username app --password secret
psql cloak context switch production
psql analytics

redis-cli cloak context add local --host localhost --port 6379
redis-cli cloak context switch local
redis-cli ping
```

### Command output

V1 command output is human-readable text. Structured `--json` output is postponed.

`context show` uses key/value output and redacts Secret Material:

```text
name: production
host: db.example.com
port: 5432
default database: <none>
username: app
password: <stored>
sslmode: require
active: yes
```

Secret Material is never printed by Cloak notices, errors, or inspection commands.

## Shim model

Installed shims are symlinks to the Cloak binary.

Example:

```text
<user-data-dir>/shims/psql -> /usr/local/bin/cloak
<user-data-dir>/shims/mongosh -> /usr/local/bin/cloak
<user-data-dir>/shims/redis-cli -> /usr/local/bin/cloak
```

Cloak detects whether it is running as the standalone Cloak Command or as a Shim by inspecting the invocation name.

When invoked as a Shim:

- If the first argument is `cloak`, Cloak handles the command internally.
- Otherwise, Cloak treats the invocation as a Real Command invocation.

The generic `context` subcommand is not reserved because upstream CLIs may use it. Only the `cloak` Control Prefix is shim-reserved.

## Real Command resolution

A Shim resolves the Real Command at runtime:

1. Search `PATH` for executables matching the invocation name.
2. Canonicalize each candidate by resolving symlinks.
3. Canonicalize the Cloak binary path.
4. Exclude candidates that canonicalize to the Cloak binary.
5. Use the first remaining executable.
6. If none exists, error with install guidance.

This avoids stale absolute paths and prevents recursion through Cloak's own symlink shims.

`cloak shim install <cli>` verifies that:

- `<cli>` is registered in the built-in Adapter registry.
- A Real Command for `<cli>` exists in `PATH` before installing the symlink.

After install or uninstall, Cloak runs `cloak doctor`.

## Invocation flow

Normal Shim invocation flow:

```text
shim invoked as <cli>
  ↓
first arg is `cloak`?
  yes → handle shim-scoped Cloak command
  no  → resolve Real Command
          ↓
        Active Context exists for <cli>?
          no  → Invocation Notice + Pass-through Invocation
          yes → Explicit Connection Input present?
                  yes → Invocation Notice + Pass-through Invocation
                  no  → load Context + Secret Material
                         ↓
                       Activation succeeds?
                         yes → Invocation Notice + exec Real Command with activated argv/env
                         no  → fail closed; do not delegate
```

### Pass-through Invocation

Cloak performs a Pass-through Invocation when:

- no Active Context exists for the Managed CLI; or
- Explicit Connection Input is detected.

Pass-through delegates to the Real Command unchanged and emits a short stderr Invocation Notice.

Example:

```text
cloak: no active context for psql; running without activation
```

```text
cloak: explicit connection input detected for psql; running without activation
```

### Activation

Activation applies an Active Context to one Real Command invocation only. It does not mutate the Real Command's native config files.

Activation emits a short stderr Invocation Notice:

```text
cloak: activated psql context production
```

If an Active Context exists but Activation fails, Cloak fails closed and does not delegate:

```text
cloak: failed to activate psql context production: missing secret password
```

If Cloak state cannot be read because `config.json` or `state.json` is corrupt or has an unsupported version, the Shim fails loudly and does not delegate. This prevents a broken Cloak state from silently running commands against an unmanaged default target.

### Explicit Connection Input

Explicit Connection Input is caller-supplied argv/env that identifies connection target or credential-bearing connection state.

If present, it wins over the Active Context for the whole invocation. Cloak skips Activation entirely rather than mixing caller-provided connection state with Context-provided connection state.

Examples include host, URI, port, username, password, token, TLS identity, or connection-service selectors.

Transport-only settings such as `redis-cli --tls` or libpq `sslmode` are not Explicit Connection Input by themselves. They compose with an Active Context and override only the Context's transport metadata for that invocation.

### Scope Selection

Scope Selection is caller-supplied database or logical namespace selection within an already selected connection target.

Scope Selection does not skip Activation by itself.

Examples:

```bash
psql analytics
redis-cli -n 2
```

In both examples, Cloak can still apply host and identity from the Active Context while allowing the caller-selected database/logical namespace to win over the Context's Default Database.

## Delegation

On macOS/Linux, Cloak delegates normal Real Command invocations with Unix process replacement.

Cloak:

1. prepares argv/env;
2. emits any Invocation Notice to stderr;
3. calls exec to replace itself with the Real Command.

Cloak preserves Real Command stdio, exit behavior, and signal behavior as much as Unix process replacement naturally provides.

## Context command semantics

### Add

```bash
<cli> cloak context add <name> [adapter flags...]
```

- Creates a Context if it does not exist.
- If it already exists, overwrites it by full replacement.
- Does not merge omitted old fields.
- Does not automatically switch the Active Context.
- Stores Secret Material in the OS secret store.
- Allows Secret Material in CLI flags in v1; operators are responsible for any exposure during enrollment.

If a Context is replaced, old Secret Material is deleted on a best-effort basis. Cleanup failure should be reported, but the Context replacement should not be left half-complete.

Context replacement writes new Secret Material before writing the new config so the persisted Context never references secrets that were not stored. If the later config write fails, the OS secret store may contain orphaned new Secret Material while the old config remains active; `cloak doctor` can report missing referenced Secret Material, but it cannot currently discover unreferenced keyring entries.

### List

```bash
<cli> cloak context list
```

Shows Contexts for the Managed CLI and marks the Active Context:

```text
  staging
* production
```

### Current

```bash
<cli> cloak context current
```

Shows the Active Context for the Managed CLI, or reports that none is selected.

### Switch

```bash
<cli> cloak context switch <name>
```

Sets the User-Global Active Context for that Managed CLI.

### Remove

```bash
<cli> cloak context remove <name>
```

- Removes Context Metadata.
- Deletes associated Secret Material on a best-effort basis.
- If the removed Context is active, clears the Active Context for that Managed CLI.

### Show

```bash
<cli> cloak context show <name>
cloak context show <cli> <name>
```

Shows Context Metadata and whether Secret Material exists, but never prints Secret Material values.

## Doctor

`cloak doctor` is report-only in v1. It does not repair problems automatically.

Checks:

1. User data directory exists and is writable.
2. Shim directory exists and is writable.
3. Built-in supported Adapters are listed.
4. For each installed shim:
   - symlink exists;
   - symlink points to the Cloak binary;
   - Real Command exists;
   - Real Command resolution will not recurse into Cloak;
   - shim appears before the Real Command in `PATH`.
5. OS secret store is available. This check may perform a small Set/Get/Delete round trip in the OS keyring because `go-keyring` has no non-destructive ping.
6. Config/state files parse and have supported `version`.
7. Active Contexts point to existing Contexts.
8. Secret references exist for stored Contexts.

`cloak shim install` and `cloak shim uninstall` run `cloak doctor` afterward so installation feedback is centralized.

## Storage

Cloak stores files in OS-native per-user application directories.

Internal layout:

```text
config.json      # Context Metadata and secret references
state.json       # User-Global Active Context selections
shims/           # symlink shims
```

Cloak creates private data directories with `0700` permissions.

Writes are atomic and durable on Unix-like filesystems:

1. write new JSON to a temporary file in the same directory;
2. close/fsync the temporary file;
3. rename over the old file;
4. best-effort fsync the containing directory; a directory fsync failure after a successful rename is not treated as write failure.

V1 does not use lock files. Concurrent writes are not protected against lost updates, but atomic writes prevent half-written JSON files after crashes. `cloak doctor` can report detectable inconsistency afterward, such as dangling Active Context selections or missing Secret Material references, but it cannot reconstruct updates lost by concurrent writes. Locking should be revisited after v1 once real agent concurrency patterns are known.

## Secret storage

Cloak uses `github.com/zalando/go-keyring` for OS secret storage.

Secret keys are deterministic and scoped by Managed CLI, Context name, and secret field. The canonical `go-keyring` mapping is:

```text
service: cloak
user: <managed-cli>/<context-name>/<secret-field>
```

For example, the `psql` production password is stored with `service="cloak"` and `user="psql/production/password"`. JSON secret references store the same pair as `service` and `user`; there is no separate path-like `key` field.

Secret Material may be entered through command-line flags during Context Enrollment in v1. After enrollment, Secret Material is stored in the OS secret store and not repeated during normal CLI use.

Cloak does not distinguish between human-driven and agent-driven enrollment. If an operator gives Secret Material to an agent during enrollment, that exposure is the operator's responsibility.

## Adapter contract

Each built-in Adapter owns Managed CLI-specific behavior:

- CLI name.
- Supported Context Metadata fields.
- Supported Secret Material fields.
- Required fields.
- Context Enrollment flag parsing/validation.
- Secret storage references.
- Explicit Connection Input detection.
- Scope Selection detection.
- Transport-only option detection.
- Activation argv/env transformation.
- Redacted Context display fields.

Adapters are strict in v1. Unknown `context add` fields are errors.

A conceptual Go interface:

```go
type Adapter interface {
    Name() string
    AddCommandSpec() AddCommandSpec
    Validate(metadata Metadata, secrets SecretRefs) error
    DetectExplicitConnectionInput(argv []string, env Env) bool
    DetectScopeSelection(argv []string, env Env) ScopeSelection
    Activate(inv Invocation, ctx Context, secrets SecretValues) (ActivatedInvocation, error)
    Display(ctx Context) DisplayFields
}
```

The final implementation does not need this exact interface, but package boundaries should preserve these responsibilities.

## V1 Adapters

### `mongosh`

Context fields:

Metadata:

- `uri` required
- `username` optional
- `authSource` optional
- `defaultDatabase` optional

Secret Material:

- `password` optional

Enrollment example:

```bash
mongosh cloak context add production \
  --uri "mongodb+srv://cluster.example.com" \
  --username app \
  --password secret \
  --auth-source admin
```

Activation:

- Pass the MongoDB URI as the connection target.
- If `defaultDatabase` is set and the URI has no database path, apply the default database through URI parsing, preserving `mongodb://` vs `mongodb+srv://`, query options, credentials, and any existing non-empty path.
- Pass `--username` when metadata includes `username`.
- Pass `--password` when Secret Material includes `password`.
- Pass `--authenticationDatabase` when metadata includes `authSource`.
- Keep TLS and advanced MongoDB options in the URI for v1.

Explicit Connection Input examples:

- positional URI beginning with `mongodb://` or `mongodb+srv://`
- `--host`
- `--port`
- `--username`
- `--password`
- `--authenticationDatabase`
- TLS/certificate flags

A caller-provided MongoDB URI causes Pass-through Invocation.

### `psql`

Context fields:

Metadata:

- `host` required
- `port` optional
- `username` optional
- `defaultDatabase` optional
- `sslmode` optional

Secret Material:

- `password` optional

Enrollment example:

```bash
psql cloak context add production \
  --host db.example.com \
  --port 5432 \
  --username app \
  --password secret \
  --sslmode require
```

Activation uses libpq environment variables:

- `PGHOST`
- `PGPORT`
- `PGUSER`
- `PGPASSWORD`
- `PGDATABASE`
- `PGSSLMODE`

`PGDATABASE` is set only if:

- the Context has `defaultDatabase`; and
- the caller did not provide database Scope Selection; and
- the caller did not set `PGDATABASE`.

Scope Selection examples:

- positional database name: `psql analytics`
- `-d analytics`
- `--dbname analytics`, unless the value is a connection URI
- `PGDATABASE=analytics`

Explicit Connection Input examples:

- `-h`, `--host`
- `-p`, `--port`
- `-U`, `--username`
- connection URI in positional dbname or `--dbname`
- `PGHOST`
- `PGPORT`
- `PGUSER`
- `PGPASSWORD`
- `PGSERVICE`
- `PGPASSFILE`

Transport-only option examples:

- `PGSSLMODE`
- caller-supplied `sslmode` metadata in future psql-specific flags

If only database Scope Selection or a transport-only `sslmode`/`PGSSLMODE` override is present, Cloak still activates the Context for host and identity. Caller-supplied `sslmode`/`PGSSLMODE` suppresses only the Context's `PGSSLMODE` injection.

### `redis-cli`

Context fields:

Metadata:

- `host` required
- `port` optional
- `username` optional
- `defaultDatabase` optional Redis DB index
- `tls` optional boolean

Secret Material:

- `password` optional

Enrollment example:

```bash
redis-cli cloak context add production \
  --host redis.example.com \
  --port 6379 \
  --username default \
  --password secret \
  --tls
```

Activation uses argv flags:

- `-h <host>`
- `-p <port>`
- `--user <username>`
- `-a <password>`
- `-n <defaultDatabase>`
- `--tls`

`-n` is injected only if:

- the Context has `defaultDatabase`; and
- the caller did not provide database Scope Selection.

Scope Selection examples:

- `-n 2`
- `--db 2` if supported by the installed `redis-cli`

Explicit Connection Input examples:

- `-h`, `--host`
- `-p`, `--port`
- `-u`, `--uri`
- `--user`
- `-a`, `--pass`
- `REDISCLI_AUTH`
- `REDISCLI_AUTH_USERNAME`

Transport-only option examples:

- `--tls`

If only database Scope Selection or a transport-only `--tls` override is present, Cloak still activates the Context for host and identity. Caller-supplied `--tls` suppresses only duplicate Context `--tls` injection.

## Security posture

V1 improves normal CLI usage by avoiding repeated manual credential passing after Context Enrollment.

V1 does not guarantee secret-safe enrollment. It intentionally allows Secret Material in `context add` flags for simplicity and compatibility with existing CLI behavior.

Cloak never prints Secret Material in its own output, but Real Commands may expose secrets through their standard argv/env mechanisms during Activation. Cloak preserves native security warnings instead of suppressing them.

## Go implementation structure

```text
cmd/cloak/              # main package / entrypoint
internal/app/           # top-level command routing
internal/shim/          # shim detection + Real Command resolution
internal/contextstore/  # config/state JSON read/write
internal/secrets/       # OS secret store abstraction
internal/adapters/      # built-in adapters: mongosh, psql, redis-cli
internal/doctor/        # diagnostics
internal/notice/        # stderr Invocation Notices
```

Dependencies:

- Cobra for command parsing.
- `github.com/zalando/go-keyring` for OS secret storage.

## Testing focus

The default suite runs with:

```bash
go test ./...
```

Testcontainers-backed integration tests run with:

```bash
go test -tags=integration ./test/integration -count=1 -v
```

Integration tests should provide live PostgreSQL, Redis, and MongoDB backends for validating the v1 Managed CLIs.

V1 tests should prioritize:

- invocation mode detection: standalone vs shim;
- Real Command resolution avoiding symlink recursion;
- command routing for Control Prefix;
- context add/list/current/switch/remove/show semantics;
- config/state atomic write behavior;
- secret redaction in all Cloak-owned output;
- adapter Explicit Connection Input detection;
- adapter Scope Selection detection;
- activation argv/env construction;
- MongoDB Default Database URI parsing across standard vs SRV URIs, existing paths, and query params;
- end-to-end Activation against Testcontainers-backed PostgreSQL, Redis, and MongoDB services;
- doctor reports for broken shims, missing Real Commands, bad PATH ordering, invalid state, and missing secrets.
