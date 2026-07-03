package appdirs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// wantAppName returns the app-name path segment expected on the current
// runtime.GOOS, matching the convention documented in appdirs.go.
func wantAppName() string {
	if runtime.GOOS == "linux" {
		return appNameUnix
	}
	return appNameGUI
}

// setDeterministicEnv points HOME/XDG_CONFIG_HOME/APPDATA at tmpDir so
// os.UserConfigDir and os.UserHomeDir resolve predictably in tests,
// regardless of the environment the test runner happens to have.
func setDeterministicEnv(t *testing.T, tmpDir string) {
	t.Helper()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))
	t.Setenv("APPDATA", filepath.Join(tmpDir, "AppData", "Roaming"))
	t.Setenv("USERPROFILE", tmpDir)
}

func TestConfig(t *testing.T) {
	tmpDir := t.TempDir()
	setDeterministicEnv(t, tmpDir)

	got := Config()
	if got == "" {
		t.Fatal("Config() returned empty path")
	}
	want := wantAppName()
	if !strings.Contains(got, want) {
		t.Errorf("Config() = %q, want it to contain %q", got, want)
	}
}

func TestData(t *testing.T) {
	tmpDir := t.TempDir()
	setDeterministicEnv(t, tmpDir)

	got := Data()
	if got == "" {
		t.Fatal("Data() returned empty path")
	}
	want := wantAppName()
	if !strings.Contains(got, want) {
		t.Errorf("Data() = %q, want it to contain %q", got, want)
	}
}

func TestLogs(t *testing.T) {
	tmpDir := t.TempDir()
	setDeterministicEnv(t, tmpDir)

	got := Logs()
	if got == "" {
		t.Fatal("Logs() returned empty path")
	}
	want := wantAppName()
	if !strings.Contains(got, want) {
		t.Errorf("Logs() = %q, want it to contain %q", got, want)
	}
}

func TestEnsureAll(t *testing.T) {
	tmpDir := t.TempDir()
	setDeterministicEnv(t, tmpDir)

	if err := EnsureAll(); err != nil {
		t.Fatalf("EnsureAll() error = %v, want nil", err)
	}

	for _, dir := range []string{Config(), Data(), Logs()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected directory %q to exist, got error: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory", dir)
		}
	}
}

func TestName(t *testing.T) {
	got := name()

	if runtime.GOOS == "linux" {
		if got != "azan-agent" {
			t.Errorf("name() on linux = %q, want %q", got, "azan-agent")
		}
		return
	}

	if got != "AzanAgent" {
		t.Errorf("name() on %s = %q, want %q", runtime.GOOS, got, "AzanAgent")
	}
}
