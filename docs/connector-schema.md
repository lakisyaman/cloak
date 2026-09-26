# Connector schema, version 1

Connector definitions are single YAML documents. Unknown keys, duplicate keys, unsupported versions, and ambiguous repeated bindings are errors. No scripts or executable hooks are accepted. See the complete [Redis](../registry/redis-cli.yaml), [PostgreSQL](../registry/psql.yaml), and [MongoDB](../registry/mongosh.yaml) definitions.

```yaml
version: 1
command: acme-cli
description: Example client
parsing:
  shortOptions: true
  stopAtFirstPositional: true
  valueFlags: ["--output"]
passthrough:
  flags: ["--connection-uri"]
fields:
  endpoint:
    type: string
    required: true
    onInput: passthrough
    input:
      flags: ["--endpoint"]
      env: [ACME_ENDPOINT]
    inject: {env: ACME_ENDPOINT}
  token:
    type: string
    secret: true
    onInput: passthrough
    input: {env: [ACME_TOKEN]}
    inject: {env: ACME_TOKEN}
  namespace:
    type: string
    onInput: override
    input: {flags: ["-n", "--namespace"]}
    inject: {flag: "--namespace"}
  insecure:
    type: boolean
    onInput: override
    input: {flags: ["--insecure"]}
    inject: {flag: "--insecure"}
```

Install with `cloak connector add ./acme-cli.yaml`, configure with `cloak acme-cli context configure production`, and select with `cloak acme-cli context switch production`.

## Top-level properties

| Property | Meaning |
| --- | --- |
| `version` | Required integer, currently `1`. |
| `command` | Executable basename and Connector identity; letters, digits, `.`, `_`, `-`, with an alphanumeric first character, at most 100 characters. Cloak management names are reserved. |
| `description` | Optional description. |
| `fields` | Required nonempty mapping from stable field names to field definitions. Names start with a letter and otherwise contain letters or digits. Existing Contexts are keyed by these names. |
| `passthrough` | Optional input selectors that skip the entire Context without storing a corresponding field. |
| `parsing` | Optional native parser hints described below. |

## Fields

| Property | Meaning |
| --- | --- |
| `type` | Required: `string`, `integer` (signed 64-bit), `number` (finite float64), or `boolean`. |
| `required` | Defaults to false. Requires a nonempty value during configuration and when Activation uses this field. False is a valid boolean value. |
| `secret` | Defaults to false. True requires a string and stores the value in the OS keyring. Never put literal secrets in a definition. |
| `configureFlag` | Optional lowercase flag name; defaults to the lowercase field name. Use e.g. `default-database` for `defaultDatabase`. |
| `input` | Optional native flag, environment, or positional selectors. |
| `onInput` | Required when input selectors exist: `passthrough` or `override`. No conditions or roles. |
| `inject` | Optional single flag, environment, or positional binding. A field may omit it when used by another field's URI transformation. |
| `uri` | Optional URI validation and default-path rule for string fields. |

Secret fields are never copied from old plaintext metadata during Activation. If a schema changes a field's secret classification, explicitly reconfigure it. Unknown retained fields are ignored by Activation and preserved by configuration; Context inspection hides fields that the installed schema cannot classify.

Types are checked without echoing invalid values. Integer/number/boolean configuration strings are converted to their scalar types. Empty values mean absent; required fields reject absence. Boolean strings are exactly `true` or `false`.

## Inputs and precedence

`input` and top-level `passthrough` accept `flags: ["-h", "--host"]`, `flagValues: [{flag: "-d", prefixes: ["postgres://"], contains: ["="]}]`, `env: [PGHOST]`, and `positionals: [{index: 0}]`. Positional indices start at zero after removing parsed options and their values. Optional `prefixes` and `contains` lists on a positional select it only when its value starts with or contains one of the strings, as used to recognize a MongoDB URI. A `flagValues` entry needs at least one of the lists and matches when any value of that flag does; the flag consumes a value for parsing purposes and cannot be a boolean field flag. A value-matched positional or flag may repeat one plain selector for the same position or flag (see [ADR 0023](adr/0023-select-connection-strings-by-value.md)).

All selectors are alternatives. Presence of a declared flag is enough to match, including a missing native value; the Real Command still receives the original arguments and determines native syntax validity. Environment selectors match nonempty variables. A passthrough match anywhere wins over overrides and bypasses all Context access. An override suppresses validation, secret lookup, and injection for that field; it never rewrites the caller's value.

For example, psql's `-d`, `--dbname`, and database operand override only the default database, while a top-level passthrough value selector skips the Context when the same input holds a `postgres://` URI or a `key=value` connection string. There is no `passthroughWhen`, `role`, or `takesValue` property.

## Parsing hints

- `valueFlags` lists native options unrelated to fields that consume one following value, such as psql's `-c` and `--command`. This prevents command text or filenames from being mistaken for a database operand. Field flags derive value consumption from their scalar type; top-level passthrough flags consume a value for parsing purposes.
- `shortOptions: true` enables bundles and attached values (`-Atc 'select 1'`, `-danalytics`). A value-taking short flag consumes the bundle's remaining characters or the next argument.
- `stopAtFirstPositional: true` treats everything after the first operand as command data, as needed for Redis command payloads. Default false allows native options after operands, as in psql.

Equals forms are recognized. `--` ends option scanning. A lone `-` is a stdin marker. Unknown flags remain untouched and do not alone skip Activation, but unknown value-taking options can confuse positional detection. Connector authors should keep parser hints and connection input selectors current.

## Injection

Use exactly one of `inject: {flag: "--host"}`, `inject: {env: PGHOST}`, or `inject: {positional: true}`. Only one field can inject a positional argument. Repeated injection destinations are rejected. Positional output precedes flag output; flags are ordered by field name; caller arguments follow unchanged. Environment injection replaces the matching child variable.

Boolean fields may inject only flags: true emits the flag alone; false or absence emits nothing. Other scalar fields emit the flag followed by a separate value, or their scalar string as an environment/positional value. Cloak never invokes a shell to expand values.

## URI rule

```yaml
uri:
  type: string
  required: true
  secret: true
  inject: {positional: true}
  uri:
    schemes: [mongodb, mongodb+srv]
    defaultPathFrom: defaultDatabase
```

`schemes` is a required nonempty allowlist. `defaultPathFrom` optionally names another non-secret string field. When the URI has no path and the referenced field has a value, Cloak adds that value as an escaped path segment. Existing paths and queries win; authorities containing multiple hosts and embedded credentials are preserved. URIs containing passwords require a secret field. This operation is deliberately small and does not validate every backend-specific URI constraint.

## Contributing and distribution

Add `<command>.yaml` to `registry/`, then add behavioral coverage for its native connection inputs and required capabilities. The test suite validates every registry YAML file and its filename/command identity. Existing capabilities need no runtime implementation or binary rebuild. A new vocabulary feature requires a compatible Cloak version; unsupported schema versions fail before installation.

Only the explicitly requested definition is downloaded. Add/update save an independent private snapshot with source provenance. Editing a source file or the repository has no effect until `cloak connector update <cli>` succeeds. Definitions are not bundled into releases.
