<!-- cloak:start -->
## Cloak

Use Cloak to manage connection Contexts for Managed CLIs.

- Discover installed Connectors with `cloak connector list`. Available definitions are listed in the [Connector Registry](https://github.com/lakisyaman/cloak/tree/main/registry). When the task requires enabling a new Managed CLI, install its definition with `cloak connector add @cloak/<cli>`. The native CLI must also be installed.
- Inspect Contexts with `cloak <cli> context list`, `cloak <cli> context current`, and `cloak <cli> context show <name>`. Use the Context intended for the task; ask the user if the target is unclear.
- Select a Context for your own session with `CLOAK_<CLI>_CONTEXT`, for example `export CLOAK_PSQL_CONTEXT=staging` or `CLOAK_REDIS_CLI_CONTEXT=cache redis-cli PING`. Prefer it over `cloak <cli> context switch <name>`, which is user-global and changes the target for other sessions and terminals.
- Run the native CLI normally through its Cloak Shim. Use `cloak doctor` to diagnose setup and PATH ordering. Check the Invocation Notice to see whether a Context was applied.
- Connector installation does not configure Contexts. If a Context needs configuration, use `cloak <cli> context configure <name> --help` for its fields and follow Cloak's configuration guidance. Keep Secret Material out of agent instruction files.
- Navigate with `cloak --help`, `cloak connector --help`, and `cloak <cli> context --help`; each subcommand also supports `--help`.
<!-- cloak:end -->
