package prayer

import (
	_ "embed"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The Muslim Association of Ireland (maionline.ie) publishes only today's times,
// as a table on its home page. The agent reads that page each day and keeps what
// it has seen.

//go:embed testdata/mai_timetable_block_2026-11-01.html
var maiRealBlock string

func ordinal(d int) string {
	switch {
	case d%100 >= 11 && d%100 <= 13:
		return "th"
	case d%10 == 1:
		return "st"
	case d%10 == 2:
		return "nd"
	case d%10 == 3:
		return "rd"
	}
	return "th"
}

// maiPage builds a page with the same structure as the real one.
func maiPage(date time.Time, ts Times) string {
	cell := func(label, hhmm string) string {
		return fmt.Sprintf("<td class='prayer-cell'><div class='prayer-label %s'>%s</div><div class='prayer-time'>"+
			"<span class='prayer-passed-hour'>%s</span><span class='prayer-separator'>:</span>"+
			"<span class='prayer-passed-minute'>%s</span></div></td>", label, label, hhmm[:2], hhmm[3:])
	}
	return fmt.Sprintf("<html><body><table class=\"timetable\"><tr><td class='date-time'><span class='date'>%s, %d%s %s<br />th  </span></td>%s%s%s%s%s%s</tr></table></body></html>",
		date.Format("Mon"), date.Day(), ordinal(date.Day()), date.Format("January 2006"),
		cell("fajr", ts.Fajr), cell("shurooq", "07:17"), cell("dhuhr", ts.Dhuhr), cell("asr", ts.Asr), cell("maghrib", ts.Maghrib), cell("isha", ts.Isha))
}

// plausibleTimes returns a realistic day whose Dhuhr is at solar noon on the
// Dublin wall clock for the given date.
func plausibleTimes(t *testing.T, date time.Time) Times {
	t.Helper()
	noon := int(math.Round(solarNoonLocalMinutes(date, mustDublin(t)))) + 3
	hm := func(offsetMin int) string {
		m := ((noon+offsetMin)%1440 + 1440) % 1440
		return fmt.Sprintf("%02d:%02d", m/60, m%60)
	}
	return Times{Fajr: hm(-420), Dhuhr: hm(0), Asr: hm(140), Maghrib: hm(240), Isha: hm(330)}
}

type maiServer struct {
	*httptest.Server
	hits atomic.Int32
	page atomic.Value // string
	fail atomic.Bool
}

func newMAIServer(t *testing.T, page string) *maiServer {
	t.Helper()
	m := &maiServer{}
	m.page.Store(page)
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.hits.Add(1)
		if m.fail.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, m.page.Load().(string))
	}))
	t.Cleanup(m.Close)
	t.Setenv("AZAN_MAI_URL", m.URL)
	return m
}

func today(t *testing.T) time.Time {
	t.Helper()
	n := time.Now().In(mustDublin(t))
	return time.Date(n.Year(), n.Month(), n.Day(), 12, 0, 0, 0, n.Location())
}

func TestParseMAIPageFromTheRealSite(t *testing.T) {
	date, got, err := parseMAIPage([]byte(maiRealBlock))
	if err != nil {
		t.Fatal(err)
	}
	if date.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("date = %s, want 2026-11-01", date.Format("2006-01-02"))
	}
	want := Times{Fajr: "05:34", Dhuhr: "12:09", Asr: "14:28", Maghrib: "16:57", Isha: "18:34"}
	if got != want {
		t.Errorf("times = %+v, want %+v", got, want)
	}
}

func TestParseMAIPageRejectsPagesWhoseFormatChanged(t *testing.T) {
	d := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	good := Times{Fajr: "05:34", Dhuhr: "12:09", Asr: "14:28", Maghrib: "16:57", Isha: "18:34"}
	for name, page := range map[string]string{
		"empty page":           "",
		"not the timetable":    "<html><body>Under maintenance</body></html>",
		"an impossible minute": maiPage(d, Times{Fajr: "05:99", Dhuhr: "12:09", Asr: "14:28", Maghrib: "16:57", Isha: "18:34"}),
		"Dhuhr before Fajr":    maiPage(d, Times{Fajr: "13:00", Dhuhr: "12:09", Asr: "14:28", Maghrib: "16:57", Isha: "18:34"}),
		"Maghrib before Asr":   maiPage(d, Times{Fajr: "05:34", Dhuhr: "12:09", Asr: "17:30", Maghrib: "16:57", Isha: "18:34"}),
		"no date on the page":  strings.Replace(maiPage(d, good), "<span class='date'>Sun, 1st November 2026<br />th  </span>", "", 1),
	} {
		if _, _, err := parseMAIPage([]byte(page)); err == nil {
			t.Errorf("%s: accepted, but the page cannot be trusted", name)
		}
	}
	// An Isha after midnight is legitimate in summer and must be accepted.
	if _, ts, err := parseMAIPage([]byte(maiPage(d, Times{Fajr: "02:40", Dhuhr: "13:35", Asr: "17:55", Maghrib: "21:50", Isha: "00:40"}))); err != nil || ts.Isha != "00:40" {
		t.Errorf("an after-midnight Isha was rejected: %v %+v", err, ts)
	}
}

