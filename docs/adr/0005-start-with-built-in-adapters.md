# Start with built-in Adapters

Cloak will initially support Managed CLIs through built-in Adapters rather than declarative manifests or executable plugins. We chose this because the first product needs to prove the context model and shim workflow before investing in a general adapter system; external Adapter definitions remain a likely future extension once multiple CLIs expose repeated patterns.
