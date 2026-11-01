package prayer

import (
	"math"
	"testing"
	"time"
)

func mustDublin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// Every bundled timetable, run through the real load path, must have no
// one-hour jumps except exactly on the 2026 clock changes (29 Mar, 25 Oct).
func TestEmbeddedTimetablesHaveNoDSTGlitchesIn2026(t *testing.T) {
	loc := mustDublin(t)
	f, err := NewFetcher(t.TempDir()) // empty dir => embedded fallback is used
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"icci", "naas", "newbridge", "cork", "galway"} {
		t.Run(src, func(t *testing.T) {
			var prev float64
			for d := time.Date(2026, 1, 1, 12, 0, 0, 0, loc); d.Year() == 2026; d = d.AddDate(0, 0, 1) {
				got, err := f.loadFromFile(src, d)
				if err != nil {
					t.Fatalf("%s: %v", d.Format("2006-01-02"), err)
				}
				m, ok := clockToMinutes(got.Dhuhr)
				if !ok {
					t.Fatalf("%s: bad Dhuhr %q", d.Format("2006-01-02"), got.Dhuhr)
				}
				// Dhuhr must sit close to astronomical solar noon on the wall clock.
				if off := math.Abs(float64(m) - solarNoonLocalMinutes(d, loc)); off > 35 {
					t.Errorf("%s %s: Dhuhr %s is %.0f min from solar noon", src, d.Format("2006-01-02"), got.Dhuhr, off)
				}
				if d.YearDay() > 1 {
					step := float64(m) - prev
					switch d.Format("01-02") {
					case "03-29":
						if math.Abs(step-60) > 10 {
							t.Errorf("%s 29 Mar: step %.0f min, want ~+60", src, step)
						}
					case "10-25":
						if math.Abs(step+60) > 10 {
							t.Errorf("%s 25 Oct: step %.0f min, want ~-60", src, step)
						}
					default:
						if math.Abs(step) > 10 {
							t.Errorf("%s %s: unexpected %.0f min jump in Dhuhr", src, d.Format("2006-01-02"), step)
						}
					}
				}
				prev = float64(m)
			}
		})
	}
}

func TestGuardDST(t *testing.T) {
	loc := mustDublin(t)
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 9, 0, 0, 0, loc) }

	// Winter (GMT): ICCI-style data still on summer time is 1h late -> fix.
	late := Times{Fajr: "06:26", Dhuhr: "13:11", Asr: "15:38", Maghrib: "18:10", Isha: "19:46"}
	got, shift := guardDST(late, day(time.October, 26), loc)
	want := Times{Fajr: "05:26", Dhuhr: "12:11", Asr: "14:38", Maghrib: "17:10", Isha: "18:46"}
	if got != want || shift != -1 {
		t.Errorf("late: got %+v shift %d, want %+v shift -1", got, shift, want)
	}

	// Summer (IST): winter data is 1h early -> shift forward.
	early := Times{Fajr: "03:50", Dhuhr: "12:42", Asr: "16:09", Maghrib: "19:03", Isha: "20:30"}
	got, shift = guardDST(early, day(time.April, 2), loc)
	if got.Dhuhr != "13:42" || got.Fajr != "04:50" || shift != 1 {
		t.Errorf("early: got %+v shift %d", got, shift)
	}

	// Double-applied DST (2h late), as in the bundled Naas data on 25 Oct.
	dbl := Times{Fajr: "07:26", Dhuhr: "14:13", Asr: "16:41", Maghrib: "19:15", Isha: "20:50"}
	if got, shift = guardDST(dbl, day(time.October, 25), loc); got.Dhuhr != "12:13" || got.Fajr != "05:26" || shift != -2 {
		t.Errorf("double: got %+v shift %d", got, shift)
	}

	// Already correct data must be returned untouched.
	ok := Times{Fajr: "06:28", Dhuhr: "12:13", Asr: "14:40", Maghrib: "17:12", Isha: "18:48"}
	if got, shift = guardDST(ok, day(time.October, 26), loc); got != ok || shift != 0 {
		t.Errorf("correct data was modified: %+v shift %d", got, shift)
	}

	// Garbage / empty input must not panic or be rewritten.
	for _, bad := range []Times{{}, {Dhuhr: "xx:yy"}, {Dhuhr: "25:99"}} {
		if got, shift = guardDST(bad, day(time.October, 26), loc); got != bad || shift != 0 {
			t.Errorf("bad input %+v was changed to %+v", bad, got)
		}
	}
}
