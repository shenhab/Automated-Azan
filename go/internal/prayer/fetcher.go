package prayer

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

//go:embed embedded
var embeddedTimetables embed.FS

const (
	icciBuildURL      = "https://islamireland.ie/api/timetable/"
	naasURL           = "https://mawaqit.net/en/m/-34"
	newbridgeURL      = "https://mawaqit.net/en/newbridge-masjid-newbridge-newbridge-ireland"
	corkURL           = "https://mawaqit.net/en/m/icc-bandon-cork-bandon-p72ad92-ireland"
	galwayURL         = "https://mawaqit.net/en/m/tuam-islamic-culture-centre-tuam-h54p62b-ireland-1"
	timezone          = "Europe/Dublin"
	cacheTTL          = time.Hour
)

// Times holds the five daily prayer times as HH:MM strings.
type Times struct {
	Fajr    string `json:"Fajr"`
	Dhuhr   string `json:"Dhuhr"`
	Asr     string `json:"Asr"`
	Maghrib string `json:"Maghrib"`
	Isha    string `json:"Isha"`
}

type cacheEntry struct {
	times Times
	at    time.Time
}

// Fetcher fetches and caches prayer times for naas/icci/newbridge/aladhan.
type Fetcher struct {
	dataDir string
	tz      *time.Location
	mu      sync.Mutex
	cache   map[string]cacheEntry // key: "location_YYYY-MM-DD"
	aladhan aladhanState
}

// NewFetcher creates a Fetcher that stores timetable files in dataDir.
func NewFetcher(dataDir string) (*Fetcher, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone %s: %w", timezone, err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	return &Fetcher{dataDir: dataDir, tz: loc, cache: make(map[string]cacheEntry)}, nil
}

// Fetch returns prayer times for location on date, downloading if necessary.
// When location is "aladhan", delegates to the Aladhan API path.
func (f *Fetcher) Fetch(location string, date time.Time, forceDownload bool) (Times, error) {
	if location == "aladhan" {
		return f.fetchAladhan(date, forceDownload)
	}

	key := fmt.Sprintf("%s_%s", location, date.Format("2006-01-02"))

	if !forceDownload {
		f.mu.Lock()
		if e, ok := f.cache[key]; ok && time.Since(e.at) < cacheTTL {
			f.mu.Unlock()
			return e.times, nil
		}
		f.mu.Unlock()
	}

	if forceDownload || f.needsRefresh(location) {
		if err := f.download(location); err != nil {
			log.Printf("[prayer/fetcher] download %s failed: %v — using local file if available", location, err)
		}
	}

	times, err := f.loadFromFile(location, date)
	if err != nil {
		return Times{}, err
	}

	f.mu.Lock()
	f.cache[key] = cacheEntry{times: times, at: time.Now()}
	f.mu.Unlock()

	return times, nil
}

// ForceRefresh forces a download for the given location (or all if empty).
// For "aladhan" it triggers a full-year backup download.
func (f *Fetcher) ForceRefresh(location string) error {
	if location == "aladhan" {
		f.aladhan.mu.Lock()
		city, country, method := f.aladhan.city, f.aladhan.country, f.aladhan.method
		f.aladhan.mu.Unlock()
		return f.downloadAladhanBackup(city, country, method, time.Now().Year())
	}
	locations := []string{location}
	if location == "" {
		locations = []string{"icci", "naas", "newbridge", "cork", "galway"}
	}
	for _, loc := range locations {
		if err := f.download(loc); err != nil {
			return fmt.Errorf("%s: %w", loc, err)
		}
	}
	return nil
}

// ClearCache removes all in-memory cached entries.
func (f *Fetcher) ClearCache() {
	f.mu.Lock()
	f.cache = make(map[string]cacheEntry)
	f.mu.Unlock()
}

// needsRefresh returns true if the timetable file is missing, older than 7 days,
// or a DST change has been detected since the last download.
func (f *Fetcher) needsRefresh(location string) bool {
	path := f.timetablePath(location)
	info, err := os.Stat(path)
	if err != nil {
		return true // file missing
	}
	if time.Since(info.ModTime()) > 7*24*time.Hour {
		return true // weekly refresh
	}
	if f.dstChanged(location) {
		return true
	}
	return false
}

