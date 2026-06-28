# Cloak

Cloak gives third-party command-line tools a consistent way to use named connection contexts without exposing raw credentials during normal agent operation.

## Language

**Managed CLI**:
A third-party command-line tool whose connection target is selected by Cloak.
_Avoid_: tool, platform, integration

**Context**:
A named connection target for a Managed CLI, such as `production` or `staging`.
_Avoid_: profile, environment, account

**Active Context**:
The Context currently selected for a Managed CLI.
_Avoid_: current profile, selected environment

**Context Metadata**:
The non-secret data that describes a Context.
_Avoid_: config, settings

**Default Database**:
An optional database or logical namespace selected by default when a Context is activated.
_Avoid_: required database, context identity

**User-Global Active Context**:
An Active Context selection that applies across the current operating system user account.
_Avoid_: project context, local context

**Secret Material**:
The credential-bearing data needed to use a Context.
_Avoid_: password, token, credentials

**Context Enrollment**:
The act of creating a Context and establishing access to its Secret Material.
_Avoid_: login, setup, configure

**Authentication Flow**:
The Adapter-specific way Secret Material is obtained or refreshed for a Context.
_Avoid_: credential prompt, password entry

**Activation**:
The act of applying an Active Context to a single Real Command invocation.
_Avoid_: native config mutation, persistent login

**Pass-through Invocation**:
A Real Command invocation delegated by a Shim without applying a Context.
_Avoid_: unmanaged run, fallback mode

**Explicit Connection Input**:
Caller-supplied arguments or environment variables that identify connection state for a Real Command invocation.
_Avoid_: override, manual config

**Scope Selection**:
Caller-supplied arguments that select a database or logical namespace within an already selected connection target.
_Avoid_: explicit connection input, context override

**Transport Option**:
Caller-supplied transport security settings that can compose with an Active Context.
_Avoid_: explicit connection input, connection identity

**Invocation Notice**:
A short stderr message explaining whether Cloak applied, skipped, or could not apply a Context for an invocation.
_Avoid_: log, warning, banner

**Control Prefix**:
The Shim-reserved first argument, `cloak`, that routes a command to Cloak instead of the Real Command.
_Avoid_: context command, magic subcommand

**Cloak Command**:
The standalone `cloak` executable used for installation, diagnostics, and cross-CLI management.
_Avoid_: managed CLI, shim

**Shim**:
The user-facing command that makes a Managed CLI context-aware while preserving its normal command shape.
_Avoid_: wrapper, proxy, alias

**Real Command**:
The actual third-party executable that a Shim delegates to after Cloak handling.
_Avoid_: original binary, underlying CLI

**Adapter**:
The Managed CLI-specific knowledge Cloak uses to apply a Context to a Real Command.
_Avoid_: plugin, manifest, integration script

## Relationships

- A **Managed CLI** has zero or more **Contexts**.
- A **Context** name is unique within its **Managed CLI**, not globally across Cloak.
- A **Managed CLI** has at most one **User-Global Active Context** at a time.
- A **User-Global Active Context** is the only supported **Active Context** scope in the initial product.
- A **Context** is made of **Context Metadata** and references to **Secret Material**.
- **Secret Material** is not part of **Context Metadata**.
- **Secret Material** is not revealed by Context inspection commands.
- A **Context** may have a **Default Database**, but a **Default Database** is not required.
- **Context Enrollment** uses the **Authentication Flow** defined by the **Adapter** for the **Managed CLI**.
- **Activation** is ephemeral and does not mutate the Real Command's native configuration.
- A **Pass-through Invocation** happens when a Managed CLI has no Active Context selected.
- A **Pass-through Invocation** also happens when a Real Command invocation contains **Explicit Connection Input**.
- **Explicit Connection Input** wins over an **Active Context** for the whole invocation.
- **Scope Selection** does not prevent **Activation** by itself.
- A **Transport Option** does not prevent **Activation** by itself.
- **Activation** emits an **Invocation Notice**.
- A **Pass-through Invocation** emits an **Invocation Notice**.
- A **Control Prefix** invocation is handled by Cloak and is not delegated to the Real Command.
- The **Cloak Command** manages Shims and Contexts across Managed CLIs.
- A **Managed CLI** has exactly one **Adapter**.
- A **Shim** belongs to exactly one **Managed CLI**.
- A **Shim** delegates ordinary CLI usage to the **Real Command** for its **Managed CLI** under the selected **Active Context**.

