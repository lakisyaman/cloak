# Dynamic Connector implementation plan

Status: implemented; see the schema and architecture for the current contract.

The accepted product decisions are in [architecture.md](./architecture.md), [ADR 0020](./adr/0020-load-connectors-on-demand-from-the-repository-registry.md), and [ADR 0021](./adr/0021-route-context-management-through-cloak.md). The implemented YAML schema is in [connector-schema.md](./connector-schema.md).

## 1. Complete the minimal schema against the three reference definitions

Write complete repository registry definitions for `redis-cli`, `psql`, and `mongosh`. Use those definitions to finish only the schema capabilities required for this first implementation:

- Context field types, required values, Secret Material classification, and configuration flag names.
- Native flag and environment input recognition, with fixed `passthrough` or `override` behavior.
- Positional input handling and the parsing information needed to distinguish native option values from operands, including psql short-flag bundles.
- Flag, environment-variable, and positional argument injection.
- Reusable URI path handling for MongoDB Default Database application.
- A schema version and validation that reports unsupported or malformed definitions before installation.

The psql `-d` and `--dbname` inputs always use `override`. Do not introduce conditional `onInput` expressions or executable hooks. The same generic machinery must run all three definitions without dispatching on their CLI names.

## 2. Build the generic runtime and internal Connector storage

Replace the compiled Adapter registry with loading and validation of definitions stored in Cloak's internal per-user directory. Interpret declarations to detect whole-Context bypass, validate a Context when Activation is attempted, and prepare argv/env for the existing process-replacement delegate.

The original source is used only for explicit registration or update. Registry references use `@cloak/<name>`; local sources use YAML file paths. Keep installed definitions out of the binary and release archive. Ordinary invocations use the internal copy and require no source access.

Preserve the existing no-Active-Context Pass-through Invocation, Invocation Notices, OS keyring storage, atomic private state writes, and PATH resolution that excludes Cloak itself. Unknown native options are forwarded unchanged.

## 3. Wire Connector and Context management

- `cloak connector add <source>` validates and stores the definition and installs its Shim, independently of Context values.
- Explicit update refreshes the internal definition from its source, independently of existing Context validity.
- `cloak connector remove <cli>` removes the definition and Shim while retaining Contexts and Secret Material.
- Route Context management through `cloak <cli> context ...` and retire Shim Control Prefix routing.
- Implement `context configure <name>` for explicit Context Configuration: a wizard interactively, supplied flags non-interactively, and actionable missing-value errors without a terminal.
- Report invalid Contexts during Activation with the exact configure command needed to fix them; never start a wizard from an ordinary Shim invocation.
- Adapt diagnostics and Context inspection/deletion for retained Contexts whose Connector has been removed.

Use the existing command behavior as the reference for routine choices not changed by the accepted design. Keep installation, configuration, and Context selection separate.

## 4. Prove the replacement, then remove the old implementations

Run the three existing live-backend scenarios through loaded YAML definitions and the generic runtime. Preserve their host, identity, Default Database, transport, and Secret Material behavior, accounting for the agreed psql `-d` simplification.

Verify the new boundaries directly:

- A clean installation has zero registered Connectors.
- Registry and local sources both become independent internal copies; source edits have no effect until explicit update.
- Add and update require no Context values and perform no Context Configuration.
- Remove retains Contexts and Secret Material.
- The two fixed input behaviors, boolean flag handling, and unknown-option forwarding behave as agreed.
- Invalid Context Activation fails before delegation and points to explicit configuration.
- Cloak-owned output does not reveal Secret Material.

Retire the dedicated Go Adapters and hard-coded enrollment branches once the generic path covers the reference definitions. Update README, installer guidance, architecture, and agent instructions to describe the implemented system rather than the earlier baseline.

Run `go test ./...` and the required Testcontainers integration suite. Complete the repository's build, vet, and race-enabled checks; report any integration skips due to Docker availability.

## Verification completed

- Default and race-enabled Go test suites; `go vet ./...`; normal builds.
- Live PostgreSQL, Redis, and MongoDB Activation via installed repository YAML definitions.
- Local and HTTP acquisition snapshots, explicit updates, independent Connector lifecycle, retained Contexts, schema errors, and a fourth declarative CLI.
- Context merging, required-value failures, secret rollback, MongoDB URI migration, and safe inspection.
- A real pseudo-terminal check of masked wizard entry and Ctrl-C cancellation with terminal mode restoration.

The official registry serves definitions from the repository's `main` branch. Local YAML imports also support development directly from a checkout.
