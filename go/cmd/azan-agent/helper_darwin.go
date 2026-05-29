//go:build darwin

package main

import (
	"io"
	"os"
	"path/filepath"
)

// ensureHelperBinary copies the current executable to a stable location outside
// the .app bundle and returns that path.
//
// Why: launchd associates any process whose executable lives inside a .app
// bundle with that bundle.  When the service runs from inside AzanAgent.app,
// macOS treats it as the "running instance" of the app.  A subsequent
// double-click in Finder then tries to *activate* the service process instead
// of launching a fresh one — and since the service has no NSApplication event
// loop it never responds, producing "You can't open … because it is not
// responding."
//
// Running the LaunchAgent from outside the bundle breaks the association so
// Finder always starts a new interactive process.
func ensureHelperBinary() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	helperDir := filepath.Join(home, "Library", "Application Support", "AzanAgent")
	helperBin := filepath.Join(helperDir, "azan-agent")

	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}

	// Already running as the helper (i.e. we are the service process) — nothing to do.
	if self == helperBin {
		return helperBin, nil
	}

	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		return "", err
	}

	src, err := os.Open(self)
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Write to a temp file first so a partial overwrite never leaves a broken binary.
	tmp := helperBin + ".tmp"
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmp)
		return "", err
	}
	dst.Close()

	if err := os.Rename(tmp, helperBin); err != nil {
		os.Remove(tmp)
		return "", err
	}

	return helperBin, nil
}
