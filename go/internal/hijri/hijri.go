package hijri

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Date holds the Hijri calendar date for a given Gregorian day.
type Date struct {
	Day      int
	Month    int
	MonthEN  string
	MonthAR  string
	Year     int
	Weekday  string
	Holidays []string
}

// String returns a human-readable English date, e.g. "12 Dhū al-Ḥijjah 1447 AH".
func (d Date) String() string {
	return fmt.Sprintf("%d %s %d AH", d.Day, d.MonthEN, d.Year)
}

// IsRamadan reports whether this date falls in the month of Ramadan (month 9).
func (d Date) IsRamadan() bool { return d.Month == 9 }

// RamadanDay returns the day number within Ramadan (1–29/30), or 0 if not Ramadan.
func (d Date) RamadanDay() int {
	if d.IsRamadan() {
		return d.Day
	}
	return 0
}

// SpecialDay returns the name of any significant Islamic occasion today,
// or an empty string on an ordinary day.
func (d Date) SpecialDay() string {
	switch {
	case d.Month == 10 && d.Day == 1:
		return "Eid al-Fitr"
	case d.Month == 12 && d.Day == 10:
		return "Eid al-Adha"
	case d.Month == 12 && d.Day == 9:
		return "Day of Arafat"
	case d.Month == 1 && d.Day == 1:
		return "Islamic New Year"
	case d.Month == 3 && d.Day == 12:
		return "Mawlid al-Nabi"
	case d.Month == 7 && d.Day == 27:
		return "Laylat al-Mi'raj"
	case d.Month == 8 && d.Day == 15:
		return "Laylat al-Bara'at"
	case d.IsRamadan() && d.Day == 27:
		return "Laylat al-Qadr (27th)"
	case d.IsRamadan():
		return fmt.Sprintf("Ramadan — Day %d", d.Day)
	}
	if len(d.Holidays) > 0 {
		return d.Holidays[0]
	}
	return ""
}

// in-memory daily cache
var (
	mu        sync.Mutex
	cached    *Date
	cacheDate string // "YYYY-MM-DD"
)

// Today returns the Hijri date for today, fetching from the Aladhan API and
// caching the result for the rest of the calendar day. On a network error after
// a successful previous fetch the last known value is returned so the dashboard
// keeps working offline.
func Today() (*Date, error) {
	key := time.Now().Format("2006-01-02")
	mu.Lock()
	defer mu.Unlock()
	if cached != nil && cacheDate == key {
		return cached, nil
	}
	d, err := fetch(time.Now())
	if err != nil {
		if cached != nil {
			return cached, nil // stale but better than nothing
		}
		return nil, err
	}
	cached = d
	cacheDate = key
	return d, nil
}

// fetch calls the Aladhan gToH endpoint for the given time.
func fetch(t time.Time) (*Date, error) {
	url := fmt.Sprintf("https://api.aladhan.com/v1/gToH/%s", t.Format("02-01-2006"))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("hijri fetch: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Data struct {
			Hijri struct {
				Day  string `json:"day"`
				Month struct {
					Number int    `json:"number"`
					EN     string `json:"en"`
					AR     string `json:"ar"`
				} `json:"month"`
				Year    string `json:"year"`
				Weekday struct {
					EN string `json:"en"`
				} `json:"weekday"`
				Holidays []string `json:"holidays"`
			} `json:"hijri"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("hijri decode: %w", err)
	}

	h := payload.Data.Hijri
	day, _ := strconv.Atoi(strings.TrimSpace(h.Day))
	year, _ := strconv.Atoi(strings.TrimSpace(h.Year))

	return &Date{
		Day:      day,
		Month:    h.Month.Number,
		MonthEN:  h.Month.EN,
		MonthAR:  h.Month.AR,
		Year:     year,
		Weekday:  h.Weekday.EN,
		Holidays: h.Holidays,
	}, nil
}
