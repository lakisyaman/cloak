# Use `cloak` as the Shim Control Prefix

Status: superseded by [ADR 0021](./0021-route-context-management-through-cloak.md).

Cloak-specific commands inside a Managed CLI will use `cloak` as the Control Prefix, such as `mongosh cloak context list`, rather than reserving the generic `context` subcommand. We chose this because upstream CLIs may already use or later add their own `context` command; the product-named prefix is still readable for agents while making collisions much less likely.
