//go:build e2e

package e2e

import (
	"path/filepath"
	"testing"
	"time"

	"azan-agent/internal/fakecast"
)

func TestQuietTimeStaysOnTheSameDay(t *testing.T) {
	day := func(h, m, s int) time.Time { return time.Date(2026, 11, 2, h, m, s, 0, dublin) }
	for _, c := range []struct {
		name      string
		now, want time.Time
	}{
		{"morning", day(9, 0, 0), day(9, 30, 0)},
		{"just after midnight", day(0, 0, 0), day(0, 30, 0)},
		{"last moment that fits today", day(23, 29, 59), day(23, 59, 59)},
		{"first moment that would be tomorrow", day(23, 30, 0), day(23, 0, 0)},
		// A run at 23:48 failed here: the old helper slept until 00:00:05 and the
		// test hit the 12-minute limit.
		{"when CI failed", day(23, 48, 20), day(23, 18, 20)},
		{"last second of the day", day(23, 59, 59), day(23, 29, 59)},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := quietTime(c.now); !got.Equal(c.want) {
				t.Errorf("quietTime(%s) = %s, want %s", c.now.Format("15:04:05"), got.Format("15:04:05"), c.want.Format("15:04:05"))
			}
		})
	}
}

// Late in the evening quietTime is in the past. The agent must treat that as
// already gone and play nothing, or the tests relying on it would hear an Athan.
func TestAPrayerEarlierTodayDoesNotPlayWhenTheAgentStarts(t *testing.T) {
	t.Parallel()
	sp := fakecast.New(speakerName)
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	now := time.Now().In(dublin)
	fakeNow := time.Date(now.Year(), now.Month(), now.Day(), 23, 45, 0, 0, dublin)
	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
	writeNaasTimetable(t, home, quietTime(fakeNow), "Asr")
	writeConfig(t, cfgFile, webPort, "Asr")
	startAgent(t, home, cfgFile, sp, withFakeNow(fakeNow.Format(time.RFC3339)), withCredentials(testUser, testPass))
	api := newClient(t, webPort)
	api.mustLogin()

	time.Sleep(10 * time.Second)
	if n := len(sp.Loads()); n != 0 {
		t.Errorf("the agent played %d time(s) for a prayer already past", n)
	}
}
