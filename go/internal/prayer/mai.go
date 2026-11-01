package prayer

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"azan-agent/internal/clock"
	"encoding/json"
)

// The Muslim Association of Ireland (maionline.ie) publishes today's prayer times
// on its home page, as a table rendered on the server; there is no yearly file
// or API. The agent reads the page for today, checks it, and records each day it
// sees so that yesterday's times (needed for an after-midnight Isha) and a few
// days of fallback survive the site being briefly unavailable.

const (
	maiURLDefault  = "https://maionline.ie/"
	maiMaxStale    = 7 * 24 * time.Hour // never fall back to data older than this
	maiKeepEntries = 30                 // days kept in the local record
)

// maiURL is the page to read. AZAN_MAI_URL overrides it (used by tests).
func maiURL() string {
	if v := os.Getenv("AZAN_MAI_URL"); v != "" {
		return v
	}
	return maiURLDefault
}

var (
	maiPrayerRE = regexp.MustCompile(`prayer-label (fajr|shurooq|dhuhr|asr|maghrib|isha)'>[^<]*</div>\s*<div class='prayer-time'>\s*` +
		`<span[^>]*>(\d{1,2})</span>\s*<span[^>]*>:</span>\s*<span[^>]*>(\d{1,2})</span>`)
	maiDateRE = regexp.MustCompile(`(?i)class='date'>[^<]*?(\d{1,2})(?:st|nd|rd|th)\s+([A-Za-z]+)\s+(\d{4})`)

	maiMu sync.Mutex // guards the on-disk record
)

// parseMAIPage extracts the date and the five prayer times from the MAI home
// page. It returns an error for anything it cannot fully trust, so a changed page
// never turns into wrong prayer times.
func parseMAIPage(page []byte) (time.Time, Times, error) {
	dm := maiDateRE.FindSubmatch(page)
	if dm == nil {
		return time.Time{}, Times{}, fmt.Errorf("MAI page has no date (the page layout may have changed)")
	}
	date, err := time.Parse("2 January 2006", fmt.Sprintf("%s %s %s", dm[1], dm[2], dm[3]))
	if err != nil {
		return time.Time{}, Times{}, fmt.Errorf("MAI page date %q %q %q: %w", dm[1], dm[2], dm[3], err)
	}

	found := map[string]string{}
	for _, m := range maiPrayerRE.FindAllSubmatch(page, -1) {
		h, _ := strconv.Atoi(string(m[2]))
		min, _ := strconv.Atoi(string(m[3]))
		if h > 23 || min > 59 {
			return time.Time{}, Times{}, fmt.Errorf("MAI page: impossible time %s:%s for %s", m[2], m[3], m[1])
		}
		found[string(m[1])] = fmt.Sprintf("%02d:%02d", h, min)
	}
	for _, p := range []string{"fajr", "dhuhr", "asr", "maghrib", "isha"} {
		if found[p] == "" {
			return time.Time{}, Times{}, fmt.Errorf("MAI page has no %s time (the page layout may have changed)", p)
		}
	}
	ts := Times{Fajr: found["fajr"], Dhuhr: found["dhuhr"], Asr: found["asr"], Maghrib: found["maghrib"], Isha: found["isha"]}

	// Fajr < Dhuhr < Asr < Maghrib on the clock. (Isha is not checked: in summer
	// it falls after midnight, as an early-morning time.)
	prev := -1
	for _, hhmm := range []string{ts.Fajr, ts.Dhuhr, ts.Asr, ts.Maghrib} {
		m, _ := clockToMinutes(hhmm)
		if m <= prev {
			return time.Time{}, Times{}, fmt.Errorf("MAI page times are out of order: %+v", ts)
		}
		prev = m
	}
	return date, ts, nil
}

func (f *Fetcher) maiCachePath() string { return filepath.Join(f.dataDir, "mai_times.json") }

func (f *Fetcher) readMAICache() map[string]Times {
	maiMu.Lock()
	defer maiMu.Unlock()
	return f.readMAICacheLocked()
}

