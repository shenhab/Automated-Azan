package main

// version is injected at build time via -ldflags "-X main.version=vX.Y.Z".
// Falls back to "dev" for local builds without the flag.
var version = "dev"
