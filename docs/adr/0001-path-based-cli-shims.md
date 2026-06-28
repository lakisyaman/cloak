# Use PATH-based CLI shims

Cloak will make third-party CLIs context-aware by installing command shims earlier in `PATH` than the real executable. We chose this over a separate `cloak run ...` workflow because the primary user is an AI agent, and preserving the original CLI shape keeps commands discoverable, natural for models, and compatible with existing CLI habits while still allowing Cloak to intercept Cloak-specific control commands and inject the selected connection context.