func TestMAIFetchReturnsTodaysTimesAndCachesThem(t *testing.T) {
	day := today(t)
	want := plausibleTimes(t, day)
	srv := newMAIServer(t, maiPage(day, want))

	dir := t.TempDir()
	f, err := NewFetcher(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Fetch("mai", day, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("times = %+v, want %+v", got, want)
	}
	if _, err := f.Fetch("mai", day, false); err != nil || srv.hits.Load() != 1 {
		t.Errorf("a second fetch hit the site again (%d hits, err %v)", srv.hits.Load(), err)
	}
	if _, err := os.Stat(dir + "/mai_times.json"); err != nil {
		t.Errorf("the day was not recorded for later: %v", err)
	}
}

func TestMAIStillWorksWhenTheSiteIsDownAfterTodayWasFetched(t *testing.T) {
	day := today(t)
	want := plausibleTimes(t, day)
	srv := newMAIServer(t, maiPage(day, want))
	f, _ := NewFetcher(t.TempDir())
	if _, err := f.Fetch("mai", day, false); err != nil {
		t.Fatal(err)
	}
	srv.fail.Store(true)
	f.ClearCache() // as after a restart's in-memory loss or the hourly expiry
	got, err := f.Fetch("mai", day, true)
	if err != nil || got != want {
		t.Errorf("with the site down got %+v, err %v; want today's recorded times %+v", got, err, want)
	}
}

func TestMAIUsesTheMostRecentRecordedDayIfTheSiteHasNeverAnsweredToday(t *testing.T) {
	day := today(t)
	srv := newMAIServer(t, "")
	srv.fail.Store(true)
	f, _ := NewFetcher(t.TempDir())
	yesterday, threeAgo := day.AddDate(0, 0, -1), day.AddDate(0, 0, -3)
	f.recordMAI(t, threeAgo, plausibleTimes(t, threeAgo))
	f.recordMAI(t, yesterday, plausibleTimes(t, yesterday))

	got, err := f.Fetch("mai", day, false)
	if err != nil {
		t.Fatalf("no fallback although yesterday's times are recorded: %v", err)
	}
	if got != plausibleTimes(t, yesterday) {
		t.Errorf("fell back to %+v, want yesterday's %+v", got, plausibleTimes(t, yesterday))
	}
}

func TestMAIDoesNotGuessFromOldData(t *testing.T) {
	day := today(t)
	srv := newMAIServer(t, "")
	srv.fail.Store(true)
	f, _ := NewFetcher(t.TempDir())
	old := day.AddDate(0, 0, -10)
	f.recordMAI(t, old, plausibleTimes(t, old))
	if got, err := f.Fetch("mai", day, false); err == nil {
		t.Errorf("used 10-day-old times %+v instead of reporting that today's are unavailable", got)
	}
}

func TestMAIRefusesAPageForAnotherDay(t *testing.T) {
	day := today(t)
	yesterday := day.AddDate(0, 0, -1)
	newMAIServer(t, maiPage(yesterday, plausibleTimes(t, yesterday))) // a stale cached copy of the site
	f, _ := NewFetcher(t.TempDir())
	if got, err := f.Fetch("mai", day, false); err == nil {
		t.Errorf("accepted yesterday's page as today's: %+v", got)
	}
	if _, ok := f.readMAICache()[day.Format("2006-01-02")]; ok {
		t.Error("yesterday's page was recorded as today's")
	}
}

func TestMAIAppliesTheDaylightSavingSanityCheck(t *testing.T) {
	day := today(t)
	right := plausibleTimes(t, day)
	late := Times{} // the same day, an hour late (a wrong clock on the site)
	shift := func(hhmm string) string {
		h, m, _ := parseHHMM(hhmm)
		return fmt.Sprintf("%02d:%02d", (h+1)%24, m)
	}
	late = Times{Fajr: shift(right.Fajr), Dhuhr: shift(right.Dhuhr), Asr: shift(right.Asr), Maghrib: shift(right.Maghrib), Isha: shift(right.Isha)}
	newMAIServer(t, maiPage(day, late))
	f, _ := NewFetcher(t.TempDir())
	got, err := f.Fetch("mai", day, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != right {
		t.Errorf("an hour-late page gave %+v, want it corrected to %+v", got, right)
	}
}

func TestMAIYesterdayComesFromTheRecordAndTheFutureIsRefused(t *testing.T) {
	day := today(t)
	newMAIServer(t, maiPage(day, plausibleTimes(t, day)))
	f, _ := NewFetcher(t.TempDir())
	yesterday := day.AddDate(0, 0, -1)
	f.recordMAI(t, yesterday, plausibleTimes(t, yesterday))

	if got, err := f.Fetch("mai", yesterday, false); err != nil || got != plausibleTimes(t, yesterday) {
		t.Errorf("yesterday = %+v, %v", got, err)
	}
	if got, err := f.Fetch("mai", day.AddDate(0, 0, 1), false); err == nil {
		t.Errorf("invented tomorrow's times: %+v", got)
	}
}

// recordMAI puts a day's times into the on-disk record, as an earlier day's
// fetch would have.
func (f *Fetcher) recordMAI(t *testing.T, date time.Time, ts Times) {
	t.Helper()
	if err := f.writeMAICacheEntry(date.In(f.tz).Format("2006-01-02"), ts); err != nil {
		t.Fatal(err)
	}
}
