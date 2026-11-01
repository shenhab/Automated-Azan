//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"azan-agent/internal/fakecast"
)

// These tests run the whole real agent with its clock started on a daylight-
// saving night (binary mode only; see withFakeNow) and check that prayers fire,
// and the schedule is rebuilt, at the right absolute instants.
//
// Ireland's 2026 clock changes, written down here independently of any code
// under test: clocks go forward at 01:00 UTC on 29 March (GMT → IST, UTC+1) and
// back at 01:00 UTC on 25 October.

func isIST(month time.Month, day int) bool {
	switch {
	case month > time.March && month < time.October:
		return true
	case month == time.March:
		return day >= 29
	case month == time.October:
		return day <= 24
	}
	return false
}

// A correct timetable keeps the prayers at the same UTC instants all year, so
// its local times move by an hour with the clocks. Order: Fajr, Sunrise, Dhuhr,
// Asr, Maghrib, Isha.
var utcTimes = [6][2]int{{3, 10}, {5, 0}, {12, 25}, {15, 0}, {18, 0}, {20, 0}}

func correctTimes(month time.Month, day int) [6]string {
	var out [6]string
	shift := 0
	if isIST(month, day) {
		shift = 1
	}
	for i, hm := range utcTimes {
		out[i] = fmt.Sprintf("%02d:%02d", hm[0]+shift, hm[1])
	}
	return out
}

// writeYearTimetable writes a full-year Naas-format timetable using fn for each
// (month, day).
func writeYearTimetable(t *testing.T, home string, fn func(month time.Month, day int) [6]string) {
	t.Helper()
	var months []string
	for m := time.January; m <= time.December; m++ {
		var days []string
		for d := 1; d <= 31; d++ {
			times := fn(m, d)
			days = append(days, fmt.Sprintf("%q:[%s]", fmt.Sprint(d), quoteJoin(times[:])))
		}
		months = append(months, "{"+strings.Join(days, ",")+"}")
	}
	dir := filepath.Join(home, ".local", "share", "azan-agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "[" + strings.Join(months, ",") + "]"
	if err := os.WriteFile(filepath.Join(dir, "naas_prayers_timetable.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSpeaker(t *testing.T) *fakecast.Speaker {
	t.Helper()
	sp := fakecast.New(speakerName)
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	return sp
}

// An Athan scheduled for 02:01 IST on the spring-forward day is 3 real minutes
// after 00:58 GMT — the clock skips the hour in between. It must fire then, not
// an hour late (as if 01:xx had existed) or an hour early.
func TestAthanFiresAtTheRightInstantAcrossTheSpringForwardGap(t *testing.T) {
	sp := newSpeaker(t)
	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)

	fakeStart := time.Date(2026, 3, 29, 0, 58, 0, 0, time.UTC)
	fakeFire := time.Date(2026, 3, 29, 1, 1, 0, 0, time.UTC) // = 02:01 IST
	writeYearTimetable(t, home, func(m time.Month, d int) [6]string {
		times := correctTimes(m, d)
		if m == time.March && d == 29 {
			times[3] = "02:01" // Asr, in local (IST) time
		}
		return times
	})
	writeConfig(t, cfgFile, webPort, "Asr")

	startReal := time.Now()
	startAgent(t, home, cfgFile, sp, withFakeNow(fakeStart.Format(time.RFC3339)))

	loads, err := sp.WaitForLoads(1, 4*time.Minute+30*time.Second)
	if err != nil {
		t.Fatalf("the Athan never reached the speaker: %v", err)
	}
	// Convert the speaker's real receive time into the agent's fake time.
	fakeAt := loads[0].At.Add(fakeStart.Sub(startReal)).UTC()
	if early := fakeFire.Sub(fakeAt); early > 4*time.Second {
		t.Errorf("fired %v early: at %s UTC, want %s UTC", early, fakeAt.Format("15:04:05"), fakeFire.Format("15:04:05"))
	}
	if late := fakeAt.Sub(fakeFire); late > 25*time.Second {
		t.Errorf("fired %v late: at %s UTC, want %s UTC", late, fakeAt.Format("15:04:05"), fakeFire.Format("15:04:05"))
	}
	time.Sleep(8 * time.Second)
	if n := len(sp.Loads()); n != 1 {
		t.Errorf("speaker received %d LOAD commands, want exactly 1", n)
	}
}

// At 00:05 on each clock-change night the agent rebuilds the day's schedule —
// exactly once — and every prayer lands on the right absolute instant.
func TestScheduleIsRebuiltOnceAndCorrectlyOnClockChangeNights(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start time.Time // 30 s before the 00:05 local refresh
		date  string    // local date being scheduled
	}{
		{"spring-forward day, 29 Mar 2026", time.Date(2026, 3, 29, 0, 4, 30, 0, time.UTC), "2026-03-29"},
		// 00:05 on 25 Oct is still IST (UTC+1), i.e. 23:05 UTC the evening before.
		{"fall-back day, 25 Oct 2026", time.Date(2026, 10, 24, 23, 4, 30, 0, time.UTC), "2026-10-25"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sp := newSpeaker(t)
			home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
			writeYearTimetable(t, home, correctTimes)
			writeConfig(t, cfgFile, webPort, "all")

			ag := startAgent(t, home, cfgFile, sp, withFakeNow(tc.start.Format(time.RFC3339)), withCredentials(testUser, testPass))
			api := newClient(t, webPort)

			// The refresh happens 30 s of fake (= real) time after start.
			deadline := time.Now().Add(90 * time.Second)
			for !strings.Contains(ag.logs.String(), "daily refresh triggered") {
				if time.Now().After(deadline) {
					t.Fatal("the 00:05 refresh never ran")
				}
				time.Sleep(500 * time.Millisecond)
			}
			time.Sleep(8 * time.Second)
			if n := strings.Count(ag.logs.String(), "daily refresh triggered"); n != 1 {
				t.Errorf("the refresh ran %d times, want exactly 1", n)
			}

			// What did it schedule?
			api.mustLogin()
			var status struct {
				Jobs []struct {
					Prayer  string `json:"prayer"`
					NextRun string `json:"next_run"`
				} `json:"jobs"`
			}
			if code := api.getJSON("/api/scheduler-status", &status); code != http.StatusOK {
				t.Fatalf("scheduler-status returned %d", code)
			}
			got := map[string]string{}
			for _, j := range status.Jobs {
				at, err := time.Parse(time.RFC3339, j.NextRun)
				if err != nil {
					t.Fatalf("%s: bad next_run %q", j.Prayer, j.NextRun)
				}
				got[j.Prayer] = at.UTC().Format("2006-01-02 15:04")
			}
			// The same five UTC instants on both days, on the right date.
			want := map[string]string{
				"Fajr": tc.date + " 03:10", "Dhuhr": tc.date + " 12:25", "Asr": tc.date + " 15:00",
				"Maghrib": tc.date + " 18:00", "Isha": tc.date + " 20:00",
			}
			for prayer, w := range want {
				if got[prayer] != w {
					t.Errorf("%s scheduled at %q UTC, want %q UTC", prayer, got[prayer], w)
				}
			}
		})
	}
}
