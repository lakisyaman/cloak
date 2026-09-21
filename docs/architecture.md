# Cloak Architecture

Cloak adds named Contexts to third-party CLIs through PATH-based symlink Shims. It targets macOS and Linux, uses Go and Cobra, and replaces its process with the Real Command after preparing one invocation.

See [domain language](../CONTEXT.md), [Connector schema](./connector-schema.md), and [architecture decisions](./adr/).

## Connectors and acquisition

A fresh installation has zero Connectors. Definitions live in the repository's `registry/` directory, outside the binary and release archives. A CLI becomes available when a user explicitly installs its definition:

```sh
cloak connector add @cloak/redis-cli
# Or import a local definition:
cloak connector add ./redis-cli.yaml
cloak connector list
cloak connector update redis-cli
cloak connector update redis-cli --source ./replacement.yaml
cloak connector remove redis-cli
```

Only `@cloak/<cli>` is supported as a remote source. It downloads one YAML file from `https://raw.githubusercontent.com/lakisyaman/cloak/main/registry/<cli>.yaml`. Local `.yaml` and `.yml` paths are resolved to absolute paths. A missing local file never triggers a registry lookup. HTTP acquisition has a timeout and size limit. The downloaded definition's command must match the requested name.

Add validates the definition, copies it into Cloak's internal directory, and installs its Shim. It refuses an already installed command and points to update. The native CLI may be installed separately; PATH guidance explains how to enable the Shim. Update refreshes the saved source, or explicitly replaces that source with `--source`, and requires the command identity to stay unchanged. Update does not reinstall a removed Shim; `cloak shim install <cli>` does that.

Remove deletes the definition and Cloak-owned Shim while retaining Contexts, Secret Material, and Active Context selection. Reinstall can reuse those Contexts, subject to validation. Retained Contexts remain inspectable and removable through `cloak <cli> context ...`; configuration requires an installed Connector. Cloak refuses to overwrite unrelated files or symlinks in its Shim directory.

Installation and update do not create, configure, select, or validate Contexts. Even corrupt Context files do not prevent Connector lifecycle operations. Source acquisition happens only on add/update. Execution never contacts the registry or opens the original source.

## Declarative runtime

`internal/connectors` contains one YAML parser, input detector, Activation engine, and installed-definition store. There are no Managed CLI-specific Go implementations. `mongosh`, `mysql`, `psql`, and `redis-cli` are optional reference definitions.

Version 1 accepts scalar fields, required and secret annotations, configuration flag names, input selectors, injection bindings, and a URI default-path operation. Unknown schema properties, duplicate YAML keys, conflicting bindings, unsupported versions, and invalid command names fail validation. Embedded scripts, hooks, expressions, arbitrary templates, and value-dependent input behaviors are unsupported.

Each declared field input has one fixed behavior:

- `passthrough` skips the entire Context and delegates caller arguments and environment unchanged.
- `override` suppresses that field's Context value, preserving the caller's input while the remaining fields activate.

Any passthrough input wins over all overrides. Connector-level `passthrough` selectors cover inputs that have no stored Context field. There are no YAML roles or `takesValue` properties. Boolean field flags are switches; true injects a flag, false or absence omits it. Other scalar field flags consume and inject a value.

The parser recognizes declared flags, equals forms, environment variables with nonempty values, and positional operands. Definitions can enable short-option bundles and attached short values, describe native value-taking options unrelated to fields, and stop option scanning at the first operand. `--` ends option scanning; a lone `-` is treated as a stdin marker. Caller arguments are never rewritten. Unknown native flags are forwarded and do not themselves disable Activation. Definitions are responsible for declaring enough native syntax to avoid misidentifying option values as operands; an outdated definition may miss a new connection option until explicitly updated.

Injection is deterministic: the optional positional binding first, then flags in sorted field-name order, followed by all original arguments. Environment bindings replace the corresponding variable in the child environment. This ephemeral preparation does not modify native config files or the parent environment.

The psql definition always treats `-d` and `--dbname` as database overrides, including values containing a URI. PGDATABASE and the database operand also override the default database. PGSSLMODE composes with connection values. Redis's database and TLS flags override individual fields; connection flags skip the Context. MongoDB's URI operation preserves SRV and multi-host authorities, existing database paths, and queries while adding an escaped Default Database when the URI has no path.

