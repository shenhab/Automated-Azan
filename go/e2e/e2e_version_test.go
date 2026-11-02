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

// TestVersionFlagPrintsAndExits asserts that asking for the version only prints
// it. Before this flag existed, `azan-agent --version` started a complete second
// agent beside the running one, which would have played every Athan twice.
//
// A version request must exit at once and must not start anything or write to
// disk, so the test fails if the process is still alive after the timeout or if
// it leaves anything in its home directory.
func TestVersionFlagPrintsAndExits(t *testing.T) {
	for _, arg := range []string{"--version", "-version", "-v", "version"} {
		t.Run(arg, func(t *testing.T) {
			home := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var cmd *exec.Cmd
			image := os.Getenv("E2E_DOCKER_IMAGE")
			if image != "" {
				name := containerName()
				defer exec.Command("docker", "rm", "-f", name).Run()
				cmd = exec.CommandContext(ctx, "docker", "run", "--rm", "--name", name,
					"--network", "none", // never touch real devices on the LAN
					image, arg)
			} else {
				cmd = exec.CommandContext(ctx, agentBinary(t, ""), arg)
				cmd.Env = append(os.Environ(),
					"HOME="+home,
					"AZAN_DISABLE_MDNS=1", // never touch real devices on the LAN
				)
			}

			out, err := cmd.CombinedOutput()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatalf("%s did not exit: it started the agent instead\n%s", arg, out)
			}
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", arg, err, out)
			}

			got := strings.TrimSpace(string(out))
			if image == "" {
				if got != "azan-agent dev" {
					t.Errorf("%s printed %q, want %q", arg, got, "azan-agent dev")
				}
				entries, err := os.ReadDir(home)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Errorf("%s wrote to the home directory: %v", arg, entries)
				}
			} else if !strings.HasPrefix(got, "azan-agent ") || strings.Contains(got, "\n") {
				t.Errorf("%s printed %q, want one line starting with %q", arg, got, "azan-agent ")
			}
		})
	}
}
