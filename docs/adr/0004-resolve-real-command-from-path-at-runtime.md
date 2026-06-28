# Resolve the Real Command from PATH at runtime

A Shim will find its Real Command by searching `PATH` at execution time and excluding candidates that canonicalize to the Cloak binary itself, rather than relying on an absolute path captured during installation. We chose this because developer machines often change CLI locations through Homebrew, asdf, nvm-style version managers, or upgrades; runtime resolution avoids stale paths, and canonical self-exclusion prevents symlink shim recursion while still allowing an explicit override later if needed.
