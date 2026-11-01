package prayer

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"azan-agent/internal/config"
)

// The scheduler rebuilds each day's prayer timers from a daily cron job. If that
// job is skipped on any calendar day, no prayers are scheduled for that day.
// Clock changes are where cron implementations go wrong: in Ireland 01:00 does
// not exist on the spring-forward day (29 Mar 2026) and happens twice on the
// fall-back day (25 Oct 2026).
//
// Property under test: starting from any time on day D-1, the next refresh falls
// on day D, exactly once, in the early hours — for every day of the year.
func TestDailyRefreshFiresEveryCalendarDayIncludingDSTChanges(t *testing.T) {
	loc := mustDublin(t)
	c := cron.New(cron.WithLocation(loc))
	if _, err := c.AddFunc(dailyRefreshSpec, func() {}); err != nil {
		t.Fatalf("invalid refresh spec %q: %v", dailyRefreshSpec, err)
	}
	sched := c.Entries()[0].Schedule

	for day := time.Date(2026, 1, 1, 0, 0, 0, 0, loc); day.Year() == 2026; day = day.AddDate(0, 0, 1) {
		// Start just after the previous refresh window, at noon the day before.
		from := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, -1)
		next := sched.Next(from).In(loc)

		if next.YearDay() != day.YearDay() || next.Year() != day.Year() {
			t.Errorf("after %s the next refresh is %s — the refresh for %s is skipped",
				from.Format("2006-01-02 15:04 MST"), next.Format("2006-01-02 15:04 MST"), day.Format("2006-01-02"))
			continue
		}
		if h := next.Hour(); h > 3 {
			t.Errorf("refresh on %s runs at %s, expected the small hours", day.Format("2006-01-02"), next.Format("15:04 MST"))
		}
		// And it must not fire twice on the same day.
		if again := sched.Next(next).In(loc); again.YearDay() == next.YearDay() {
			t.Errorf("refresh fires twice on %s (%s and %s)", day.Format("2006-01-02"),
				next.Format("15:04 MST"), again.Format("15:04 MST"))
		}
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

func utcMinutes(t time.Time) float64 { return float64(t.Hour()*60+t.Minute()) + float64(t.Second())/60 }

func TestScheduleIsConsistentAllYearForEverySource(t *testing.T) {
	loc := mustDublin(t)
	order := []string{"Fajr", "Dhuhr", "Asr", "Maghrib", "Isha"}
	// Largest plausible change from one day to the next, in minutes, in UTC. A
	// daylight-saving error shows up as a 60-minute step, far above these.
	maxStep := map[string]float64{"Dhuhr": 6, "Asr": 12, "Maghrib": 12, "Fajr": 25, "Isha": 25}

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

			var prev map[string]time.Time
			for day := time.Date(2026, 1, 1, 0, 0, 0, 0, loc); day.Year() == 2026; day = day.AddDate(0, 0, 1) {
				got := scheduleFor(t, src, day, dir)
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
						step := utcMinutes(got[p]) - utcMinutes(prev[p])
						if math.Abs(step) > maxStep[p] {
							t.Errorf("%s: %s jumped %+.0f min in UTC since yesterday (%s -> %s)", date, p, step,
								prev[p].Format("15:04"), got[p].Format("15:04"))
						}
					}
				}
				prev = got
			}
		})
	}
}