func (f *Fetcher) download(location string) error {
	switch location {
	case "icci":
		return f.downloadICCI()
	case "naas":
		return f.downloadMawaqit(naasURL, "naas")
	case "newbridge":
		return f.downloadMawaqit(newbridgeURL, "newbridge")
	case "cork":
		return f.downloadMawaqit(corkURL, "cork")
	case "galway":
		return f.downloadMawaqit(galwayURL, "galway")
	default:
		return fmt.Errorf("unknown location: %s", location)
	}
}

func (f *Fetcher) downloadICCI() error {
	log.Println("[prayer/fetcher] downloading ICCI timetable")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(icciBuildURL)
	if err != nil {
		return fmt.Errorf("GET icci: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("icci HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read icci body: %w", err)
	}
	// Validate JSON before saving
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid ICCI JSON: %w", err)
	}
	path := f.timetablePath("icci")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write icci: %w", err)
	}
	f.saveDSTMetadata("icci")
	log.Printf("[prayer/fetcher] ICCI timetable saved to %s", path)
	return nil
}

// downloadMawaqit fetches a Mawaqit mosque page, extracts the confData calendar
// block, and saves it as the timetable for the given location key.
// Handles both "var confData = {...}" and "confData = {...}" page variants.
func (f *Fetcher) downloadMawaqit(url, location string) error {
	log.Printf("[prayer/fetcher] downloading %s timetable from Mawaqit", location)
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", location, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s body: %w", location, err)
	}

	// Some pages use "var confData = {...};" others just "confData = {...};"
	re := regexp.MustCompile(`(?:var )?confData = ({[^\n]+});`)
	m := re.FindSubmatch(body)
	if m == nil {
		return fmt.Errorf("confData block not found in Mawaqit page for %s", location)
	}

	var conf struct {
		Calendar []interface{} `json:"calendar"`
	}
	if err := json.Unmarshal(m[1], &conf); err != nil {
		return fmt.Errorf("parse confData for %s: %w", location, err)
	}
	if len(conf.Calendar) != 12 {
		return fmt.Errorf("expected 12 months for %s, got %d", location, len(conf.Calendar))
	}

	data, err := json.MarshalIndent(conf.Calendar, "", "  ")
	if err != nil {
		return fmt.Errorf("re-marshal %s: %w", location, err)
	}

	path := f.timetablePath(location)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", location, err)
	}
	f.saveDSTMetadata(location)
	log.Printf("[prayer/fetcher] %s timetable saved to %s", location, path)
	return nil
}

// loadFromFile reads the local JSON timetable and extracts prayer times for date.
// Falls back to the embedded timetable bundled in the binary if the local file
// is missing (e.g. first run before a download has completed).
func (f *Fetcher) loadFromFile(location string, date time.Time) (Times, error) {
	path := f.timetablePath(location)
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[prayer/fetcher] local file not found (%s), using embedded fallback", path)
		data, err = f.embeddedTimetable(location)
		if err != nil {
			return Times{}, fmt.Errorf("no timetable available for %s: %w", location, err)
		}
	}

	switch location {
	case "icci":
		return f.extractICCI(data, date)
	case "naas", "newbridge", "cork", "galway":
		return f.extractNaas(data, date)
	default:
		return Times{}, fmt.Errorf("unknown location: %s", location)
	}
}

// embeddedTimetable returns the bundled timetable JSON for the given location.
func (f *Fetcher) embeddedTimetable(location string) ([]byte, error) {
	names := map[string]string{
		"naas":      "embedded/naas_prayers_timetable.json",
		"icci":      "embedded/icci_timetable.json",
		"newbridge": "embedded/newbridge_timetable.json",
		"cork":      "embedded/cork_timetable.json",
		"galway":    "embedded/galway_timetable.json",
	}
	name, ok := names[location]
	if !ok {
		return nil, fmt.Errorf("no embedded timetable for %s", location)
	}
	return embeddedTimetables.ReadFile(name)
}

