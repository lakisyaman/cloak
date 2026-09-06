# Cloak

Cloak gives third-party command-line tools an AWS/kubectl-like Context layer through PATH-based shims.

The goal is to let humans and AI agents use credential-bearing CLIs without repeatedly passing raw connection details on every command.

## Problem

Today, agents usually connect to third-party software through either:

- MCP tools, which can be cumbersome, memory-heavy, and context-bloating.
- Native CLIs, which models generally understand well.

Some CLIs are already agent-friendly because they manage profiles or contexts internally, such as `aws`, `kubectl`, and `doctl`.

Other CLIs require connection details or credentials repeatedly, which is awkward and risky when an AI agent is operating the shell.

## Approach

Cloak installs PATH-based Shims using declarative YAML Connectors. The binary ships with zero Connector definitions. Install a definition from the repository registry or a local file, configure and select a Context, then use the native CLI normally.

```bash
cloak connector add @cloak/psql
export PATH="$(cloak shim dir):$PATH"
cloak psql context configure production
cloak psql context switch production
psql analytics
```

The repository includes optional definitions for `mongosh`, `psql`, and `redis-cli`. More CLIs can be enabled by adding YAML definitions that use the shared runtime's capabilities. Cloak targets macOS and Linux, uses Cobra for management commands, and stores Secret Material in the OS keyring.

## Installation

