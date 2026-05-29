//go:build !darwin && !windows

package main

import (
	"os/exec"
	"strings"
)

func confirmDialog(title, message string) bool {
	// Try zenity (GTK dialog tool, common on Linux desktops).
	// Fall back to true (proceed without confirmation) on headless systems.
	if _, err := exec.LookPath("zenity"); err != nil {
		return true
	}
	out, err := exec.Command("zenity",
		"--question",
		"--title="+title,
		"--text="+message,
		"--ok-label=Confirm",
		"--cancel-label=Cancel",
	).Output()
	return err == nil || strings.TrimSpace(string(out)) == ""
}

func showInfoDialog(title, message string) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return
	}
	exec.Command("zenity", "--info", "--title="+title, "--text="+message).Run() //nolint:errcheck
}
