//go:build !windows

package main

// isElevated always returns true on non-Windows platforms; privilege checks
// for service installation are not needed there.
func isElevated() bool { return true }

// showServiceInstallMessage is a no-op on non-Windows platforms.
func showServiceInstallMessage() {}
