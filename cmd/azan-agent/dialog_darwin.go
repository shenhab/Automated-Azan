//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func confirmDialog(title, message string) bool {
	script := fmt.Sprintf(
		`display dialog %q with title %q `+
			`buttons {"Cancel", "Confirm"} default button "Cancel" with icon caution`,
		message, title,
	)
	out, err := exec.Command("osascript", "-e", script).Output()
	return err == nil && strings.Contains(string(out), "Confirm")
}

func showInfoDialog(title, message string) {
	script := fmt.Sprintf(
		`display dialog %q with title %q buttons {"OK"} default button "OK"`,
		message, title,
	)
	exec.Command("osascript", "-e", script).Run() //nolint:errcheck
}
