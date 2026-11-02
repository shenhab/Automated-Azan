package chromecast

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"

	"azan-agent/internal/fakecast"
)

// Several connections in one process (the agent's own session, a TV being muted,
// the Journal watching) must never interfere with each other. The Cast client
// library keeps its request counter in a package-level variable and its reply
// table in an unguarded map; with those, concurrent requests lost replies and
// each loss cost a 5-second timeout (and could crash the program outright).
// See third_party/go-chromecast/PATCHES.md.
func TestConnectionsInOneProcessNeverLoseEachOthersReplies(t *testing.T) {
	sp := fakecast.New("Speaker")
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	host, port := sp.Addr()

	for round := 1; round <= 8; round++ {
		var wg sync.WaitGroup
		var slow atomic.Int32
		for k := 0; k < 4; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				app := application.NewApplication()
				t0 := time.Now()
				err := app.Start(host, port)
				if err != nil || time.Since(t0) > 2*time.Second {
					slow.Add(1)
				}
				if err == nil {
					for i := 0; i < 5; i++ {
						t1 := time.Now()
						if err := app.Update(); err != nil || time.Since(t1) > 2*time.Second {
							slow.Add(1)
						}
					}
					app.Close(false)
				}
			}()
		}
		wg.Wait()
		if n := slow.Load(); n > 0 {
			t.Fatalf("round %d: %d request(s) lost their reply or took over 2 s", round, n)
		}
	}
}
