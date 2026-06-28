# Explicit Connection Input skips Activation

If a Real Command invocation contains Explicit Connection Input, Cloak will skip Activation entirely and perform a Pass-through Invocation, even when an Active Context exists. We chose this all-or-nothing rule because user-supplied connection state should win, and mixing caller-provided targets with Cloak-provided credentials could accidentally connect to the wrong system with the wrong identity.
