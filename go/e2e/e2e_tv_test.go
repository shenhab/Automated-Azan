//go:build e2e

package e2e

import (
	"path/filepath"
	"testing"
	"time"

	"azan-agent/internal/fakecast"
)

// With TV-mute enabled, TVs on the network are quieted just before the Athan and
// put back afterwards — and only what the agent changed is undone.
func TestTVsAreQuietedForTheAthanAndRestored(t *testing.T) {
	t.Parallel()
	const resumeAfter = 12 // seconds; configured below
	for _, tc := range []struct {
		name  string
		setup func(tv *fakecast.Speaker)
		// what the agent should do to this TV, and how it ends up
		before, after string
		endState      func(tv *fakecast.Speaker) string
	}{
		{
			name:   "an idle TV is muted, then unmuted",
			setup:  func(tv *fakecast.Speaker) {},
			before: "mute", after: "unmute",
			endState: func(tv *fakecast.Speaker) string {
				if _, m := tv.Volume(); m {
					return "muted"
				}
				return "unmuted"
			},
		},
		{
			name:   "a TV playing a show is paused, then resumed",
			setup:  func(tv *fakecast.Speaker) { tv.SimulateCast("http://example/show.mp4") },
			before: "pause", after: "play",
			endState: func(tv *fakecast.Speaker) string { return tv.PlayerState() },
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sp := newSpeaker(t)
			tv := fakecast.New("Living Room TV")
			if err := tv.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tv.Close)
			tc.setup(tv)

			home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
			fireAt := nextFireTime(100 * time.Second)
			writeNaasTimetable(t, home, fireAt, "Asr")
			writeConfig(t, cfgFile, webPort, "Asr", "\n[tv_pause]\nenabled = true\nresume_delay_seconds = 12\n")
			startAgent(t, home, cfgFile, sp, withExtraDevice("Living Room TV", tv))

			loads, err := sp.WaitForLoads(1, time.Until(fireAt)+90*time.Second)
			if err != nil {
				t.Fatalf("the Athan never reached the speaker: %v", err)
			}
			athanAt := loads[0].At

			// The TV is quieted just before the Athan starts.
			before := tv.EventsOfKind(tc.before)
			if len(before) != 1 {
				t.Fatalf("TV got %d %q event(s), want 1 (events: %v)", len(before), tc.before, tv.Events())
			}
			if lead := athanAt.Sub(before[0].At); lead < -time.Second || lead > 10*time.Second {
				t.Errorf("TV was quieted %v before the Athan, want a few seconds before", lead)
			}

			// ...and restored after the configured delay.
			deadline := time.Now().Add(resumeAfter*time.Second + 20*time.Second)
			for len(tv.EventsOfKind(tc.after)) == 0 && time.Now().Before(deadline) {
				time.Sleep(200 * time.Millisecond)
			}
			after := tv.EventsOfKind(tc.after)
			if len(after) != 1 {
				t.Fatalf("TV got %d %q event(s), want 1 (events: %v)", len(after), tc.after, tv.Events())
			}
			if gap := after[0].At.Sub(before[0].At); gap < (resumeAfter-3)*time.Second || gap > (resumeAfter+15)*time.Second {
				t.Errorf("TV restored %v after being quieted, want about %ds", gap, resumeAfter)
			}
			want := map[string]string{"mute": "unmuted", "pause": "PLAYING"}[tc.before]
			if got := tc.endState(tv); got != want {
				t.Errorf("TV ends %s, want %s", got, want)
			}

			// The Athan speaker itself is never muted or paused.
			if n := len(sp.EventsOfKind("mute")) + len(sp.EventsOfKind("pause")); n != 0 {
				t.Errorf("the Athan speaker was muted/paused %d time(s)", n)
			}
		})
	}
}

// A TV whose owner already muted it must be left exactly as it was.
func TestATVTheOwnerMutedIsLeftAlone(t *testing.T) {
	t.Parallel()
	sp := newSpeaker(t)
	tv := fakecast.New("Living Room TV")
	if err := tv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tv.Close)
	tv.SetVolumeState(0.4, true)

	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
	fireAt := nextFireTime(100 * time.Second)
	writeNaasTimetable(t, home, fireAt, "Asr")
	writeConfig(t, cfgFile, webPort, "Asr", "\n[tv_pause]\nenabled = true\nresume_delay_seconds = 6\n")
	startAgent(t, home, cfgFile, sp, withExtraDevice("Living Room TV", tv))

	if _, err := sp.WaitForLoads(1, time.Until(fireAt)+90*time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(14 * time.Second) // well past the resume delay
	if ev := tv.Events(); len(ev) != 0 {
		t.Errorf("the agent touched a TV the owner had muted: %v", ev)
	}
	if lvl, muted := tv.Volume(); !muted || lvl != 0.4 {
		t.Errorf("TV is now muted=%v level=%v, want muted at 0.4", muted, lvl)
	}
}
