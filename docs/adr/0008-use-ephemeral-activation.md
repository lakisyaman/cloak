# Use ephemeral Activation

Cloak will apply an Active Context to each Real Command invocation through ephemeral Activation, such as environment variables, arguments, temporary files, or stdin, rather than mutating the Real Command's native configuration files. We chose this because native config formats and side effects differ across CLIs; keeping Activation per-invocation makes behavior easier to reason about, avoids corrupting user config, and gives agents a safer reversible execution model.
