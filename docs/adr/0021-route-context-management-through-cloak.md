# Route Context management through Cloak

Context management will use `cloak <cli> context ...`, including `configure <name>` for explicitly creating, editing, and repairing Context values, while Shims handle ordinary Real Command invocations without a Cloak Control Prefix. This makes management ownership explicit, improves discovery beneath the Cloak Command, and allows Context management independently of Shim invocation. It supersedes the Control Prefix in ADR 0011 and the split management surfaces in ADR 0012.
