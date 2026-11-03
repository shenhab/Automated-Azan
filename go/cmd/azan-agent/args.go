package main

import (
	"fmt"
	"strings"
)

// The commands the agent accepts on its command line.
const (
	cmdRun       = "run"
	cmdVersion   = "version"
	cmdInstall   = "install"
	cmdUninstall = "uninstall"
	cmdStart     = "start"
	cmdStop      = "stop"
)

const usage = `Usage: azan-agent [command]

With no command, the agent runs.

Commands:
  install      register the agent as a background service
  uninstall    remove the background service
  start        start the background service
  stop         stop the background service
  --version    print the version and exit`

// parseArgs maps the command line to a command. Anything else is an error, never
// a normal start: a mistyped argument would otherwise run a second agent beside
// the real one, and every Athan would play twice.
func parseArgs(args []string, goos string) (string, error) {
	switch len(args) {
	case 0:
		return cmdRun, nil
	case 1:
		switch arg := args[0]; {
		case arg == "--version" || arg == "-version" || arg == "-v" || arg == "version":
			return cmdVersion, nil
		case arg == cmdInstall || arg == cmdUninstall || arg == cmdStart || arg == cmdStop:
			return arg, nil
		case goos == "darwin" && strings.HasPrefix(arg, "-psn_"):
			// Finder can add a process serial number when it opens the app.
			return cmdRun, nil
		}
	}
	return "", fmt.Errorf("unknown arguments: %s", strings.Join(args, " "))
}
