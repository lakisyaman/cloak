# Use human-readable JSON for private state

Cloak will store Context Metadata and Active Context selections as human-readable JSON files in its user data directory, but those files are not a stable public API in v1. We chose this because JSON makes early debugging and support easier, while requiring users and agents to mutate state through the Cloak Command or Shim control commands preserves freedom to evolve the schema.
