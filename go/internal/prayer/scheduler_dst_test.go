package prayer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"azan-agent/internal/config"
)

// The scheduler rebuilds each day's prayer timers from a daily refresh. If that
// refresh is skipped on any calendar day, no prayers are scheduled for that day.
// Clock changes are where this goes wrong: in Ireland 01:00 does not exist on the
// spring-forward day (29 Mar 2026) and happens twice on the fall-back day
// (25 Oct 2026).
//
// Property under test: starting from any time on day D-1, the next refresh falls
// on day D, exactly once, in the early hours — for every day of the year.
func TestDailyRefreshFiresEveryCalendarDayIncludingDSTChanges(t *testing.T) {
	loc := mustDublin(t)

	for day := time.Date(2026, 1, 1, 0, 0, 0, 0, loc); day.Year() == 2026; day = day.AddDate(0, 0, 1) {
		// Start just after the previous refresh window, at noon the day before.
		from := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, -1)
		next := nextDailyRefresh(from, loc).In(loc)

		if next.YearDay() != day.YearDay() || next.Year() != day.Year() {
			t.Errorf("after %s the next refresh is %s — the refresh for %s is skipped",
				from.Format("2006-01-02 15:04 MST"), next.Format("2006-01-02 15:04 MST"), day.Format("2006-01-02"))
			continue
		}
		if h := next.Hour(); h > 3 {
			t.Errorf("refresh on %s runs at %s, expected the small hours", day.Format("2006-01-02"), next.Format("15:04 MST"))
		}
		// And it must not fire twice on the same day.
		if again := nextDailyRefresh(next, loc).In(loc); again.YearDay() == next.YearDay() {
			t.Errorf("refresh fires twice on %s (%s and %s)", day.Format("2006-01-02"),
				next.Format("15:04 MST"), again.Format("15:04 MST"))
		}
	}
}

// The real refresh loop, driven by a clock set just before 00:05 on each DST
// night, must rebuild the schedule exactly once and then wait for the next day.
func TestRefreshLoopRunsOncePerDSTNight(t *testing.T) {
	loc := mustDublin(t)
	for _, tc := range []struct {
		name  string
		start time.Time // 0.5 s before the 00:05 refresh
	}{
		{"spring-forward night (29 Mar 2026)", time.Date(2026, 3, 29, 0, 4, 59, 500_000_000, loc)},
		{"fall-back night (25 Oct 2026)", time.Date(2026, 10, 25, 0, 4, 59, 500_000_000, loc)},
		{"an ordinary night", time.Date(2026, 6, 10, 0, 4, 59, 500_000_000, loc)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := NewFetcher(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			writeNaasFormat(t, f, "naas", [6]string{"03:00", "05:00", "12:25", "15:00", "18:00", "20:00"})
			cfg := &config.Config{}
			cfg.Prayer.Location = "naas"
			s := NewScheduler(cfg, f, func(string, string) error { return nil }, nil, nil, nil, nil, nil, nil)

			offset := time.Until(tc.start) // the clock starts at tc.start and runs in real time
			s.now = func() time.Time { return time.Now().Add(offset) }
			refreshed := make(chan struct{}, 4)
			s.timeSync = func() {}
			s.afterRefresh = func() { refreshed <- struct{}{} }

			if err := s.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Stop)

			select {
			case <-refreshed:
			case <-time.After(5 * time.Second):
				t.Fatal("the 00:05 refresh never ran")
			}
			select {
			case <-refreshed:
				t.Error("the refresh ran twice")
			case <-time.After(1500 * time.Millisecond):
			}
		})
	}
}

// ── whole-year, every-source schedule check ──────────────────────────────────

// noonUTC is the astronomical solar noon at Dublin's longitude, in minutes after
// 00:00 UTC, using the NOAA equation-of-time approximation. It deliberately has
// no timezone in it, so it is an oracle that is independent of any DST logic.
func noonUTC(yearDay int) float64 {
	gamma := 2 * math.Pi / 365 * float64(yearDay-1)
	eq := 229.18 * (0.000075 + 0.001868*math.Cos(gamma) - 0.032077*math.Sin(gamma) -
		0.014615*math.Cos(2*gamma) - 0.040849*math.Sin(2*gamma))
	return 720 - 4*dublinLongitude - eq
}

