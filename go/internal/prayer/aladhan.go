package prayer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const aladhanBase = "https://api.aladhan.com/v1"

// AladhanMethod represents a single Aladhan prayer calculation method.
type AladhanMethod struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// AladhanMethods lists every supported Aladhan calculation method.
var AladhanMethods = []AladhanMethod{
	{0, "Shia Ithna-Ashari"},
	{1, "University of Islamic Sciences, Karachi"},
	{2, "Islamic Society of North America (ISNA)"},
	{3, "Muslim World League (MWL)"},
	{4, "Umm Al-Qura University, Makkah"},
	{5, "Egyptian General Authority of Survey"},
	{7, "Institute of Geophysics, University of Tehran"},
	{8, "Gulf Region"},
	{9, "Kuwait"},
	{10, "Qatar"},
	{11, "Majlis Ugama Islam Singapura, Singapore"},
	{12, "Union Organization Islamic de France"},
	{13, "Diyanet İşleri Başkanlığı, Turkey"},
	{14, "Spiritual Administration of Muslims of Russia"},
	{15, "Moonsighting Committee Worldwide"},
	{16, "Dubai"},
	{17, "JAKIM, Malaysia"},
	{18, "Tunisia"},
	{19, "Algeria"},
	{20, "KEMENAG, Indonesia"},
	{21, "Morocco"},
	{22, "Comunidade Islamica de Lisboa"},
	{23, "Ministry of Awqaf, Jordan"},
}

// aladhanBackup is the on-disk offline cache for an Aladhan city/method.
type aladhanBackup struct {
	City       string           `json:"city"`
	Country    string           `json:"country"`
	Method     int              `json:"method"`
	Year       int              `json:"year"`
	Downloaded string           `json:"downloaded"`
	Times      map[string]Times `json:"times"` // "YYYY-MM-DD" → Times
}

// aladhanState holds runtime Aladhan settings stored inside Fetcher.
type aladhanState struct {
	mu         sync.Mutex
	active     bool   // true when location == "aladhan"
	city       string
	country    string
	method     int
	backupOnce sync.Mutex // serialises backup downloads
}

// SetAladhan activates Aladhan mode with the given city/country/method.
// Pass active=false to deactivate (switches back to preloaded timetables).
func (f *Fetcher) SetAladhan(active bool, city, country string, method int) {
	f.aladhan.mu.Lock()
	f.aladhan.active = active
	f.aladhan.city = city
	f.aladhan.country = country
	f.aladhan.method = method
	f.aladhan.mu.Unlock()
}

// fetchAladhan is the top-level entry point when location == "aladhan".
// It tries the live API first, falls back to the local backup on failure.
func (f *Fetcher) fetchAladhan(date time.Time, forceDownload bool) (Times, error) {
	f.aladhan.mu.Lock()
	city, country, method := f.aladhan.city, f.aladhan.country, f.aladhan.method
	f.aladhan.mu.Unlock()

	if city == "" || country == "" {
		return Times{}, fmt.Errorf("Aladhan city/country not configured")
	}

	cacheKey := fmt.Sprintf("aladhan_%s_%s_%d_%s", city, country, method, date.Format("2006-01-02"))
	if !forceDownload {
		f.mu.Lock()
		if e, ok := f.cache[cacheKey]; ok && time.Since(e.at) < cacheTTL {
			f.mu.Unlock()
			return e.times, nil
		}
		f.mu.Unlock()
	}

	// Try live API.
	times, err := f.fetchAladhanAPI(city, country, method, date)
	if err == nil {
		f.mu.Lock()
		f.cache[cacheKey] = cacheEntry{times: times, at: time.Now()}
		f.mu.Unlock()
		go f.maybeRefreshAladhanBackup()
		return times, nil
	}
	log.Printf("[prayer/aladhan] API failed (%v) — trying offline backup", err)

	// Fall back to backup.
	times, err = f.loadAladhanBackup(city, country, method, date)
	if err != nil {
		return Times{}, fmt.Errorf("Aladhan: API and backup both unavailable: %w", err)
	}
	f.mu.Lock()
	f.cache[cacheKey] = cacheEntry{times: times, at: time.Now()}
	f.mu.Unlock()
	return times, nil
}

