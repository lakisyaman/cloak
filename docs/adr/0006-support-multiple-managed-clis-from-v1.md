# Support multiple Managed CLIs from v1

Cloak will validate its core model against multiple Managed CLIs in the first version rather than shipping a Mongo-only proof of concept. We chose this because the project is about giving any CLI an AWS/kubectl-like context layer; supporting more than one CLI early forces the Shim, Context, Adapter, and Secret Material boundaries to stay generic instead of accidentally encoding Mongo-specific assumptions.
