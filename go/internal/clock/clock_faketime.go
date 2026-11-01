//go:build azanfaketime

package clock

import (
	"log"
	"os"
	"time"
)

// With the faketime tag, AZAN_FAKE_NOW (RFC 3339) sets the instant the clock
// reads at process start; it then advances in real time.
func init() {
	v := os.Getenv("AZAN_FAKE_NOW")
	if v == "" {
		return
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		log.Fatalf("[clock] invalid AZAN_FAKE_NOW %q: %v", v, err)
	}
	offset := time.Until(t)
	Now = func() time.Time { return time.Now().Add(offset) }
	log.Printf("[clock] FAKE TIME in effect: starting at %s", t.Format(time.RFC3339))
}
