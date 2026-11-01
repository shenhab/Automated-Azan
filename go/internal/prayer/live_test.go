//go:build live

// Live contract tests: download the real timetables from the real websites and
// check the agent can still read them and schedule from them correctly. These
// need the internet and depend on third-party sites, so they are NOT part of the
// normal test run; a scheduled GitHub workflow runs them (see
// .github/workflows/live-sources.yml) to catch a site changing its format or
// publishing bad data before it silently breaks the Athan.
//
//	go test -tags live ./internal/prayer/ -run Live -v
package prayer

import (
	"os"
	"testing"
	"time"
)

var liveSources = []string{"icci", "naas", "newbridge", "cork", "galway"}

func TestLiveTimetablesDownloadParseAndScheduleCorrectly(t *testing.T) {
	year := time.Now().In(mustDublin(t)).Year()
	for _, src := range liveSources {
		src := src
		t.Run(src, func(t *testing.T) {
			dir := t.TempDir()
			f, err := NewFetcher(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.ForceRefresh(src); err != nil {
				t.Fatalf("could not download the live %s timetable (site down, or its page format changed): %v", src, err)
			}
			raw, err := os.ReadFile(f.timetablePath(src))
			if err != nil {
				t.Fatal(err)
			}

			// Every day of the year must parse (a changed layout shows up here).
			// Count the days whose DST handling the sanity guard had to repair, so
			// a site publishing wrong clock-change dates is visible even though
			// the agent corrects for it.
			loc := mustDublin(t)
			fixed := 0
			for day := time.Date(year, 1, 1, 12, 0, 0, 0, loc); day.Year() == year; day = day.AddDate(0, 0, 1) {
				var rawTimes Times
				if src == "icci" {
					rawTimes, err = f.extractICCI(raw, day)
				} else {
					rawTimes, err = f.extractNaas(raw, day)
				}
				if err != nil {
					t.Fatalf("%s: live data no longer parses: %v", day.Format("2006-01-02"), err)
				}
				if _, shift := guardDST(rawTimes, day, loc); shift != 0 {
					fixed++
				}
			}
			if fixed > 0 {
				t.Logf("WARNING: the live %s timetable has %d day(s) with a wrong GMT/IST changeover; the agent corrects them, but the site's data is wrong", src, fixed)
			}

			// And the schedule built from the live data must be correct.
			assertConsistentYear(t, src, dir, year)
		})
	}
}

// Aladhan computes times itself, so its answer for the days around each clock
// change must be local Dublin time and show no one-hour jump in absolute time.
func TestLiveAladhanAroundClockChanges(t *testing.T) {
	loc := mustDublin(t)
	year := time.Now().In(loc).Year()
	f, err := NewFetcher(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Last Sundays of March and October, and the days either side.
	var days []time.Time
	for _, m := range []time.Month{time.March, time.October} {
		last := time.Date(year, m+1, 0, 12, 0, 0, 0, loc) // last day of month
		for last.Weekday() != time.Sunday {
			last = last.AddDate(0, 0, -1)
		}
		for d := -2; d <= 2; d++ {
			days = append(days, last.AddDate(0, 0, d))
		}
	}

	var prev, prevDay time.Time
	for _, day := range days {
		times, err := f.fetchAladhanAPI("Dublin", "Ireland", 3, day)
		if err != nil {
			t.Fatalf("%s: Aladhan request failed: %v", day.Format("2006-01-02"), err)
		}
		h, m, err := parseHHMM(times.Dhuhr)
		if err != nil {
			t.Fatalf("%s: bad Dhuhr %q", day.Format("2006-01-02"), times.Dhuhr)
		}
		dhuhr := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc).UTC()

		if d := absf(utcMinutes(dhuhr) - noonUTC(day.YearDay())); d > 35 {
			t.Errorf("%s: Aladhan Dhuhr %s local is %.0f min from solar noon — not Dublin local time?",
				day.Format("2006-01-02"), times.Dhuhr, d)
		}
		// Only compare consecutive days (the March and October groups are months apart).
		if !prev.IsZero() && day.Sub(prevDay) < 36*time.Hour {
			if step := absf(circularDiff(utcMinutes(dhuhr), utcMinutes(prev))); step > 6 {
				t.Errorf("%s: Aladhan Dhuhr jumped %.0f min in UTC since the previous day", day.Format("2006-01-02"), step)
			}
		}
		prev, prevDay = dhuhr, day
	}
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
