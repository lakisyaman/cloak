# Target macOS and Linux for v1

Cloak will officially support macOS and Linux in the initial product, while keeping Windows compatibility in mind but out of scope. We chose this because PATH-based shims and OS secret storage are materially different on Windows; focusing v1 on Unix-like environments reduces implementation complexity without forcing a permanent Windows-incompatible design.
