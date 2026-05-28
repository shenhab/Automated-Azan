// Package appdirs resolves OS-standard directories for config, data and logs.
//
// macOS:   ~/Library/Application Support/AzanAgent/
//          ~/Library/Logs/AzanAgent/
// Windows: %APPDATA%\AzanAgent\
// Linux:   ~/.config/azan-agent/   (XDG)
//          ~/.local/share/azan-agent/
package appdirs

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	appNameUnix = "azan-agent"   // XDG / Linux convention
	appNameGUI  = "AzanAgent"    // macOS / Windows convention
)

// Config returns the directory that should hold azan.toml.
func Config() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	return filepath.Join(base, name())
}

// Data returns the directory for downloaded timetable JSON files.
func Data() string {
	switch runtime.GOOS {
	case "darwin":
		// macOS: ~/Library/Application Support/AzanAgent/data
		base, err := os.UserConfigDir()
		if err != nil {
			return "data"
		}
		return filepath.Join(base, appNameGUI, "data")
	case "windows":
		base, err := os.UserConfigDir()
		if err != nil {
			return "data"
		}
		return filepath.Join(base, appNameGUI, "data")
	default:
		// Linux: ~/.local/share/azan-agent
		home, err := os.UserHomeDir()
		if err != nil {
			return "data"
		}
		return filepath.Join(home, ".local", "share", appNameUnix)
	}
}

// Logs returns the directory for log files.
func Logs() string {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "logs"
		}
		return filepath.Join(home, "Library", "Logs", appNameGUI)
	case "windows":
		base, err := os.UserConfigDir()
		if err != nil {
			return "logs"
		}
		return filepath.Join(base, appNameGUI, "logs")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "logs"
		}
		return filepath.Join(home, ".local", "share", appNameUnix, "logs")
	}
}

// EnsureAll creates all standard directories, returning the first error.
func EnsureAll() error {
	for _, dir := range []string{Config(), Data(), Logs()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func name() string {
	if runtime.GOOS == "linux" {
		return appNameUnix
	}
	return appNameGUI
}
