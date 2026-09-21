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

The repository includes optional definitions for `mongosh`, `mysql`, `psql`, and `redis-cli`. More CLIs can be enabled by adding YAML definitions that use the shared runtime's capabilities. Cloak targets macOS and Linux and stores Secret Material in the OS keyring.

## Installation

Prebuilt releases support **macOS and Linux**, on **amd64** (Intel/AMD) and **arm64** (including Apple Silicon). Install the native CLI you want to manage separately; Cloak installs its Connector and Shim, not the native executable.

Storing and using Secret Material requires access to the OS keyring: macOS Keychain, or a Secret Service provider over D-Bus on Linux, such as GNOME Keyring. Headless Linux sessions and containers need that service configured when using secret fields. See the [keyring backend requirements](https://github.com/zalando/go-keyring/tree/v0.2.8#dependencies).

### Install script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/lakisyaman/cloak/main/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
cloak version
```

This downloads the prebuilt binary for your OS/architecture from the latest
[GitHub release](https://github.com/lakisyaman/cloak/releases), verifies its checksum,
and installs it to `~/.local/bin`. The installer requires `curl` or `wget`, `tar`, and
`sha256sum` or `shasum`. It prints PATH guidance but does not edit your shell configuration.

Install a specific release:

```bash
curl -fsSL https://raw.githubusercontent.com/lakisyaman/cloak/main/install.sh | sh -s -- v0.3.0
```

Or choose a different install directory; apply `CLOAK_INSTALL_DIR` to the `sh` process:

```bash
curl -fsSL https://raw.githubusercontent.com/lakisyaman/cloak/main/install.sh | CLOAK_INSTALL_DIR="$HOME/bin" sh
export PATH="$HOME/bin:$PATH"
```

### Prebuilt binaries

Download the `tar.gz` for your platform from the
[releases page](https://github.com/lakisyaman/cloak/releases), extract the `cloak`
binary, and put it on your `PATH`. Asset names use `darwin` for macOS, `linux` for Linux,
and `amd64` or `arm64` for the architecture. Each release publishes a `checksums.txt`;
compare your archive's SHA-256 hash with its entry before extracting. The install script
performs this check automatically.

### With Go

Requires Go 1.26 or newer, as specified in [go.mod](./go.mod):

```bash
go install github.com/lakisyaman/cloak/cmd/cloak@latest
```

Go installs the executable in `GOBIN`, or the `bin` directory under `GOPATH` when `GOBIN` is unset (normally `~/go/bin`). Add that directory to PATH and run `cloak version`. Cloak does not currently provide a Homebrew formula.

## Quickstart

This example uses `redis-cli` and a Context named `production`. After installing Cloak and the native Redis CLI, confirm that the native executable is available, then install its Connector:

```bash
redis-cli --version
cloak connector add @cloak/redis-cli
export PATH="$(cloak shim dir):$PATH"
```

For the default installer location, add these lines once to your shell startup file, such as `~/.zshrc` or the appropriate Bash startup file, so new terminals can find both Cloak and its Shims. Adjust the first line if you installed Cloak elsewhere:

```bash
export PATH="$HOME/.local/bin:$PATH"
export PATH="$(cloak shim dir):$PATH"
```

Configure the Context with your server's connection values. In a terminal, this starts a wizard with masked secret entry. Then select the Context and run the native CLI normally:

```bash
cloak redis-cli context configure production
cloak redis-cli context switch production
redis-cli PING
```

An activated invocation prints `cloak: activated redis-cli context production` to stderr before the native command's output. Use `type -a redis-cli` to check PATH resolution and `cloak doctor` for diagnostics if you do not see a Cloak notice.

Each Managed CLI has its own Contexts and one **user-global Active Context**. Switching affects subsequent invocations across directories and terminals. Configuration creates or edits a Context without selecting it; `switch` selects it. A new installation has no Connectors or Contexts.

To use PostgreSQL, MySQL, or MongoDB, install `@cloak/psql`, `@cloak/mysql`, or `@cloak/mongosh` and use the corresponding name in the Context commands. Browse the [repository registry](./registry/) for available definitions; `cloak connector list` shows only those installed for you.

## Everyday use

Inspect and switch Contexts:

```bash
cloak redis-cli context list
cloak redis-cli context current
cloak redis-cli context show production
cloak redis-cli context switch production
```

`show` redacts Secret Material. For scripts without interactive stdin/stderr, pass required values as flags. Existing values are retained unless replaced or cleared:

```bash
cloak redis-cli context configure production --host redis.example.com --port 6379 --default-database 2 --tls
cloak redis-cli context configure production --tls=false
cloak redis-cli context configure production --clear default-database
```

Use your own host in place of `redis.example.com`. In a terminal, omitted fields are still prompted. Secret field flags are available for non-interactive configuration, but their values can appear in shell history or process listings; the interactive wizard avoids echoing them.

Help is available at each command level. Configuration flags come from the installed Connector:

```bash
cloak --help
cloak connector --help
cloak redis-cli context --help
cloak redis-cli context configure --help
```

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

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| `cloak` is not found | Add the Cloak executable's install directory to PATH, then run `cloak version`. |
| A native CLI runs without any Cloak notice | Use `type -a redis-cli` (substitute your CLI) and `cloak doctor`. The Shim directory must precede the native executable on PATH. An alias or shell function may take precedence. |
| Cloak reports no Active Context | Run `cloak <cli> context list`, then `cloak <cli> context switch <name>`. With no selection, Cloak delegates using the native CLI's own connection behavior. |
| Cloak reports Explicit Connection Input | Caller-supplied connection flags or environment variables cause the entire Context to be skipped. Omit them when you intend to use the Active Context. Database and transport overrides can still compose with a Context. |
| Activation fails after a Connector update | Run the suggested `cloak <cli> context configure <name>` command to supply or repair the required values. Cloak does not delegate when Activation fails. |
| Secret storage or retrieval fails | Check that the OS keyring is available and unlocked; Linux needs a Secret Service provider and a usable D-Bus session. `cloak doctor` checks keyring availability. |
| `cloak init` finds no files in a script | Choose the target explicitly, for example `cloak init --agent claude` or `cloak init --global --agent codex`. |

## Updating Cloak

Rerun the install script, replace the prebuilt executable, or rerun `go install`, using the same installation method and directory. Check the result with `cloak version`. Shims point to the Cloak executable. To change its install directory, first run `cloak shim uninstall <cli>` with the old executable for each Managed CLI, then run `cloak shim install <cli>` with the new executable and check `cloak doctor`.

Binary upgrades preserve saved Contexts and installed Connector snapshots. Refresh a Connector explicitly with `cloak connector update <cli>` when you want its source's latest definition. Rerun `cloak init` in configured directories, or `cloak init --global` for user-level instructions, to refresh agent guidance after an upgrade.

## Upgrading from built-in Adapters

Add the definitions explicitly; upgrades do not download them automatically. Replace commands such as `psql cloak context switch production` with `cloak psql context switch production`. The former Control Prefix now passes to the native CLI.

Existing psql and Redis Context files and secret references remain usable. The MongoDB definition classifies its full URI as Secret Material, including embedded credentials. Reconfigure an older MongoDB Context with `cloak mongosh context configure <name> --uri ...` (or use the wizard) to store that URI in the keyring. Until then, Activation fails closed with configuration guidance.

## Development and testing

Build from a source checkout:

```bash
go build -o bin/cloak ./cmd/cloak
./bin/cloak --help
```

Use `./bin/cloak` in place of `cloak` in the usage examples. You can install a repository definition with `./bin/cloak connector add ./registry/redis-cli.yaml`. This uses the same user-global storage as a released binary; it is not an isolated test installation. Keep the built executable at its path while its Shims are installed, and follow the Shim steps above when switching executable locations. See [private storage](./docs/architecture.md#private-storage) for locations.

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