## Example dialogue

> **Dev:** "If `mongosh` is a **Managed CLI**, what happens after I switch its **Active Context** to `production`?"
> **Domain expert:** "Normal `mongosh` commands should use the `production` **Context** without the agent seeing raw credentials."
>
> **Dev:** "Does **Context Enrollment** always mean prompting for a username and password?"
> **Domain expert:** "No — each **Adapter** defines its own **Authentication Flow**, which may be password-based, token-based, or browser-based SSO."
>
> **Dev:** "When the **Shim** runs `psql`, does Cloak edit `.pgpass` or PostgreSQL config?"
> **Domain expert:** "No — Cloak performs **Activation** only for that invocation and leaves native config alone."
>
> **Dev:** "What if no **Active Context** exists for `redis-cli`?"
> **Domain expert:** "The **Shim** performs a **Pass-through Invocation** and tells the user no Context was applied."
>
> **Dev:** "What if `psql` has an **Active Context**, but the caller passes `PGHOST=custom-host`?"
> **Domain expert:** "That is **Explicit Connection Input**, so Cloak performs a **Pass-through Invocation** instead of mixing caller input with the Active Context."
>
> **Dev:** "What if the caller only passes `psql analytics`?"
> **Domain expert:** "That is **Scope Selection**, so Cloak can still apply the Active Context for host and identity."
>
> **Dev:** "What if the caller passes `redis-cli --tls ping`?"
> **Domain expert:** "That is a **Transport Option**, so Cloak can still apply the Active Context for host and identity."
>
> **Dev:** "Should Cloak stay silent when it applies or skips **Activation**?"
> **Domain expert:** "No — Cloak should emit an **Invocation Notice** so humans and agents know whether a Context was applied."
>
> **Dev:** "How does `mongosh` know `cloak context list` is not meant for MongoDB?"
> **Domain expert:** "The **Shim** treats `cloak` as the **Control Prefix** and handles the command inside Cloak."
>
> **Dev:** "When should I use the standalone `cloak` command instead of `mongosh cloak ...`?"
> **Domain expert:** "Use the **Cloak Command** for installation, diagnostics, and cross-CLI management; use a **Shim** for commands scoped to one Managed CLI."

## Flagged ambiguities

- "profile", "environment", and "account" can all mean different things in other CLIs; in Cloak, the canonical term is **Context**.
- The same **Context** name in two different **Managed CLIs** does not imply shared Context Metadata or Secret Material.
- "credentials" often mixes secret and non-secret connection data; in Cloak, **Context Metadata** and **Secret Material** are separate concepts.
- "database" does not define a **Context** by itself; when present, it is an optional **Default Database**.
- "login", "setup", and "configure" are ambiguous; in Cloak, creating a Context is **Context Enrollment**, and obtaining access is an **Authentication Flow**.
- "local context" and "project context" are intentionally out of scope for the initial product; the selected **Active Context** is user-global.
- "adapter" does not mean an external plugin or manifest in the initial product; **Adapters** are built into Cloak at first.
- "context" alone is not reserved because upstream CLIs may use it; `cloak` is the **Control Prefix** for Cloak-specific commands.
- Transport security toggles such as TLS are **Transport Options**, not **Explicit Connection Input**, unless they also carry identity or credential material.
- `cloak` can mean the standalone **Cloak Command** or the in-shim **Control Prefix**; the distinction depends on whether it is the executable or the first argument to a Shim.