// fetchAladhanAPI calls the Aladhan timingsByCity endpoint for one day.
func (f *Fetcher) fetchAladhanAPI(city, country string, method int, date time.Time) (Times, error) {
	endpoint := fmt.Sprintf("%s/timingsByCity/%s", aladhanBase, date.Format("02-01-2006"))
	q := url.Values{
		"city":    {city},
		"country": {country},
		"method":  {fmt.Sprintf("%d", method)},
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint + "?" + q.Encode())
	if err != nil {
		return Times{}, fmt.Errorf("GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Times{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Timings map[string]string `json:"timings"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Times{}, fmt.Errorf("decode: %w", err)
	}

	return Times{
		Fajr:    stripTZ(result.Data.Timings["Fajr"]),
		Dhuhr:   stripTZ(result.Data.Timings["Dhuhr"]),
		Asr:     stripTZ(result.Data.Timings["Asr"]),
		Maghrib: stripTZ(result.Data.Timings["Maghrib"]),
		Isha:    stripTZ(result.Data.Timings["Isha"]),
	}, nil
}

// stripTZ removes the " (TZ)" suffix Aladhan appends to time strings.
func stripTZ(s string) string {
	if i := strings.Index(s, " ("); i >= 0 {
		return s[:i]
	}
	return s
}

// aladhanBackupPath returns the path for the local backup JSON.
func (f *Fetcher) aladhanBackupPath() string {
	return filepath.Join(f.dataDir, "aladhan_backup.json")
}

// loadAladhanBackup reads the local backup and returns times for date.
// Returns an error if the backup does not exist, is for a different
// city/country/method, or does not contain the requested date.
func (f *Fetcher) loadAladhanBackup(city, country string, method int, date time.Time) (Times, error) {
	data, err := os.ReadFile(f.aladhanBackupPath())
	if err != nil {
		return Times{}, fmt.Errorf("no backup file: %w", err)
	}
	var b aladhanBackup
	if err := json.Unmarshal(data, &b); err != nil {
		return Times{}, fmt.Errorf("parse backup: %w", err)
	}
	if !strings.EqualFold(b.City, city) || !strings.EqualFold(b.Country, country) || b.Method != method {
		return Times{}, fmt.Errorf("backup is for %s/%s method %d, not %s/%s method %d",
			b.City, b.Country, b.Method, city, country, method)
	}
	key := date.Format("2006-01-02")
	t, ok := b.Times[key]
	if !ok {
		return Times{}, fmt.Errorf("backup has no entry for %s", key)
	}
	return t, nil
}

// maybeRefreshAladhanBackup downloads the full-year backup in the background
// if the existing one is stale, missing, or for a different city/country/method.
func (f *Fetcher) maybeRefreshAladhanBackup() {
	if !f.aladhan.backupOnce.TryLock() {
		return // another download already running
	}
	defer f.aladhan.backupOnce.Unlock()

	f.aladhan.mu.Lock()
	city, country, method := f.aladhan.city, f.aladhan.country, f.aladhan.method
	f.aladhan.mu.Unlock()

	year := time.Now().Year()
	data, err := os.ReadFile(f.aladhanBackupPath())
	if err == nil {
		var b aladhanBackup
		if json.Unmarshal(data, &b) == nil &&
			strings.EqualFold(b.City, city) &&
			strings.EqualFold(b.Country, country) &&
			b.Method == method &&
			b.Year == year {
			return // backup is current
		}
	}

	if err := f.downloadAladhanBackup(city, country, method, year); err != nil {
		log.Printf("[prayer/aladhan] backup download failed: %v", err)
	}
}

// downloadAladhanBackup downloads 12 months of calendarByCity data and saves
// them as a flat YYYY-MM-DD keyed JSON backup file.
func (f *Fetcher) downloadAladhanBackup(city, country string, method, year int) error {
	log.Printf("[prayer/aladhan] downloading full-year backup: %s, %s (method %d, %d)",
		city, country, method, year)

	client := &http.Client{Timeout: 30 * time.Second}
	allTimes := make(map[string]Times, 366)

	for month := 1; month <= 12; month++ {
		endpoint := fmt.Sprintf("%s/calendarByCity/%d/%d", aladhanBase, year, month)
		q := url.Values{
			"city":    {city},
			"country": {country},
			"method":  {fmt.Sprintf("%d", method)},
		}
		resp, err := client.Get(endpoint + "?" + q.Encode())
		if err != nil {
			return fmt.Errorf("month %d: %w", month, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("month %d HTTP %d", month, resp.StatusCode)
		}

		var result struct {
			Data []struct {
				Date struct {
					Gregorian struct {
						Date string `json:"date"` // "DD-MM-YYYY"
					} `json:"gregorian"`
				} `json:"date"`
				Timings map[string]string `json:"timings"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("month %d decode: %w", month, err)
		}
		for _, day := range result.Data {
			// Convert DD-MM-YYYY → YYYY-MM-DD
			parts := strings.Split(day.Date.Gregorian.Date, "-")
			if len(parts) != 3 {
				continue
			}
			key := fmt.Sprintf("%s-%s-%s", parts[2], parts[1], parts[0])
			allTimes[key] = Times{
				Fajr:    stripTZ(day.Timings["Fajr"]),
				Dhuhr:   stripTZ(day.Timings["Dhuhr"]),
				Asr:     stripTZ(day.Timings["Asr"]),
				Maghrib: stripTZ(day.Timings["Maghrib"]),
				Isha:    stripTZ(day.Timings["Isha"]),
			}
		}
	}

	b := aladhanBackup{
		City:       city,
		Country:    country,
		Method:     method,
		Year:       year,
		Downloaded: time.Now().Format(time.RFC3339),
		Times:      allTimes,
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(f.aladhanBackupPath(), data, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	log.Printf("[prayer/aladhan] backup saved (%d days)", len(allTimes))
	return nil
}
