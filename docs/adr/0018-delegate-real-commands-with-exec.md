# Delegate Real Commands with process replacement

On macOS and Linux, Cloak will delegate normal Real Command invocations by preparing argv/env, emitting any Invocation Notice, and replacing the Cloak process with the Real Command via Unix process replacement. We chose this over spawning and waiting because Cloak does not need post-command cleanup in v1, and process replacement preserves the Real Command's stdio, signal behavior, and exit semantics with less proxying code.