// extractICCI parses ICCI JSON and extracts times for date.
// ICCI format: {"timetable": {"month": {"day": [[h,m],[h,m],...]}}}
// Indices: 0=Fajr, 1=Sunrise, 2=Dhuhr, 3=Asr, 4=Maghrib, 5=Isha
func (f *Fetcher) extractICCI(data []byte, date time.Time) (Times, error) {
	var doc struct {
		Timetable map[string]map[string][][2]int `json:"timetable"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return Times{}, fmt.Errorf("parse icci: %w", err)
	}

	month := fmt.Sprintf("%d", date.Month())
	day := fmt.Sprintf("%d", date.Day())

	monthData, ok := doc.Timetable[month]
	if !ok {
		return Times{}, fmt.Errorf("ICCI: month %s not found", month)
	}
	dayData, ok := monthData[day]
	if !ok {
		return Times{}, fmt.Errorf("ICCI: day %s not found in month %s", day, month)
	}
	if len(dayData) < 6 {
		return Times{}, fmt.Errorf("ICCI: unexpected prayer count %d", len(dayData))
	}

	hm := func(i int) string {
		return fmt.Sprintf("%02d:%02d", dayData[i][0], dayData[i][1])
	}
	return Times{
		Fajr:    hm(0),
		Dhuhr:   hm(2),
		Asr:     hm(3),
		Maghrib: hm(4),
		Isha:    hm(5),
	}, nil
}

// extractNaas parses Naas JSON (12-element array, months 0-indexed).
// Day format: {"1": ["07:45", "09:39", "13:32", ...]}
// Indices: 0=Fajr, 1=Sunrise, 2=Dhuhr, 3=Asr, 4=Maghrib, 5=Isha
func (f *Fetcher) extractNaas(data []byte, date time.Time) (Times, error) {
	var months []map[string][]string
	if err := json.Unmarshal(data, &months); err != nil {
		return Times{}, fmt.Errorf("parse naas: %w", err)
	}

	monthIdx := int(date.Month()) - 1 // 0-indexed
	if monthIdx < 0 || monthIdx >= len(months) {
		return Times{}, fmt.Errorf("Naas: month %d out of range", date.Month())
	}

	day := fmt.Sprintf("%d", date.Day())
	prayers, ok := months[monthIdx][day]
	if !ok {
		return Times{}, fmt.Errorf("Naas: day %s not found", day)
	}
	if len(prayers) < 6 {
		return Times{}, fmt.Errorf("Naas: unexpected prayer count %d", len(prayers))
	}

	return Times{
		Fajr:    prayers[0],
		Dhuhr:   prayers[2],
		Asr:     prayers[3],
		Maghrib: prayers[4],
		Isha:    prayers[5],
	}, nil
}

// --- DST detection ---

type dstMetadata struct {
	Offset      int    `json:"dst_offset"`
	LastChecked string `json:"last_checked"`
	Timezone    string `json:"timezone"`
}

func (f *Fetcher) currentOffset() int {
	_, offset := time.Now().In(f.tz).Zone()
	return offset
}

func (f *Fetcher) dstMetaPath(location string) string {
	return filepath.Join(f.dataDir, location+"_dst_metadata.json")
}

func (f *Fetcher) saveDSTMetadata(location string) {
	meta := dstMetadata{
		Offset:      f.currentOffset(),
		LastChecked: time.Now().In(f.tz).Format(time.RFC3339),
		Timezone:    timezone,
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(f.dstMetaPath(location), data, 0o644)
}

func (f *Fetcher) dstChanged(location string) bool {
	data, err := os.ReadFile(f.dstMetaPath(location))
	if err != nil {
		return false // no metadata, assume no change
	}
	var meta dstMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return false
	}
	return meta.Offset != f.currentOffset()
}

func (f *Fetcher) timetablePath(location string) string {
	names := map[string]string{
		"naas":      "naas_prayers_timetable.json",
		"icci":      "icci_timetable.json",
		"newbridge": "newbridge_timetable.json",
		"cork":      "cork_timetable.json",
		"galway":    "galway_timetable.json",
	}
	return filepath.Join(f.dataDir, names[location])
}

// Timezone returns the timezone to use for date calculations.
// For Aladhan mode the API already returns local times, so we use the system
// local timezone (assumes the agent runs on the user's own machine).
func (f *Fetcher) Timezone() *time.Location {
	f.aladhan.mu.Lock()
	active := f.aladhan.active
	f.aladhan.mu.Unlock()
	if active {
		return time.Local
	}
	return f.tz
}
