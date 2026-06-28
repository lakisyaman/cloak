# Implement Cloak in Go

Cloak will be implemented in Go for the initial product. We chose Go because Cloak is a CLI infrastructure tool that needs reliable argv/env handling, subprocess delegation, OS integration, and simple binary distribution across macOS and Linux; Go gives us those properties with less packaging complexity than Node, Python, or a plugin-heavy runtime.
