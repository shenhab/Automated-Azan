package main

// version is injected at build time via -ldflags "-X main.version=vX.Y.Z".
// Falls back to "dev" for local builds without the flag.
var version = "dev"

// isVersionRequest reports whether the command line asks only for the version.
func isVersionRequest(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "--version", "-version", "-v", "version":
		return true
	}
	return false
}
