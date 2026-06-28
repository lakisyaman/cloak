# Separate Context Metadata from Secret Material

Cloak will store non-secret Context Metadata in its own configuration, while Secret Material is stored in the operating system's secret store or an equivalent secure backend. We chose this split over keeping full connection strings in Cloak config because the project exists to make CLIs safer for AI agents; plaintext configuration would make credentials easy to expose in shell history, logs, prompts, repository files, or debugging output.
