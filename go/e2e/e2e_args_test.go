//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// runOnce runs the agent with args and returns its combined output and error.
// These commands must exit at once and start nothing, so the test fails if the
// process is still alive after the timeout, or if it leaves anything in its home
// directory (binary mode). Before these checks existed, any argument the agent
// did not recognise started a complete second agent beside the running one, which
// would have played every Athan twice.
func runOnce(t *testing.T, args ...string) (string, error) {
	t.Helper()
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	image := os.Getenv("E2E_DOCKER_IMAGE")
	if image != "" {
		name := containerName()
		defer exec.Command("docker", "rm", "-f", name).Run()
		dockerArgs := []string{"run", "--rm", "--name", name,
			"--network", "none", // never touch real devices on the LAN
			image}
		cmd = exec.CommandContext(ctx, "docker", append(dockerArgs, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, agentBinary(t, ""), args...)
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			"AZAN_DISABLE_MDNS=1", // never touch real devices on the LAN
		)
	}

	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("%q did not exit: it started the agent instead\n%s", args, out)
	}
	if image == "" {
		entries, readErr := os.ReadDir(home)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Errorf("%q wrote to the home directory: %v", args, entries)
		}
	}
	return strings.TrimSpace(string(out)), err
}

func TestVersionFlagPrintsAndExits(t *testing.T) {
	for _, arg := range []string{"--version", "-version", "-v", "version"} {
		t.Run(arg, func(t *testing.T) {
			got, err := runOnce(t, arg)
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", arg, err, got)
			}
			if os.Getenv("E2E_DOCKER_IMAGE") == "" {
				if got != "azan-agent dev" {
					t.Errorf("%s printed %q, want %q", arg, got, "azan-agent dev")
				}
			} else if !strings.HasPrefix(got, "azan-agent ") || strings.Contains(got, "\n") {
				t.Errorf("%s printed %q, want one line starting with %q", arg, got, "azan-agent ")
			}
		})
	}
}

// Only arguments that could not reach the service manager are used here: a
// rejected "start x" is covered by the unit test, so a regression can never make
// this test start or stop a service.
func TestUnknownArgumentsAreRejected(t *testing.T) {
	for _, args := range [][]string{
		{"--versoin"},
		{"--version", "extra"},
		{"restart"},
		{"-psn_0_12345"}, // only accepted on macOS, and these tests run on Linux
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got, err := runOnce(t, args...)
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 2 {
				t.Fatalf("%q: got error %v, want exit code 2\n%s", args, err, got)
			}
			if !strings.Contains(got, "unknown arguments") || !strings.Contains(got, "Usage:") {
				t.Errorf("%q printed %q, want the reason and the usage", args, got)
			}
			if !strings.Contains(got, args[0]) {
				t.Errorf("%q printed %q, want it to name the bad argument", args, got)
			}
		})
	}
}
