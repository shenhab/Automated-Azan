package prayer

import (
	"strings"
	"testing"
	"time"
)

func TestParseHHMM(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantHour   int
		wantMinute int
		wantErr    bool
	}{
		// valid
		{"valid mid morning", "05:30", 5, 30, false},
		{"valid end of day", "23:59", 23, 59, false},
		{"valid midnight", "00:00", 0, 0, false},

		// malformed
		{"non-numeric", "abc", 0, 0, true},
		{"empty string", "", 0, 0, true},
		{"wrong separator", "5-30", 0, 0, true},

		// out of range
		{"hour out of range", "24:00", 0, 0, true},
		{"minute out of range", "12:60", 0, 0, true},
		{"negative hour", "-1:30", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHour, gotMinute, err := parseHHMM(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseHHMM(%q) = (%d, %d, nil); want error", tt.input, gotHour, gotMinute)
				}
				// parseHHMM must own every rejection path (malformed and
				// out-of-range alike) rather than passing through whatever
				// fmt.Sscanf happens to produce, so callers get a single,
				// recognizable validation error and never a partially
				// populated hour/minute.
				if !strings.Contains(err.Error(), "parseHHMM") {
					t.Fatalf("parseHHMM(%q) returned error %q; want an error produced by parseHHMM's own validation", tt.input, err.Error())
				}
				if gotHour != 0 || gotMinute != 0 {
					t.Errorf("parseHHMM(%q) on error = (%d, %d); want zero values", tt.input, gotHour, gotMinute)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHHMM(%q) returned unexpected error: %v", tt.input, err)
			}
			if gotHour != tt.wantHour || gotMinute != tt.wantMinute {
				t.Errorf("parseHHMM(%q) = (%d, %d); want (%d, %d)", tt.input, gotHour, gotMinute, tt.wantHour, tt.wantMinute)
			}
		})
	}
}

// futureHHMM returns an HH:MM string guaranteed to be later than now on the
// same calendar day (nudges the hour, or the minute when already at 23:xx),
// so NextPrayer's same-day time.Date reconstruction treats it as upcoming.
func futureHHMM(now time.Time) string {
	h, m := now.Hour(), now.Minute()
	if h < 23 {
		h++
	} else if m < 59 {
		m++
	}
	return fmtHHMM(h, m)
}

// pastHHMM returns an HH:MM string guaranteed to be earlier than now on the
// same calendar day.
func pastHHMM(now time.Time) string {
	h, m := now.Hour(), now.Minute()
	if h > 0 {
		h--
	} else if m > 0 {
		m--
	}
	return fmtHHMM(h, m)
}

func fmtHHMM(h, m int) string {
	return time.Date(0, 1, 1, h, m, 0, 0, time.UTC).Format("15:04")
}

func TestSchedulerNextPrayer(t *testing.T) {
	loc := time.UTC

	t.Run("no upcoming jobs", func(t *testing.T) {
		s := &Scheduler{
			tz: loc,
			todayJobs: []JobStatus{
				{Prayer: "Fajr", Time: pastHHMM(time.Now().In(loc)), Status: "done"},
				{Prayer: "Dhuhr", Time: pastHHMM(time.Now().In(loc)), Status: "skipped"},
			},
		}

		name, at, ok := s.NextPrayer()

		if ok {
			t.Fatalf("NextPrayer() ok = true; want false, got name=%q at=%v", name, at)
		}
		if name != "" {
			t.Errorf("NextPrayer() name = %q; want empty", name)
		}
		if !at.IsZero() {
			t.Errorf("NextPrayer() at = %v; want zero time", at)
		}
	})

	t.Run("single upcoming job in future", func(t *testing.T) {
		now := time.Now().In(loc)
		future := futureHHMM(now)
		s := &Scheduler{
			tz: loc,
			todayJobs: []JobStatus{
				{Prayer: "Isha", Time: future, Status: "upcoming"},
			},
		}

		name, at, ok := s.NextPrayer()

		if !ok {
			t.Fatalf("NextPrayer() ok = false; want true")
		}
		if name != "Isha" {
			t.Errorf("NextPrayer() name = %q; want %q", name, "Isha")
		}
		if at.Format("15:04") != future {
			t.Errorf("NextPrayer() at = %s; want time-of-day %s", at.Format("15:04"), future)
		}
	})

	t.Run("earliest-in-list past job skipped in favor of later future job", func(t *testing.T) {
		now := time.Now().In(loc)
		past := pastHHMM(now)
		future := futureHHMM(now)
		s := &Scheduler{
			tz: loc,
			todayJobs: []JobStatus{
				// Listed first but its clock time has already passed; the
				// "upcoming" status is stale and NextPrayer must skip it.
				{Prayer: "Fajr", Time: past, Status: "upcoming"},
				{Prayer: "Maghrib", Time: future, Status: "upcoming"},
			},
		}

		name, at, ok := s.NextPrayer()

		if !ok {
			t.Fatalf("NextPrayer() ok = false; want true")
		}
		if name != "Maghrib" {
			t.Errorf("NextPrayer() name = %q; want %q (Fajr's stale past time should be skipped)", name, "Maghrib")
		}
		if at.Format("15:04") != future {
			t.Errorf("NextPrayer() at = %s; want time-of-day %s", at.Format("15:04"), future)
		}
	})

	t.Run("malformed job time is skipped rather than erroring", func(t *testing.T) {
		now := time.Now().In(loc)
		future := futureHHMM(now)
		s := &Scheduler{
			tz: loc,
			todayJobs: []JobStatus{
				{Prayer: "Asr", Time: "not-a-time", Status: "upcoming"},
				{Prayer: "Isha", Time: future, Status: "upcoming"},
			},
		}

		name, at, ok := s.NextPrayer()

		if !ok {
			t.Fatalf("NextPrayer() ok = false; want true")
		}
		if name != "Isha" {
			t.Errorf("NextPrayer() name = %q; want %q (malformed Asr time should be skipped)", name, "Isha")
		}
		if at.Format("15:04") != future {
			t.Errorf("NextPrayer() at = %s; want time-of-day %s", at.Format("15:04"), future)
		}
	})
}
