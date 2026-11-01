package prayer

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// The Naas mosque's Mawaqit calendar, exactly as it was published on 1 Nov 2026.
// It is correct from 25 Oct to 30 Nov but has the wrong GMT/IST changeover on 151
// other days (1 Jan–28 Mar and all of Dec are an hour late; 29 Mar–30 Apr an hour
// early). Mawaqit's own page shows these values unchanged, so the agent must not
// rely on the site being right.
//
//go:embed testdata/naas_mawaqit_calendar_2026-11-01.json
var naasPublished []byte

func mawaqitPage(calendarJSON []byte) string {
	var compact bytes.Buffer
	for _, b := range calendarJSON { // the agent's page parser expects confData on one line
		if b != '\n' {
			compact.WriteByte(b)
		}
	}
	return "<html><script>var confData = {\"timezone\":\"Europe/Dublin\",\"calendar\":" + compact.String() + "};</script></html>"
}

func TestNaasDownloadIsCorrectedAndTheProblemIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, mawaqitPage(naasPublished))
	}))
	defer srv.Close()
	f, err := NewFetcher(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.downloadMawaqit(srv.URL, "naas"); err != nil {
		t.Fatal(err)
	}

	// 1. It is never silent: the agent knows, and says how many days were wrong.
	if got := f.SourceIssue("naas"); got != 151 {
		t.Errorf("SourceIssue(naas) = %d days, want 151", got)
	}
	if got := f.SourceIssue("cork"); got != 0 {
		t.Errorf("SourceIssue for a source never downloaded = %d, want 0", got)
	}

	// 2. Every one of those days still gets the right time.
	loc := mustDublin(t)
	for _, d := range []time.Time{
		time.Date(2026, 1, 15, 12, 0, 0, 0, loc), time.Date(2026, 3, 28, 12, 0, 0, 0, loc),
		time.Date(2026, 3, 29, 12, 0, 0, 0, loc), time.Date(2026, 4, 10, 12, 0, 0, 0, loc),
		time.Date(2026, 4, 30, 12, 0, 0, 0, loc), time.Date(2026, 12, 1, 12, 0, 0, 0, loc),
		time.Date(2026, 12, 25, 12, 0, 0, 0, loc),
	} {
		got, err := f.loadFromFile("naas", d)
		if err != nil {
			t.Fatal(err)
		}
		h, m, _ := parseHHMM(got.Dhuhr)
		if off := math.Abs(float64(h*60+m) - solarNoonLocalMinutes(d, loc)); off > 35 {
			t.Errorf("%s: Dhuhr %s is %.0f min from solar noon — the mosque's wrong hour got through", d.Format("2 Jan"), got.Dhuhr, off)
		}
	}
}

func TestACorrectCalendarIsNotReportedAsWrong(t *testing.T) {
	// The bundled Cork timetable has the changeover right.
	f, _ := NewFetcher(t.TempDir())
	raw, err := f.embeddedTimetable("cork")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, mawaqitPage(raw)) }))
	defer srv.Close()
	if err := f.downloadMawaqit(srv.URL, "cork"); err != nil {
		t.Fatal(err)
	}
	if got := f.SourceIssue("cork"); got != 0 {
		t.Errorf("a correct calendar was reported as wrong on %d days", got)
	}
}

// A restarted agent normally reuses the timetable file it already has, without
// downloading. It must still know — and say — that the calendar is wrong.
func TestTheProblemIsReportedForAnAlreadyDownloadedTimetable(t *testing.T) {
	f, err := NewFetcher(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.timetablePath("naas"), naasPublished, 0o644); err != nil { // fresh file, no download needed
		t.Fatal(err)
	}
	if _, err := f.Fetch("naas", time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if got := f.SourceIssue("naas"); got != 151 {
		t.Errorf("SourceIssue(naas) = %d after loading an existing file, want 151", got)
	}
}
