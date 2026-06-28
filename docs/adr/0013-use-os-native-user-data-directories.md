# Use OS-native user data directories

Cloak will store user-global configuration, state, and installed shims under OS-native per-user application directories rather than a hardcoded `~/.cloak` directory. We chose this so Cloak can behave naturally across macOS, Linux, and Windows while keeping its internal layout simple: configuration for Context Metadata, state for Active Context selections, and a shim directory for PATH-based executables.
