# Delegate without Activation when no Active Context exists

When a Managed CLI has no Active Context selected, Cloak will perform a Pass-through Invocation of the Real Command and emit a notice that no Context was applied, rather than failing closed. We chose this for compatibility with existing CLI behavior and to keep installing a Shim from breaking ordinary usage; the trade-off is that users and agents must notice when a command ran without Cloak-managed connection state.
