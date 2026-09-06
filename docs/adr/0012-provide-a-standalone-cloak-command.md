# Provide a standalone Cloak Command

Status: the split between standalone and Shim-scoped management is superseded by [ADR 0021](./0021-route-context-management-through-cloak.md); the standalone Cloak Command remains.

Cloak will provide both a standalone `cloak` executable and per-Managed-CLI Shim control commands such as `mongosh cloak context list`. We chose this split because shim control keeps common context operations close to the CLI an agent is using, while the standalone command is necessary for installation, diagnostics, and cross-CLI management that do not naturally belong to a single Managed CLI.
