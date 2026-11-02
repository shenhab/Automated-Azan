//go:build e2e

package e2e

import "testing"

// The agent binds its web and media ports some seconds after the harness picks
// them. A port from the kernel's ephemeral range (32768+ on Linux) can be taken in
// that gap by any outbound connection on the machine, which killed an agent
// ("media server listen ...: address already in use") and failed a release run.
// Picked ports must therefore come from outside that range, and never repeat.
func TestPickedPortsAvoidTheEphemeralRangeAndNeverRepeat(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 300; i++ {
		p := freePorts(t)
		if p >= 32768 || p+1 >= 32768 {
			t.Fatalf("port %d is in the ephemeral range, where outbound connections can steal it", p)
		}
		if p < 1024 {
			t.Fatalf("port %d is privileged", p)
		}
		if seen[p] || seen[p+1] || seen[p-1] {
			t.Fatalf("port %d (or a neighbour) was handed out twice", p)
		}
		seen[p], seen[p+1] = true, true
	}
}

// Parallel tests start containers at the same instant; their names must never
// collide (a clash makes the second `docker run` fail and its test time out).
func TestContainerNamesAreUniqueEvenWhenStartedAtTheSameInstant(t *testing.T) {
	const n = 2000
	names := make(chan string, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() { <-start; names <- containerName() }()
	}
	close(start)
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		nm := <-names
		if seen[nm] {
			t.Fatalf("container name %q was generated twice", nm)
		}
		seen[nm] = true
	}
}
