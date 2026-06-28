# Follow-ups

## Post-v1: revisit config/state write locking

V1 intentionally uses atomic JSON writes without lock files. This prevents torn writes but does not prevent lost updates when multiple Cloak processes mutate `config.json` or `state.json` concurrently.

After v1, revisit whether Cloak should add file locking around mutations based on observed agent concurrency patterns and any `cloak doctor` reports of inconsistent state.