func (f *Fetcher) readMAICacheLocked() map[string]Times {
	out := map[string]Times{}
	if b, err := os.ReadFile(f.maiCachePath()); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

// writeMAICacheEntry records one day's times (and forgets very old ones).
func (f *Fetcher) writeMAICacheEntry(day string, ts Times) error {
	maiMu.Lock()
	defer maiMu.Unlock()
	rec := f.readMAICacheLocked()
	rec[day] = ts
	if len(rec) > maiKeepEntries {
		keys := make([]string, 0, len(rec))
		for k := range rec {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys[:len(keys)-maiKeepEntries] {
			delete(rec, k)
		}
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.maiCachePath(), b, 0o644)
}

// scrapeMAI reads the MAI home page and records today's times. day is today's
// date (YYYY-MM-DD); a page dated differently is refused.
func (f *Fetcher) scrapeMAI(day string) (Times, error) {
	req, err := http.NewRequest("GET", maiURL(), nil)
	if err != nil {
		return Times{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/124.0.0.0 Safari/537.36")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return Times{}, fmt.Errorf("GET MAI: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Times{}, fmt.Errorf("MAI HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Times{}, fmt.Errorf("read MAI page: %w", err)
	}
	pageDate, ts, err := parseMAIPage(body)
	if err != nil {
		return Times{}, err
	}
	if got := pageDate.Format("2006-01-02"); got != day {
		return Times{}, fmt.Errorf("MAI page is for %s, not today (%s)", got, day)
	}
	d, _ := time.ParseInLocation("2006-01-02", day, f.tz)
	ts, _ = guardDST(ts, d.Add(12*time.Hour), f.tz) // same sanity check as every other source
	if err := f.writeMAICacheEntry(day, ts); err != nil {
		log.Printf("[prayer/mai] could not record today's times: %v", err)
	}
	return ts, nil
}

// maiFromRecord returns the recorded times for day; if there are none and day
// is not in the future, the most recent recorded day within a week before it.
func (f *Fetcher) maiFromRecord(day, today string) (Times, error) {
	rec := f.readMAICache()
	if ts, ok := rec[day]; ok {
		return ts, nil
	}
	if day > today {
		return Times{}, fmt.Errorf("MAI publishes only today's times; %s is in the future", day)
	}
	wanted, _ := time.Parse("2006-01-02", day)
	best := ""
	for k := range rec {
		kd, err := time.Parse("2006-01-02", k)
		if err != nil || kd.After(wanted) || wanted.Sub(kd) > maiMaxStale {
			continue
		}
		if k > best {
			best = k
		}
	}
	if best == "" {
		return Times{}, fmt.Errorf("no MAI times recorded for %s or the week before", day)
	}
	log.Printf("[prayer/mai] WARNING: no times for %s — using %s's (they move about a minute a day)", day, best)
	return rec[best], nil
}

// fetchMAI returns the MAI times for date: read live for today, otherwise from
// the local record.
func (f *Fetcher) fetchMAI(date time.Time, force bool) (Times, error) {
	day := date.In(f.tz).Format("2006-01-02")
	key := "mai_" + day
	if !force {
		f.mu.Lock()
		if e, ok := f.cache[key]; ok && clock.Now().Sub(e.at) < cacheTTL {
			f.mu.Unlock()
			return e.times, nil
		}
		f.mu.Unlock()
	}

	today := clock.Now().In(f.tz).Format("2006-01-02")
	var (
		ts  Times
		err error
	)
	if day == today {
		if ts, err = f.scrapeMAI(day); err != nil {
			log.Printf("[prayer/mai] could not read today's times from the site: %v — using the local record", err)
			ts, err = f.maiFromRecord(day, today)
		}
	} else {
		ts, err = f.maiFromRecord(day, today)
	}
	if err != nil {
		return Times{}, err
	}

	f.mu.Lock()
	f.cache[key] = cacheEntry{times: ts, at: clock.Now()}
	f.mu.Unlock()
	return ts, nil
}

var _ = strings.TrimSpace
