//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// maiPage mimics the structure of maionline.ie's home page for the given day.
func maiPage(day time.Time, t [5]string) string {
	suffix := "th"
	switch d := day.Day(); {
	case d%100 >= 11 && d%100 <= 13:
	case d%10 == 1:
		suffix = "st"
	case d%10 == 2:
		suffix = "nd"
	case d%10 == 3:
		suffix = "rd"
	}
	cell := func(label, hhmm string) string {
		return fmt.Sprintf("<td class='prayer-cell'><div class='prayer-label %s'>%s</div><div class='prayer-time'>"+
			"<span class='prayer-passed-hour'>%s</span><span class='prayer-separator'>:</span>"+
			"<span class='prayer-passed-minute'>%s</span></div></td>", label, label, hhmm[:2], hhmm[3:])
	}
	return fmt.Sprintf("<html><body><table class=\"timetable\"><tr><td class='date-time'><span class='date'>%s, %d%s %s<br />th  </span></td>%s%s%s%s%s%s</tr></table></body></html>",
		day.Format("Mon"), day.Day(), suffix, day.Format("January 2006"),
		cell("fajr", t[0]), cell("shurooq", "07:17"), cell("dhuhr", t[1]), cell("asr", t[2]), cell("maghrib", t[3]), cell("isha", t[4]))
}

// With the MAI source selected, the agent reads the (fake) MAI site, schedules
// from it, and keeps working if the site later goes down.
func TestMAISourceEndToEnd(t *testing.T) {
	t.Parallel()
	_, off := time.Now().In(dublin).Zone()
	times := [5]string{"03:10", fmt.Sprintf("%02d:25", 12+off/3600), "15:00", "18:00", "20:00"}
	day := time.Now().In(dublin)

	var down atomic.Bool
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, maiPage(time.Now().In(dublin), times))
	}))
	defer site.Close()

	sp := newSpeaker(t)
	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
	writeConfigAt(t, cfgFile, webPort, "mai", "all")
	startAgent(t, home, cfgFile, sp, withCredentials(testUser, testPass), withEnv("AZAN_MAI_URL="+site.URL))

	c := newClient(t, webPort)
	c.mustLogin()

	var pt struct {
		Success     bool              `json:"success"`
		PrayerTimes map[string]string `json:"prayer_times"`
		Location    string            `json:"location"`
		Date        string            `json:"date"`
	}
	c.getJSON("/api/prayer-times", &pt)
	want := map[string]string{"Fajr": times[0], "Dhuhr": times[1], "Asr": times[2], "Maghrib": times[3], "Isha": times[4]}
	if !pt.Success || pt.Location != "mai" || pt.Date != day.Format("2006-01-02") {
		t.Fatalf("prayer-times = %+v", pt)
	}
	for p, w := range want {
		if pt.PrayerTimes[p] != w {
			t.Errorf("%s = %q, want %q (from the MAI page)", p, pt.PrayerTimes[p], w)
		}
	}
	var st struct{ Jobs []struct{ Prayer string } }
	c.getJSON("/api/scheduler-status", &st)
	if len(st.Jobs) != 5 {
		t.Errorf("the scheduler has %d jobs from the MAI times, want 5", len(st.Jobs))
	}

	// The site goes down: forcing a refresh must still give today's times.
	down.Store(true)
	pt.PrayerTimes = nil
	c.getJSON("/api/prayer-times?force=true", &pt)
	if !pt.Success || pt.PrayerTimes["Dhuhr"] != times[1] {
		t.Errorf("with the MAI site down, prayer-times = %+v; want the times recorded earlier today", pt)
	}
}
