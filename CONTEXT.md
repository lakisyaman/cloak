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
_Avoid_: login, setup

**Context Configuration**:
The act of setting or changing a Context's Metadata and Secret Material.
_Avoid_: connector installation, connector update

**Context Wizard**:
An interactive flow for supplying the values a Context requires.
_Avoid_: connector editor, setup wizard

**Authentication Flow**:
The Connector-specific way Secret Material is obtained or refreshed for a Context.
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

**Cloak Command**:
The command interface for managing Connectors, Shims, and Contexts, diagnosing their state, and setting up agent guidance for using Cloak.
_Avoid_: managed CLI, shim

**Shim**:
The user-facing command that makes a Managed CLI context-aware while preserving its normal command shape.
_Avoid_: wrapper, proxy, alias

**Real Command**:
The actual third-party executable that a Shim delegates to after Cloak handling.
_Avoid_: original binary, underlying CLI

**Connector**:
A user-configurable definition of how Cloak manages Context Enrollment and Activation for a Managed CLI.
_Avoid_: adapter, integration, context

**Connector Registry**:
A catalog of Connector definitions users can obtain and register with Cloak.
_Avoid_: adapter registry, installed connectors, context store

**Connector Source**:
The origin from which a Connector definition is obtained.
_Avoid_: connector type, context

## Relationships

- A **Managed CLI** has zero or more **Contexts**.
- A **Context** name is unique within its **Managed CLI**, not globally across Cloak.
- A **Managed CLI** has at most one **User-Global Active Context** at a time.
- A **User-Global Active Context** is the only supported **Active Context** scope in the initial product.
- A **Context** is made of **Context Metadata** and references to **Secret Material**.
- **Secret Material** is not part of **Context Metadata**.
- **Secret Material** is not revealed by Context inspection commands.
- A **Context** may have a **Default Database**, but a **Default Database** is not required.
- **Context Enrollment** uses the **Authentication Flow** defined by the **Connector** for the **Managed CLI**.
- A **Context Wizard** supplies Context values during explicit **Context Configuration** according to the **Connector**.
- Installing or updating a **Connector** is independent of **Context Enrollment** and **Context Configuration**.
- A **Context** must satisfy its **Connector**'s requirements for **Activation** to succeed.
- **Activation** is ephemeral and does not mutate the Real Command's native configuration.
- A **Pass-through Invocation** happens when a Managed CLI has no Active Context selected.
- A **Pass-through Invocation** also happens when a Real Command invocation contains **Explicit Connection Input**.
- **Explicit Connection Input** wins over an **Active Context** for the whole invocation.
- An unrecognized native CLI option does not by itself prevent **Activation**.
- **Scope Selection** does not prevent **Activation** by itself.
- A **Transport Option** does not prevent **Activation** by itself.
- **Activation** emits an **Invocation Notice**.
- A **Pass-through Invocation** emits an **Invocation Notice**.
- The **Cloak Command** owns Connector management, Context management, and Shim management.
- A **Shim** handles ordinary Real Command invocations, including Activation or Pass-through Invocation.
- A **Managed CLI** has at most one registered **Connector**.
- Removing a **Connector** preserves its Managed CLI's **Contexts** and **Secret Material**.
- A **Connector Registry** contains Connector definitions available for users to register.
- A **Connector** being available in a **Connector Registry** does not mean it is registered for a user.
- A **Connector Source** may be a **Connector Registry** entry or a user-supplied definition.
- Changes to a source definition affect a registered **Connector** only when the user explicitly updates it.
- A **Shim** belongs to exactly one **Managed CLI**.
- A **Shim** delegates ordinary CLI usage to the **Real Command** for its **Managed CLI** under the selected **Active Context**.

## Example dialogue

> **Dev:** "If `mongosh` is a **Managed CLI**, what happens after I switch its **Active Context** to `production`?"
> **Domain expert:** "Normal `mongosh` commands should use the `production` **Context** without the agent seeing raw credentials."
>
> **Dev:** "Does **Context Enrollment** always mean prompting for a username and password?"
> **Domain expert:** "No — a **Connector** identifies the values its **Authentication Flow** needs, such as a password or token. A **Context Wizard** can collect those values during explicit **Context Configuration**."
>
> **Dev:** "Is the `production` **Context** itself a **Connector**?"
> **Domain expert:** "No — the **Connector** describes how Cloak manages a **Managed CLI**; the `production` **Context** supplies a named connection target for that Managed CLI."
>
> **Dev:** "If `psql` is listed in the **Connector Registry**, is it already a **Managed CLI** for me?"
> **Domain expert:** "You first register its **Connector** with Cloak; registry availability alone does not enable it."
>
> **Dev:** "If I edit the definition I used to register a **Connector**, does its behavior change immediately?"
> **Domain expert:** "The registered **Connector** changes only when you explicitly update it, whether its source is your own definition or the **Connector Registry**."
>
> **Dev:** "Are registry Connectors and local Connectors different kinds of **Connector**?"
> **Domain expert:** "They differ in **Connector Source**; once registered, both supply the same kind of definition for Context Enrollment and Activation."
>
> **Dev:** "What happens when a **Connector** update requires a value that an existing **Context** is missing?"
> **Domain expert:** "The Connector update succeeds independently. Attempting Activation with that Context fails and directs the user to explicitly configure the Context."
>
> **Dev:** "Does removing a **Connector** delete its Managed CLI's **Contexts**?"
> **Domain expert:** "The Contexts and their Secret Material remain available for reuse after the Connector is registered again; deleting a Context is a separate operation."
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
> **Dev:** "Does the **Shim** handle **Context Configuration**?"
> **Domain expert:** "The **Cloak Command** handles Context management. The **Shim** applies an Active Context to ordinary Real Command invocations."

## Flagged ambiguities

- "profile", "environment", and "account" can all mean different things in other CLIs; in Cloak, the canonical term is **Context**.
- The same **Context** name in two different **Managed CLIs** does not imply shared Context Metadata or Secret Material.
- "credentials" often mixes secret and non-secret connection data; in Cloak, **Context Metadata** and **Secret Material** are separate concepts.
- "database" does not define a **Context** by itself; when present, it is an optional **Default Database**.
- Creating a Context is **Context Enrollment**, setting or changing its values is **Context Configuration**, and obtaining access uses an **Authentication Flow**.
- "local context" and "project context" are intentionally out of scope for the initial product; the selected **Active Context** is user-global.
- "Adapter" was the former term for Managed CLI-specific behavior; **Connector** is the canonical term for its user-configurable definition.
- "Connector Registry" means the catalog of available definitions; it does not mean the set of Connectors a user has registered.
- "Registry Connector" and "local Connector" describe different **Connector Sources**, not different execution models.
- "Missing arguments" when a Context no longer satisfies its Connector means missing Context values to supply through explicit **Context Configuration**; Connector installation and update do not require those values.
- Forwarding an unrecognized native option unchanged can occur during **Activation**; it does not by itself mean a **Pass-through Invocation**.
- "Control Prefix" is a retired concept; the **Cloak Command** owns Context management, and Shims handle Real Command invocations.
- Transport security toggles such as TLS are **Transport Options**, not **Explicit Connection Input**, unless they also carry identity or credential material.
