# Compose transport-only options with the Active Context

Transport-only settings such as `redis-cli --tls` and libpq `sslmode`/`PGSSLMODE` will not be treated as Explicit Connection Input by themselves; they compose with an Active Context and override only the Context's transport metadata for that invocation. We chose this because making transport toggles trigger a full Pass-through Invocation would silently drop host and identity Activation, causing commands like `redis-cli --tls ping` to run against the wrong default target instead of the selected Context.
