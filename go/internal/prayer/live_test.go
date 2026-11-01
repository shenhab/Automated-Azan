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
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// icci is not listed: islamireland.ie no longer publishes a timetable (see
// errICCIDiscontinued). The live Dublin source is MAI, tested below.
var liveSources = []string{"naas", "newbridge", "cork", "galway"}

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
				rawTimes, err := f.extractNaas(raw, day)
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

// MAI publishes today's times only. The live page must still be readable, be for
// today, and give a plausible day.
func TestLiveMAITodayParsesAndIsPlausible(t *testing.T) {
	t.Setenv("AZAN_MAI_URL", "") // the real site
	loc := mustDublin(t)
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc)

	f, err := NewFetcher(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// What the site says, before the agent's own sanity check.
	req, _ := http.NewRequest("GET", maiURLDefault, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/124.0.0.0 Safari/537.36")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("maionline.ie unreachable: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	pageDate, raw, err := parseMAIPage(body)
	if err != nil {
		t.Fatalf("the live MAI page can no longer be read (its layout changed?): %v", err)
	}
	if pageDate.Format("2006-01-02") != day.Format("2006-01-02") {
		t.Fatalf("the live MAI page is for %s, not today", pageDate.Format("2006-01-02"))
	}
	if _, shift := correctDST(raw, day, loc); shift != 0 {
		t.Logf("WARNING: MAI published today's times %+v an hour out; the agent corrects this", raw)
	}

	got, err := f.Fetch("mai", day, true)
	if err != nil {
		t.Fatal(err)
	}
	h, m, _ := parseHHMM(got.Dhuhr)
	if off := absf(float64(h*60+m) - solarNoonLocalMinutes(day, loc)); off > 35 {
		t.Errorf("Dhuhr %s is %.0f min from solar noon", got.Dhuhr, off)
	}
}