## Context management

```sh
cloak <cli> context configure <name> [field flags]
cloak <cli> context list
cloak <cli> context current
cloak <cli> context show <name>
cloak <cli> context switch <name>
cloak <cli> context remove <name>
```

Context names are scoped to a Managed CLI. Each CLI has at most one user-global Active Context. Configuration creates or merges values without selecting a Context. Switch only changes selection. Remove deletes Context values and Secret Material and clears its selection if active.

The explicit configure command generates its flags from the installed definition. With interactive stdin and stderr, it prompts for fields not supplied as flags; Secret Material is read with terminal echo disabled. Blank answers keep existing values. Outside a terminal, supplied flags are merged with existing values and missing required fields cause an actionable error. `--clear <flag-name>` clears optional fields; boolean flags accept `--tls` or `--tls=false`. Wizard interruption or validation failure leaves stored data untouched.

Secret Material lives in the OS keyring through `go-keyring`. Configuration creates new secret references before atomically saving Context Metadata and rolls those new references back if persistence fails. Superseded references are deleted after the new Context commits. Failed cleanup emits a value-free warning. Context inspection never loads Secret Material, redacts declared secret fields even if old data stored them as metadata, and hides fields it cannot classify after Connector removal or schema changes.

MongoDB URIs are classified as Secret Material because they can contain embedded credentials. Their full value is stored in the keyring. Both separate username/password fields and credential-bearing URIs remain supported.

## Invocation flow

The basename of argv[0] identifies standalone Cloak versus a Shim. The standalone command owns all management. A Shim does not intercept a `cloak` argument; the former Control Prefix is retired.

1. Resolve the Real Command from PATH, skipping any executable that canonically resolves to Cloak.
2. Read the Active Context selection. A corrupt state file fails loudly.
3. With no selection, emit a stderr notice and delegate unchanged.
4. Load the installed Connector. Missing or invalid definitions fail closed when Activation is needed.
5. Detect caller inputs. Any passthrough match emits a notice and delegates unchanged without loading Context values or secrets.
6. Load the selected Context, then validate and load only declared fields not overridden by the caller. Missing required values, unavailable secrets, incompatible types, or changed secret-storage classifications fail before delegation and point to `cloak <cli> context configure <name>`.
7. Prepare argv/environment, emit an Activation notice, and invoke `syscall.Exec`.

Normal invocations never launch the Context Wizard. OS keyring errors and validation failures do not echo Context values. Native command output, warnings, exit status, and signal behavior remain the Real Command's responsibility.

## Private storage

The Cloak directory is `<os.UserConfigDir()>/cloak`: typically `~/Library/Application Support/cloak` on macOS and `$XDG_CONFIG_HOME/cloak` or `~/.config/cloak` on Linux.

```text
cloak/
  config.json              # version 1 Context Metadata and SecretRefs
  state.json               # version 1 Active Context selections
  connectors/<cli>.json    # version 1 source + exact acquired YAML snapshot
  shims/<cli>              # symlink to the Cloak executable
```

Each installed Connector is one private JSON record containing `version`, `source`, and `definition` (the original YAML string). Keeping provenance and content in one atomically replaced file avoids mixed versions on update. The original source remains unchanged. YAML is the public definition format; JSON here is a private storage envelope.

New directories use mode 0700 and data files use 0600. Writes use a temporary file, sync, and rename, with no lock files. Simultaneous writers can lose updates. Connector record and Shim changes cannot be one filesystem transaction; errors roll back completed steps where possible, and `doctor`, `connector update --source`, or `shim install` can diagnose or repair interrupted operations. Context metadata formats and existing SecretRefs remain version 1.

## Diagnostics and migration

`cloak doctor` reports installed Connectors, data readability, PATH ordering, Shim ownership, retained Contexts without an available Connector, dangling selections, secret references, and keyring availability. It is report-only and may access the keyring. `cloak shim dir/list/install/uninstall` remain available for troubleshooting; Shim installation requires an installed definition.