// scheduleFor runs the real scheduler for the given local date (at 00:10, so all
// five prayers are still upcoming) and returns each prayer's absolute instant.
func scheduleFor(t *testing.T, loc string, day time.Time, dataDir string) map[string]time.Time {
	t.Helper()
	f, err := NewFetcher(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Prayer.Location = loc
	cfg.Prayer.Enabled = config.PrayerEnabledConfig{Fajr: true, Dhuhr: true, Asr: true, Maghrib: true, Isha: true}
	s := NewScheduler(cfg, f, func(string, string) error { return nil }, nil, nil, nil, nil, nil, nil)
	now := time.Date(day.Year(), day.Month(), day.Day(), 0, 10, 0, 0, f.Timezone())
	s.now = func() time.Time { return now }

	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.cancelTimersLocked()
	if err := s.scheduleTodayLocked(); err != nil {
		t.Fatalf("%s %s: %v", loc, day.Format("2006-01-02"), err)
	}
	out := map[string]time.Time{}
	for _, j := range s.todayJobs {
		at, err := time.Parse(time.RFC3339, j.NextRunAt)
		if err != nil {
			t.Fatalf("%s %s %s: bad next_run %q", loc, day.Format("2006-01-02"), j.Prayer, j.NextRunAt)
		}
		out[j.Prayer] = at.UTC()
	}
	return out
}

// circularDiff is a-b on a 24-hour clock, in (-720, 720]: 00:10 minus 23:50 is
// +20, not -1420. (Minute-of-day in UTC wraps at midnight, which a prayer that
// legitimately falls after midnight will cross.)
func circularDiff(a, b float64) float64 {
	d := math.Mod(a-b+720+1440, 1440) - 720
	return d
}

func utcMinutes(t time.Time) float64 { return float64(t.Hour()*60+t.Minute()) + float64(t.Second())/60 }

// assertConsistentYear runs the real scheduler for every day of `year` against
// the timetable already present in dataDir and checks, without trusting any
// timezone logic in the code under test, that the schedule is sane: Dhuhr is at
// solar noon in UTC, the prayers are in order, and nothing jumps by an hour from
// one day to the next. Used for both the bundled and the live timetables.
func assertConsistentYear(t *testing.T, src, dataDir string, year int) {
	t.Helper()
	loc := mustDublin(t)
	order := []string{"Fajr", "Dhuhr", "Asr", "Maghrib", "Isha"}
	// Largest plausible change from one day to the next, in minutes, in UTC. A
	// daylight-saving error shows up as a 60-minute step, far above these.
	maxStep := map[string]float64{"Dhuhr": 6, "Asr": 12, "Maghrib": 12, "Fajr": 25, "Isha": 25}

	var prev map[string]time.Time
	for day := time.Date(year, 1, 1, 0, 0, 0, 0, loc); day.Year() == year; day = day.AddDate(0, 0, 1) {
		got := scheduleFor(t, src, day, dataDir)
		date := day.Format("2006-01-02")
		for _, p := range order {
			if _, ok := got[p]; !ok {
				t.Fatalf("%s: no %s scheduled", date, p)
			}
		}

		// 1. Dhuhr is at solar noon in absolute (UTC) terms.
		if d := math.Abs(utcMinutes(got["Dhuhr"]) - noonUTC(day.YearDay())); d > 35 {
			t.Errorf("%s: Dhuhr %s UTC is %.0f min from solar noon", date, got["Dhuhr"].Format("15:04"), d)
		}
		// 2. Prayers are in order.
		for i := 1; i < len(order); i++ {
			if !got[order[i]].After(got[order[i-1]]) {
				t.Errorf("%s: %s (%s) is not after %s (%s)", date, order[i], got[order[i]].Format("15:04"),
					order[i-1], got[order[i-1]].Format("15:04"))
			}
		}
		// 3. No sudden jumps from yesterday (the signature of a DST error).
		if prev != nil {
			for _, p := range order {
				step := circularDiff(utcMinutes(got[p]), utcMinutes(prev[p]))
				if math.Abs(step) > maxStep[p] {
					t.Errorf("%s: %s jumped %+.0f min in UTC since yesterday (%s -> %s)", date, p, step,
						prev[p].Format("15:04"), got[p].Format("15:04"))
				}
			}
		}
		prev = got
	}
}

func TestScheduleIsConsistentAllYearForEverySource(t *testing.T) {
	for _, src := range []string{"icci", "naas", "newbridge", "cork", "galway"} {
		src := src
		t.Run(src, func(t *testing.T) {
			// Use the bundled timetable as a fresh local file so no network is touched.
			dir := t.TempDir()
			seed, err := NewFetcher(dir)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := seed.embeddedTimetable(src)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(seed.timetablePath(src), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			assertConsistentYear(t, src, dir, 2026)
		})
	}
}

// ── prayers that fall after midnight ─────────────────────────────────────────

// At Ireland's latitude, Isha in late spring/summer is after midnight (the live
// Cork timetable has e.g. 00:17 and 01:44). A timetable row lists the Isha that
// ends that evening, so "00:17" in the 20 June row means 00:17 on 21 June.

// writeNaasFormat writes a full-year Naas-format timetable with the same times
// every day.
func writeNaasFormat(t *testing.T, f *Fetcher, loc string, times [6]string) {
	t.Helper()
	var months []map[string][]string
	for m := 0; m < 12; m++ {
		days := map[string][]string{}
		for d := 1; d <= 31; d++ {
			days[fmt.Sprint(d)] = times[:]
		}
		months = append(months, days)
	}
	data, err := json.Marshal(months)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.timetablePath(loc), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func scheduleAt(t *testing.T, ishaTime string, now time.Time) *Scheduler {
	t.Helper()
	f, err := NewFetcher(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Fajr, Sunrise, Dhuhr, Asr, Maghrib, Isha — a plausible June day in Cork.
	writeNaasFormat(t, f, "cork", [6]string{"02:57", "05:00", "13:34", "17:48", "21:35", ishaTime})
	cfg := &config.Config{}
	cfg.Prayer.Location = "cork"
	cfg.Prayer.Enabled = config.PrayerEnabledConfig{Fajr: true, Dhuhr: true, Asr: true, Maghrib: true, Isha: true}
	s := NewScheduler(cfg, f, func(string, string) error { return nil }, nil, nil, nil, nil, nil, nil)
	s.now = func() time.Time { return now }
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Cleanup(func() { s.mu.Lock(); s.cancelTimersLocked(); s.mu.Unlock() })
	if err := s.scheduleTodayLocked(); err != nil {
		t.Fatal(err)
	}
	return s
}

func ishaInstants(s *Scheduler) (upcoming []string) {
	loc := s.tz
	for _, j := range s.todayJobs {
		if j.Prayer == "Isha" && j.NextRunAt != "" {
			at, _ := time.Parse(time.RFC3339, j.NextRunAt)
			upcoming = append(upcoming, at.In(loc).Format("2006-01-02 15:04"))
		}
	}
	return upcoming
}

func TestIshaAfterMidnightIsScheduledOnTheFollowingDay(t *testing.T) {
	loc := mustDublin(t)
	// 20 June, just after the daily refresh. Tonight's Isha is at 00:17 on the
	// 21st; last night's Isha (listed under the 19th) is still to come at 00:17
	// today. Both are real prayers that must be scheduled.
	s := scheduleAt(t, "00:17", time.Date(2026, 6, 20, 0, 5, 0, 0, loc))
	got := ishaInstants(s)
	want := []string{"2026-06-20 00:17", "2026-06-21 00:17"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Isha scheduled at %v, want %v", got, want)
	}
}

func TestIshaAfterMidnightAlreadyPassedIsNotRepeated(t *testing.T) {
	loc := mustDublin(t)
	// 00:30 on the 20th: last night's 00:17 Isha has already happened; only
	// tonight's (00:17 on the 21st) remains.
	s := scheduleAt(t, "00:17", time.Date(2026, 6, 20, 0, 30, 0, 0, loc))
	got := ishaInstants(s)
	want := []string{"2026-06-21 00:17"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Isha scheduled at %v, want %v", got, want)
	}
}

func TestNextPrayerAfterMaghribIsTonightsAfterMidnightIsha(t *testing.T) {
	loc := mustDublin(t)
	s := scheduleAt(t, "00:17", time.Date(2026, 6, 20, 21, 40, 0, 0, loc)) // after Maghrib
	name, at, ok := s.NextPrayer()
	if !ok || name != "Isha" {
		t.Fatalf("next prayer = %q ok=%v, want Isha", name, ok)
	}
	if want := time.Date(2026, 6, 21, 0, 17, 0, 0, loc); !at.Equal(want) {
		t.Errorf("next prayer at %s, want %s", at.Format("2006-01-02 15:04"), want.Format("2006-01-02 15:04"))
	}
}

func TestOrdinaryEveningIshaIsUnchanged(t *testing.T) {
	loc := mustDublin(t)
	s := scheduleAt(t, "22:48", time.Date(2026, 5, 1, 0, 5, 0, 0, loc))
	got := ishaInstants(s)
	want := []string{"2026-05-01 22:48"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Isha scheduled at %v, want %v", got, want)
	}
}

// The DST sanity guard must also handle a timetable whose Isha is after
// midnight: shifting everything an hour must wrap around midnight, not give up.
func TestGuardDSTHandlesIshaAfterMidnight(t *testing.T) {
	loc := mustDublin(t)
	day := time.Date(2026, 6, 20, 9, 0, 0, 0, loc) // IST: solar noon ≈ 13:2x local
	// Whole row one hour early (winter times on a summer day).
	early := Times{Fajr: "01:58", Dhuhr: "12:34", Asr: "16:48", Maghrib: "20:35", Isha: "23:17"}
	got, shift := guardDST(early, day, loc)
	if shift != 1 || got.Dhuhr != "13:34" || got.Isha != "00:17" {
		t.Errorf("early row: got %+v shift %d, want Dhuhr 13:34 and Isha 00:17 (wrapped past midnight), shift 1", got, shift)
	}
	// Whole row one hour late with Isha just after midnight: shifting back must
	// wrap Isha to the previous evening instead of leaving the row uncorrected.
	late := Times{Fajr: "03:58", Dhuhr: "14:34", Asr: "18:48", Maghrib: "22:35", Isha: "00:17"}
	got, shift = guardDST(late, day, loc)
	if shift != -1 || got.Dhuhr != "13:34" || got.Isha != "23:17" {
		t.Errorf("late row: got %+v shift %d, want Dhuhr 13:34 and Isha 23:17, shift -1", got, shift)
	}
}