### Install script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/lakisyaman/cloak/main/install.sh | sh
```

This downloads the prebuilt binary for your OS/architecture from the latest
[GitHub release](https://github.com/lakisyaman/cloak/releases), verifies its checksum,
and installs it to `~/.local/bin` (override with `CLOAK_INSTALL_DIR`). Install a specific
version with `... | sh -s -- v0.3.0`.

### Prebuilt binaries

Download the `tar.gz` for your platform from the
[releases page](https://github.com/lakisyaman/cloak/releases), extract the `cloak`
binary, and put it on your `PATH`. Each release publishes a `checksums.txt`.

### With Go

```bash
go install github.com/lakisyaman/cloak/cmd/cloak@latest
```

### Homebrew

Planned for a future release.

## Quickstart

Install the native CLI, then acquire its Connector and put Cloak's Shim directory ahead of it on PATH:

```bash
cloak connector add @cloak/redis-cli
export PATH="$(cloak shim dir):$PATH"   # persist in your shell profile
```

For development, build from a source checkout and use its local registry definitions:

```bash
go build -o bin/cloak ./cmd/cloak
./bin/cloak connector add ./registry/redis-cli.yaml
export PATH="$(./bin/cloak shim dir):$PATH"
```

Configure values explicitly. In a terminal, this starts a wizard with masked secret entry:

```bash
cloak redis-cli context configure production
cloak redis-cli context switch production
redis-cli PING
```

For scripts, pass the required fields; optional fields can also be supplied. Existing values are retained unless replaced or cleared:

```bash
cloak redis-cli context configure production --host redis.example.com --port 6379 --default-database 2 --tls
cloak redis-cli context configure production --tls=false
cloak redis-cli context configure production --clear default-database
cloak redis-cli context list
cloak redis-cli context current
cloak redis-cli context show production
```

Secret field flags are available for non-interactive configuration, but their command-line values can appear in shell history or process listings. The interactive wizard avoids echoing them. Configuration does not select a Context; use `switch` separately.

## Set up coding agents

Add Cloak usage guidance to your agent's instruction files:

```bash
cloak init                       # Update recognized files in the current directory
cloak init --global              # Update existing user-level instruction files
cloak init --agent claude        # Target Claude; create its file if missing
cloak init --global --agent codex,gemini
```

`init` discovers and updates all existing files in the selected scope:

| Agent (`--agent`) | Current directory | User-level (`--global`) |
| --- | --- | --- |
| Codex (`codex`) | `AGENTS.md` | `$CODEX_HOME/AGENTS.md`, default `~/.codex/AGENTS.md` |
| Claude Code (`claude`) | `CLAUDE.md`, `.claude/CLAUDE.md` | `$CLAUDE_CONFIG_DIR/CLAUDE.md`, default `~/.claude/CLAUDE.md` |
| Gemini CLI (`gemini`) | `GEMINI.md` | `~/.gemini/GEMINI.md` |
| GitHub Copilot CLI (`copilot`) | `.github/copilot-instructions.md` | `~/.copilot/copilot-instructions.md` |

If no recognized file exists, an interactive terminal prompts for an agent. In scripts, pass `--agent`; it accepts comma-separated names or repeated flags. Explicit targeting updates that agent's existing files, or creates the first listed path when missing. Current-directory discovery stays at the listed paths, without searching parent directories or nested projects.

Cloak maintains one marked section, preserving surrounding instructions and existing file permissions. Rerun `init` after upgrading Cloak to refresh the guidance; unchanged files are left untouched. Incomplete or duplicate Cloak markers cause an error before any files are written. Symlinks are preserved and shared targets are updated once; current-directory setup rejects links that resolve outside that directory.

The guidance teaches Connector discovery and registry installation, Context inspection and switching, native CLI usage through Shims, configuration, and `--help` navigation. The local/global choice controls where instructions are written; Active Context selection remains user-global. Init does not install Connectors or configure Contexts.

## Connector lifecycle

```bash
cloak connector add ./my-client.yaml
cloak connector list
cloak connector update redis-cli
cloak connector update redis-cli --source @cloak/redis-cli
cloak connector remove redis-cli
```

Both local and registry definitions are copied into Cloak's internal directory. Execution uses that copy without reading the source or accessing the network. Source edits take effect only after an explicit update. Remote sources currently support only `@cloak/<cli>`; local sources must be `.yaml` or `.yml` files.

Connector add/update never requires Context values or changes saved Contexts. If an updated definition needs missing values, the next Activation fails with `cloak <cli> context configure <name>` guidance. Normal CLI invocations never open a wizard.

Connector removal deletes its definition and Shim but keeps Contexts and Secret Material. Reinstall to reuse them, or explicitly delete a Context with `cloak <cli> context remove <name>`.

Known connection inputs skip the whole Context; database and transport inputs can override individual fields. For example, `psql -d analytics` keeps the configured connection and selects the caller's database. Unknown native options are forwarded. See the [schema](./docs/connector-schema.md) for the declarative behavior and parser limitations.

Cloak prints a short stderr Invocation Notice when applying, skipping, or failing Activation. Run `cloak doctor` to check installation, PATH ordering, and state. `cloak shim dir/list/install/uninstall` remain available for troubleshooting.

## Upgrading from built-in Adapters

Add the definitions explicitly; upgrades do not download them automatically. Replace commands such as `psql cloak context switch production` with `cloak psql context switch production`. The former Control Prefix now passes to the native CLI.

Existing psql and Redis Context files and secret references remain usable. The MongoDB definition classifies its full URI as Secret Material, including embedded credentials. Reconfigure an older MongoDB Context with `cloak mongosh context configure <name> --uri ...` (or use the wizard) to store that URI in the keyring. Until then, Activation fails closed with configuration guidance.

## Testing

Run the default test suite:

```bash
go test ./...
```

Run the Testcontainers-backed integration smoke tests:

```bash
go test -tags=integration ./test/integration -count=1 -v
```

Integration tests require a healthy Docker/Testcontainers provider. If Docker is unavailable, the smoke tests skip with a clear message.

Run the opt-in OS keyring smoke test:

```bash
CLOAK_TEST_KEYRING=1 go test ./internal/secrets -run TestKeyringStoreRoundTrip -count=1 -v
```

See [integration test docs](./test/integration/README.md).

## Known v1 storage limitation

V1 uses atomic JSON writes but no lock files. This prevents half-written files after crashes, but concurrent writes can still lose updates. `cloak doctor` reports detectable inconsistency such as dangling Active Context selections, but it cannot reconstruct lost writes.

If `config.json` or `state.json` is corrupt, shims fail loudly instead of passing through silently.

`cloak doctor` may touch the OS keyring while checking Secret Material references and keyring availability.

## Versioning

Cloak follows [Semantic Versioning](https://semver.org). Releases are git tags of the
form `vX.Y.Z`. While on `0.x`, the CLI surface and on-disk formats may still change
between minor versions. `cloak --version` (or `cloak version`) reports the version,
commit, and build date.

## License

MIT. See [LICENSE](./LICENSE).

## Documentation

Read these before implementation:

- [Domain language](./CONTEXT.md)
- [Architecture](./docs/architecture.md)
- [Connector schema](./docs/connector-schema.md)
- [Repository registry](./registry/)
- [Architecture Decision Records](./docs/adr/)
- [Follow-ups](./docs/follow-ups.md)
- [Candidate Managed CLIs](./docs/managed-cli-candidates.md)
- [Agent instructions](./AGENT.md)

## Status

Dynamic YAML Connectors, on-demand acquisition, explicit Context Configuration, generic Activation, and local/global agent instruction setup are implemented. Registry files are ordinary repository files; publishing changes there enables on-demand downloads without bundling definitions in a binary release. The test suite includes shared-engine coverage and Testcontainers scenarios for PostgreSQL, Redis, and MongoDB.
