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

Cloak installs PATH-based shims for supported CLIs.

Example:

```bash
mongosh cloak context add production --uri "mongodb+srv://cluster.example.com" --username app --password secret
mongosh cloak context switch production
mongosh
```

After a Context is selected, ordinary CLI invocations are activated with that Context:

```bash
psql cloak context switch production
psql analytics
```

Cloak preserves the original command shape while adding Context management.

## V1 scope

V1 targets macOS and Linux and supports these Managed CLIs:

- `mongosh`
- `psql`
- `redis-cli`

Cloak is implemented in Go with:

- Cobra for command parsing.
- `github.com/zalando/go-keyring` for OS secret storage.
- symlink shims pointing to the `cloak` binary.

## Installation

### Install script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/lakisyaman/cloak/main/install.sh | sh
```

This downloads the prebuilt binary for your OS/architecture from the latest
[GitHub release](https://github.com/lakisyaman/cloak/releases), verifies its checksum,
and installs it to `~/.local/bin` (override with `CLOAK_INSTALL_DIR`). Install a specific
version with `... | sh -s -- v0.1.0`.

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

Install a shim for a Managed CLI, then put the shim directory ahead of the real CLI on
your `PATH` — this is what makes the shim take effect:

```bash
cloak shim install psql
export PATH="$(cloak shim dir):$PATH"   # add to your shell profile to persist
```

Enroll a Context through the shim's `cloak` Control Prefix, select it, then use the CLI
normally:

```bash
psql cloak context add production --host db.example.com --username app --password secret --default-database analytics
psql cloak context switch production
psql                # connects to production with the enrolled credentials
```

Cloak prints a short stderr Invocation Notice for every call (activated, passed through,
or failed). Run `cloak doctor` anytime to check your installation, PATH ordering, and
state.

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
- [Architecture Decision Records](./docs/adr/)
- [Follow-ups](./docs/follow-ups.md)
- [Candidate Managed CLIs](./docs/managed-cli-candidates.md)
- [Agent instructions](./AGENT.md)

## Status

V1 is functionally complete. Context management and ephemeral Activation are implemented end to end for all three v1 Managed CLIs (`mongosh`, `psql`, `redis-cli`): a Context can be enrolled through a Shim's `cloak` Control Prefix, selected as the Active Context, and applied to ordinary invocations, while the standalone `cloak` command handles `shim install/uninstall/list`, `context list/switch/remove/show`, and `doctor` for installation and state diagnostics.

Coverage is real rather than scaffolding. The unit suite runs race-enabled, and the Testcontainers integration suite proves end-to-end Activation — installing a Shim, enrolling a Context, switching, and running the real client against a live Postgres, Redis, or MongoDB backend.

No tagged release has been cut yet. See [Known v1 storage limitation](#known-v1-storage-limitation) above for the intentional v1 constraints.
