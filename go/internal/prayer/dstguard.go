package prayer

import (
	"fmt"
	"log"
	"math"
	"time"
)

// dublinLongitude is the longitude of Dublin in degrees east (negative = west).
// Solar noon only moves ~4 min per degree, so this is accurate for anywhere in
// Ireland.
const dublinLongitude = -6.2603

// solarNoonLocalMinutes returns the astronomical solar noon for date, expressed
// as minutes since midnight on the wall clock of loc (so it automatically
// includes whatever UTC offset loc has on that date — GMT or IST).
//
// Uses the NOAA equation-of-time approximation (error well under a minute).
func solarNoonLocalMinutes(date time.Time, loc *time.Location) float64 {
	d := date.In(loc)
	noon := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, loc)

	gamma := 2 * math.Pi / 365 * float64(noon.YearDay()-1)
	eqTime := 229.18 * (0.000075 +
		0.001868*math.Cos(gamma) -
		0.032077*math.Sin(gamma) -
		0.014615*math.Cos(2*gamma) -
		0.040849*math.Sin(2*gamma))

	solarNoonUTC := 720 - 4*dublinLongitude - eqTime // minutes after 00:00 UTC
	_, offsetSec := noon.Zone()
	return solarNoonUTC + float64(offsetSec)/60
}

func clockToMinutes(s string) (int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func minutesToClock(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// guardDST protects against timetables whose daylight-saving switch is wrong.
//
// Published timetables bake the GMT↔IST clock change into the numbers, usually
// for one particular year, so around the changeover (and sometimes for months)
// they can be exactly one hour out. Whatever the source, true Dhuhr is always
// within a few minutes of solar noon, so we compare the source's Dhuhr with
// the astronomically expected wall-clock time for that date in loc. If it is
// a whole number of hours away (1, or 2 if the adjustment was applied twice),
// all five prayers are shifted by that amount.
//
// It returns the (possibly corrected) times and the shift applied in hours
// (0 if the data was already consistent). Anything that isn't within 30 min of
// a whole 1–2 hour error is left untouched.
func guardDST(t Times, date time.Time, loc *time.Location) (Times, int) {
	out, shift := correctDST(t, date, loc)
	if shift != 0 {
		log.Printf("[prayer/dst] source timetable is %+dh out of step with %s on %s "+
			"(Dhuhr %s vs solar noon %s) — corrected",
			-shift, loc, date.In(loc).Format("2006-01-02"),
			t.Dhuhr, minutesToClock(int(math.Round(solarNoonLocalMinutes(date, loc)))))
	}
	return out, shift
}

// correctDST is guardDST without the logging: it returns the corrected times and
// the shift applied in hours (0 if the row was already consistent).
func correctDST(t Times, date time.Time, loc *time.Location) (Times, int) {
	dhuhr, ok := clockToMinutes(t.Dhuhr)
	if !ok {
		return t, 0
	}
	diff := float64(dhuhr) - solarNoonLocalMinutes(date, loc)
	k := int(math.Round(diff / 60))
	if k == 0 || k > 2 || k < -2 {
		return t, 0 // consistent (k==0), or too far off to be a DST slip
	}
	if math.Abs(diff-60*float64(k)) > 30 {
		return t, 0 // not close to a whole hour off; don't guess
	}

	shift := func(s string) (string, bool) {
		m, ok := clockToMinutes(s)
		if !ok {
			return s, s == "" // leave empty values alone, bail on garbage
		}
		// Wrap around midnight: Isha is after midnight in summer ("00:17"), and an
		// hour earlier than that is 23:17 the previous evening.
		m = ((m-60*k)%(24*60) + 24*60) % (24 * 60)
		return minutesToClock(m), true
	}

	var out Times
	var oks [5]bool
	out.Fajr, oks[0] = shift(t.Fajr)
	out.Dhuhr, oks[1] = shift(t.Dhuhr)
	out.Asr, oks[2] = shift(t.Asr)
	out.Maghrib, oks[3] = shift(t.Maghrib)
	out.Isha, oks[4] = shift(t.Isha)
	for _, ok := range oks {
		if !ok {
			return t, 0
		}
	}
	return out, -k
}
