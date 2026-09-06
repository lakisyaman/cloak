# Agent Instructions for Cloak

This project is architecture-first. Before making implementation changes, read:

1. `CONTEXT.md`
2. `docs/architecture.md`
3. all files in `docs/adr/`

The Connector architecture is recorded in ADR 0020 and ADR 0021. Use declarative YAML Connectors with a generic runtime; no CLI-specific Go implementations or bundled definitions. Context management uses `cloak <cli> context ...`; Shims have no Control Prefix.

## Project language

Use the glossary in `CONTEXT.md` exactly. In particular:

- Say **Managed CLI**, not generic "tool" when referring to a CLI Cloak manages.
- Say **Context**, not profile/environment/account.
- Say **Secret Material** for credential-bearing data.
- Say **Context Metadata** for non-secret context data.
- Say **Activation** for applying an Active Context to one invocation.
- Say **Pass-through Invocation** when Cloak delegates without Activation.
- Say **Explicit Connection Input** for caller-supplied host/identity/credential connection values.
- Say **Scope Selection** for caller-supplied database/logical namespace selection.
- Say **Transport Option** for caller-supplied transport security settings that compose with an Active Context.

If code or docs conflict with these terms, stop and clarify before continuing.

## Architectural constraints

Do not introduce these in v1 unless an ADR is added first:

- project-local Active Contexts
- arbitrary/unregistered CLIs
- executable Connector hooks or scripts
- one-shot context selection
- Windows-specific shim behavior
- disk logging
- shell completions
- public/stable JSON command output
- lock files for config/state writes
- automatic `doctor --fix`

## Implementation expectations

- Implement in Go.
- Use Cobra for CLI parsing.
- Use `github.com/zalando/go-keyring` for OS secret storage.
- Use symlink shims pointing to the `cloak` binary.
- Use OS-native per-user data directories.
- Store Context Metadata and state as private human-readable JSON with `version: 1`.
- Store Secret Material in the OS secret store, not in JSON config.
- Use atomic writes for config/state files.
- Do not add lock files in v1.
- Delegate Real Commands on macOS/Linux with process replacement after emitting any Invocation Notice.

## Safety rules

- Cloak-owned output must never print Secret Material.
- Context inspection commands must redact Secret Material.
- If an Active Context exists but Activation fails, fail closed and do not delegate.
- If no Active Context exists, perform a Pass-through Invocation with an Invocation Notice.
- If Explicit Connection Input is present, perform a Pass-through Invocation with an Invocation Notice.
- Scope Selection alone must not skip Activation.
- Transport Options alone must not skip Activation.
- Preserve native Real Command warnings and exit behavior.

## Test commands

Run the default test suite before finishing code changes:

```bash
go test ./...
```

Run Testcontainers-backed integration tests when changing Connector Activation, Context Configuration, or end-to-end CLI behavior:

```bash
go test -tags=integration ./test/integration -count=1 -v
```

Integration tests may skip if Docker/Testcontainers is unavailable.

## Connector definitions

Fresh installations contain zero Connectors. The repository registry initially contains `mongosh`, `psql`, and `redis-cli`; adding supported definitions is expected and does not require a new Go implementation. Registry downloads use `@cloak/<cli>`; local sources use YAML paths. Execution uses only the installed snapshot, refreshed explicitly.

Connector add/update must not configure or validate Context values. Invalid Contexts fail closed at Activation and point to explicit configuration. Connector removal preserves Contexts and secrets; Context removal is separate. Retained Contexts without a Connector remain manageable and are reported by doctor.

## Before adding an ADR

Add an ADR only when the decision is:

1. hard to reverse,
2. surprising without context, and
3. the result of a real trade-off.

Otherwise update `docs/architecture.md` or `CONTEXT.md` as appropriate.
