# Use symlink Shims

Installed Shims will be symlinks to the Cloak binary, with Cloak detecting the Managed CLI from its invocation name. We chose this BusyBox-style approach because macOS and Linux support symlinks well, one binary is easier to update than generated per-CLI launchers, and the same executable can serve both as the standalone Cloak Command and as each Managed CLI Shim.
