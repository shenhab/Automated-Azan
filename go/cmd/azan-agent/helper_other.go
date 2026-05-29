//go:build !darwin

package main

// ensureHelperBinary is a no-op on non-macOS platforms.
func ensureHelperBinary() (path string, updated bool, err error) { return "", false, nil }
