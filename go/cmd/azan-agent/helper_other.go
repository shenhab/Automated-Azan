//go:build !darwin

package main

// ensureHelperBinary is a no-op on non-macOS platforms; returns empty string.
func ensureHelperBinary() (string, error) { return "", nil }
