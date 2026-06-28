# Provide a standalone Cloak Command

Cloak will provide both a standalone `cloak` executable and per-Managed-CLI Shim control commands such as `mongosh cloak context list`. We chose this split because shim control keeps common context operations close to the CLI an agent is using, while the standalone command is necessary for installation, diagnostics, and cross-CLI management that do not naturally belong to a single Managed CLI.
