# Use user-global Active Context for the initial product

Cloak will initially store one Active Context selection per Managed CLI for the current operating system user, rather than resolving project-local overrides. We chose this to keep the first version simple and predictable; project-local Context selection remains a possible future extension, but it is not part of the initial resolution model.