An upgrade does not automatically download definitions. Existing users explicitly add the appropriate registry or local definitions and switch to `cloak <cli> context ...`. Existing psql and Redis Contexts remain readable, including string-valued ports. Legacy MongoDB Contexts stored their URI in metadata; explicit `cloak mongosh context configure <name> --uri ...` moves the supplied URI into keyring storage. Until configured, these Contexts fail closed with that guidance. Adding or updating a definition never performs a silent Context migration.

## Agent instruction setup

`cloak init` discovers existing agent instruction Markdown files in the current directory and adds Cloak usage guidance. `cloak init --global` targets user-level agent instruction locations. These scopes control instruction placement; Active Context selection remains user-global.

Current-directory discovery checks `AGENTS.md`, `CLAUDE.md`, `.claude/CLAUDE.md`, `GEMINI.md`, and `.github/copilot-instructions.md`. It stays within these known locations under the current directory. Global discovery checks the agents' user instruction locations, normally `~/.codex/AGENTS.md`, `~/.claude/CLAUDE.md`, `~/.gemini/GEMINI.md`, and `~/.copilot/copilot-instructions.md`. `CODEX_HOME` and `CLAUDE_CONFIG_DIR` override the respective user configuration directories.

All discovered files in the selected scope receive the guidance. If no matching file exists, an interactive invocation asks which agent to initialize and creates its instruction file. `--agent <agent>` allows explicit targeting and creation without prompting; supported names are `codex`, `claude`, `gemini`, and `copilot`, comma-separated or repeated. Explicit targeting updates existing files for those agents, or creates their default file if missing. For Claude, the default local file is `CLAUDE.md`. A non-interactive invocation with no matches and no explicit agent fails with guidance to supply `--agent`.

The guidance occupies one section delimited by `<!-- cloak:start -->` and `<!-- cloak:end -->`. Repeating init refreshes that section while preserving surrounding instructions, avoids duplicate sections and unnecessary writes, and reports created, updated, or unchanged files. All targets are read and validated before writing; incomplete, reversed, or duplicate markers fail with repair guidance. Each changed file is replaced atomically, retaining existing permissions and using its newline style for the inserted section. New local files use mode 0644 and global files 0600. Symlinks are resolved without replacing the link, shared targets are deduplicated, and local setup rejects targets outside the current directory. Dangling links and non-regular files fail. Multiple file writes are not one transaction; an I/O failure may leave previously reported files updated.

The section explains how to discover installed Connectors with `cloak connector list`, inspect and switch Contexts with `cloak <cli> context ...`, invoke the native CLI through its Shim, and use `--help` for navigation. It also explains `cloak connector add @cloak/<cli>` for tasks that require enabling a new Managed CLI and links to the repository registry for available definitions. Connector installation remains separate from Context Configuration. The section contains generic instructions rather than a snapshot of installed Connectors or Context values.

`internal/agentsetup` owns discovery, section replacement, and file persistence. The [instruction text](../internal/agentsetup/instructions.md) is embedded in the binary and refreshed only by rerunning init. It contains no Connector definitions or user data. Init does not acquire Connectors, configure Contexts, or access Secret Material, and works when Context data is corrupt.

The design draws on [Vercel skills](https://github.com/vercel-labs/skills) for agent discovery and installation scopes, [OpenSkills](https://github.com/numman-ali/openskills/blob/main/src/utils/agents-md.ts) for preserving surrounding Markdown, and [skill-cli](https://github.com/VictorTomaili/skill-cli/blob/main/src/lib/agents-md.js) for refreshing a marked instruction section in detected agent locations.

## Boundaries and verification

Deferred: executable hooks, formats other than YAML, additional remote registries and authentication, project-local selections, one-shot Context selection, native Windows Shims, disk logs, shell completions, a stable public JSON output API, and automatic doctor repairs.

The tests load repository YAML, exercise a fourth custom CLI without adding runtime code, verify local and HTTP source snapshots and lifecycle separation, check parser/override/secret/error boundaries, and exercise configuration and rollback. Testcontainers scenarios install the definitions and run the real psql, mysql, redis-cli, and mongosh clients against live backends. Release builds ship the binary and ordinary project documents, with no embedded or archived registry definitions.
