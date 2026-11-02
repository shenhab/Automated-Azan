package prayer

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"azan-agent/internal/clock"
	"azan-agent/internal/config"
	"azan-agent/internal/timesync"
	"azan-agent/internal/trace"
)

// PlayFunc is called when it's time to play Athan.
// prayer is the prayer name (e.g. "Fajr"), filename is the MP3 to play.
type PlayFunc func(prayer, filename string) error

// JobStatus represents a scheduled or completed prayer job.
type JobStatus struct {
	Label     string `json:"label"`
	Prayer    string `json:"prayer"`
	Type      string `json:"type"`
	Time      string `json:"scheduled_time"`
	Status    string `json:"status"` // "upcoming" | "done" | "skipped"
	NextRunAt string `json:"next_run,omitempty"`
}

const (
	warmupLeadTime = 60 * time.Second
	muteLeadTime   = 2 * time.Second

	// journalLeadTime is how long before a prayer its Journal session starts, so
	// the recording shows the speaker's state before anything is asked of it.
	journalLeadTime = 90 * time.Second
)

// sessionStartDelay is how long to wait before starting a prayer's Journal
// session: the lead time before the prayer, or no wait if the agent is already
// inside that lead (it started late, or was restarted just before a prayer).
func sessionStartDelay(untilPrayer time.Duration) time.Duration {
	if d := untilPrayer - journalLeadTime; d > 0 {
		return d
	}
	return 0
}

// The day's prayer schedule is rebuilt at 00:05 local time every day.
//
// It must be a wall-clock time that exists exactly once on every day of the
// year. 01:00 is not: in Ireland it does not exist on the spring-forward day
// (so that day's schedule was never built) and happens twice on the fall-back
// day. 00:05 is always valid, and still comfortably before the earliest Fajr.
const (
	dailyRefreshHour   = 0
	dailyRefreshMinute = 5
)

// nextDailyRefresh returns the next refresh instant strictly after `from`.
// time.Date resolves the wall-clock time in loc, so it is correct across
// daylight-saving changes.
func nextDailyRefresh(from time.Time, loc *time.Location) time.Time {
	f := from.In(loc)
	t := time.Date(f.Year(), f.Month(), f.Day(), dailyRefreshHour, dailyRefreshMinute, 0, 0, loc)
	if !t.After(f) {
		t = time.Date(f.Year(), f.Month(), f.Day()+1, dailyRefreshHour, dailyRefreshMinute, 0, 0, loc)
	}
	return t
}

// Scheduler schedules daily Athan playback and optional Quran/Kahf jobs.
type Scheduler struct {
	cfg       *config.Config
	fetcher   *Fetcher
	tz        *time.Location
	playAthan PlayFunc
	playQuran func(durationSec int) error
	playKahf  func() error
	stopQuran func()
	stopKahf  func()
	warmup    func() error // called warmupLeadTime before each event to pre-connect
	mute      func() error // called muteLeadTime before each Athan to silence TVs

	sessionStart func(prayer, label string, at time.Time) // called journalLeadTime before each job

	mu          sync.Mutex
	todayJobs   []JobStatus
	timers      []*time.Timer
	stopRefresh chan struct{} // closes the daily-refresh goroutine
	running     bool

	// now is the clock; nil means time.Now. Tests inject a fixed date to check
	// scheduling across daylight-saving changes.
	now func() time.Time

	// timeSync and afterRefresh are test seams for the daily refresh.
	timeSync     func()
	afterRefresh func()

	// quran/kahf stop channels
	quranStop chan struct{}
	kahfStop  chan struct{}
}

func (s *Scheduler) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return clock.Now()
}

// SetSessionStart registers a function called journalLeadTime before every job
// (or at once if that moment has passed). The Journal uses it to start recording.
func (s *Scheduler) SetSessionStart(f func(prayer, label string, at time.Time)) {
	s.mu.Lock()
	s.sessionStart = f
	s.mu.Unlock()
}

// NewScheduler constructs a Scheduler.
func NewScheduler(
	cfg *config.Config,
	fetcher *Fetcher,
	playAthan PlayFunc,
	playQuran func(durationSec int) error,
	stopQuran func(),
	playKahf func() error,
	stopKahf func(),
	warmup func() error,
	mute func() error,
) *Scheduler {
	return &Scheduler{
		cfg:       cfg,
		fetcher:   fetcher,
		tz:        fetcher.Timezone(),
		playAthan: playAthan,
		playQuran: playQuran,
		stopQuran: stopQuran,
		playKahf:  playKahf,
		stopKahf:  stopKahf,
		warmup:    warmup,
		mute:      mute,
	}
}

// Start launches the scheduler: schedules today's prayers and starts the daily
// loop that rebuilds it every day at 00:05 (see nextDailyRefresh).
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	if err := s.scheduleTodayLocked(); err != nil {
		return err
	}

	s.stopRefresh = make(chan struct{})
	go s.refreshLoop(s.stopRefresh)
	s.running = true
	log.Println("[scheduler] started")
	return nil
}

// Stop cancels all pending timers and the daily refresh.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelTimersLocked()
	if s.stopRefresh != nil {
		close(s.stopRefresh)
		s.stopRefresh = nil
	}
	s.running = false
	log.Println("[scheduler] stopped")
}

// refreshLoop rebuilds the day's schedule every day at 00:05 local time until
// stop is closed.
func (s *Scheduler) refreshLoop(stop <-chan struct{}) {
	var last time.Time
	for {
		from := s.clock()
		if from.Before(last) {
			from = last // never fire the same refresh twice
		}
		next := nextDailyRefresh(from, s.tz)
		timer := time.NewTimer(next.Sub(s.clock()))
		select {
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		}
		last = next
		log.Println("[scheduler] daily refresh triggered")
		if s.timeSync != nil {
			s.timeSync()
		} else {
			timesync.Sync()
		}
		s.Reschedule()
		if s.afterRefresh != nil {
			s.afterRefresh()
		}
	}
}

// Reschedule reloads prayer times and recreates all timers. Safe to call
// concurrently (e.g., from config watcher callback).
func (s *Scheduler) Reschedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelTimersLocked()
	if err := s.scheduleTodayLocked(); err != nil {
		log.Printf("[scheduler] reschedule error: %v", err)
	}
}

// Status returns the current job list snapshot.
func (s *Scheduler) Status() []JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]JobStatus, len(s.todayJobs))
	copy(out, s.todayJobs)
	return out
}

// NextPrayer returns the name and scheduled time of the next upcoming prayer.
func (s *Scheduler) NextPrayer() (name string, at time.Time, ok bool) {
	now := s.clock().In(s.tz)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.todayJobs {
		if j.Status != "upcoming" || j.NextRunAt == "" {
			continue
		}
		// NextRunAt carries the full date, so a prayer that falls after midnight
		// (see ishaAfterMidnight) is reported on the correct day.
		at, err := time.Parse(time.RFC3339, j.NextRunAt)
		if err != nil {
			continue
		}
		if at.After(now) {
			return j.Prayer, at.In(s.tz), true
		}
	}
	return "", time.Time{}, false
}

// --- internal ---

func (s *Scheduler) scheduleTodayLocked() error {
	now := s.clock().In(s.tz)
	loc := s.cfg.Prayer.Location

	times, err := s.fetcher.Fetch(loc, now, false)
	if err != nil {
		return fmt.Errorf("fetch prayer times: %w", err)
	}

	type job struct {
		prayer string
		label  string
		kind   string
		at     time.Time
	}

	prayers := []struct{ name, t string }{
		{"Fajr", times.Fajr},
		{"Dhuhr", times.Dhuhr},
		{"Asr", times.Asr},
		{"Maghrib", times.Maghrib},
		{"Isha", times.Isha},
	}

	var jobs []job
	for _, p := range prayers {
		// Skip muted prayers
		if !s.cfg.Prayer.Enabled.IsEnabled(p.name) {
			log.Printf("[scheduler] %s is muted — skipping", p.name)
			continue
		}
		h, m, err := parseHHMM(p.t)
		if err != nil {
			log.Printf("[scheduler] bad time for %s: %v", p.name, err)
			continue
		}
		kind := "regular_athan"
		if p.name == "Fajr" {
			kind = "fajr_athan"
		}
		day, label := now.Day(), p.name+" Athan"
		if p.name == "Isha" && ishaAfterMidnight(times) {
			// This row's Isha ends the evening, so it falls on the next calendar day.
			day, label = day+1, p.name+" Athan (after midnight)"
		}
		jobs = append(jobs, job{
			prayer: p.name,
			label:  label,
			kind:   kind,
			at:     time.Date(now.Year(), now.Month(), day, h, m, 0, 0, s.tz),
		})
	}

	// Last night's Isha, if it falls after midnight, is still to come today. It
	// must be scheduled here too: the daily refresh discards timers set
	// yesterday, so otherwise it would be lost.
	if s.cfg.Prayer.Enabled.IsEnabled("Isha") {
		if prev, err := s.fetcher.Fetch(loc, now.AddDate(0, 0, -1), false); err == nil && ishaAfterMidnight(prev) {
			if h, m, err := parseHHMM(prev.Isha); err == nil {
				at := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, s.tz)
				if at.After(now) {
					jobs = append(jobs, job{
						prayer: "Isha",
						label:  "Isha Athan (after midnight)",
						kind:   "regular_athan",
						at:     at,
					})
				}
			}
		}
	}

	// Pre-Fajr Quran
	if s.cfg.Prayer.PreFajrEnabled && times.Fajr != "" {
		h, m, err := parseHHMM(times.Fajr)
		if err == nil {
			fajrAt := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, s.tz)
			jobs = append(jobs, job{
				prayer: "pre_fajr",
				label:  "Pre-Fajr Quran",
				kind:   "quran_recitation",
				at:     fajrAt.Add(-time.Duration(s.cfg.Prayer.PreFajrMinutes) * time.Minute),
			})
		}
	}

	// Friday Surah Al-Kahf (2 hours before Dhuhr on Fridays)
	if s.cfg.Prayer.FridayKahfEnabled && now.Weekday() == time.Friday && times.Dhuhr != "" {
		h, m, err := parseHHMM(times.Dhuhr)
		if err == nil {
			dhuhrAt := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, s.tz)
			jobs = append(jobs, job{
				prayer: "friday_kahf",
				label:  "Friday Surah Al-Kahf",
				kind:   "friday_kahf",
				at:     dhuhrAt.Add(-2 * time.Hour),
			})
		}
	}

	sort.Slice(jobs, func(i, k int) bool { return jobs[i].at.Before(jobs[k].at) })

	s.todayJobs = nil
	s.timers = nil

	for _, j := range jobs {
		status := "done"
		if j.at.After(now) {
			status = "upcoming"
		}

		js := JobStatus{
			Label:  j.label,
			Prayer: j.prayer,
			Type:   j.kind,
			Time:   j.at.Format("15:04"),
			Status: status,
		}
		if status == "upcoming" {
			js.NextRunAt = j.at.Format(time.RFC3339)
		}
		s.todayJobs = append(s.todayJobs, js)

		if status != "upcoming" {
			log.Printf("[scheduler] skipping %s at %s (past)", j.label, j.at.Format("15:04"))
			continue
		}

		delay := j.at.Sub(now)
		prayer := j.prayer
		label := j.label
		idx := len(s.todayJobs) - 1

		timer := time.AfterFunc(delay, func() {
			log.Printf("[scheduler] firing %s", label)
			trace.From(trace.SourceSystem, "prayer-fired", "", "the scheduler fired "+label)
			s.fire(prayer)
			s.mu.Lock()
			if idx < len(s.todayJobs) {
				s.todayJobs[idx].Status = "done"
				s.todayJobs[idx].NextRunAt = ""
			}
			s.mu.Unlock()
		})
		s.timers = append(s.timers, timer)
		log.Printf("[scheduler] scheduled %s at %s (in %.0f min)", j.label, j.at.Format("15:04"), delay.Minutes())

		// Start the Journal's recording of this job ahead of time.
		if s.sessionStart != nil {
			start, at := s.sessionStart, j.at
			if d := sessionStartDelay(delay); d > 0 {
				s.timers = append(s.timers, time.AfterFunc(d, func() { start(prayer, label, at) }))
			} else {
				go start(prayer, label, at)
			}
		}

		// Schedule a pre-connect warmup warmupLeadTime before every job so
		// the speaker connection is already established when the event fires.
		if s.warmup != nil {
			if warmupDelay := delay - warmupLeadTime; warmupDelay > 0 {
				warmupLabel := label
				warmupTimer := time.AfterFunc(warmupDelay, func() {
					log.Printf("[scheduler] pre-connect warmup for %s", warmupLabel)
					trace.From(trace.SourceSystem, "warmup", "", "pre-connect warm-up for "+warmupLabel)
					if err := s.warmup(); err != nil {
						log.Printf("[scheduler] warmup failed for %s: %v", warmupLabel, err)
					}
				})
				s.timers = append(s.timers, warmupTimer)
				log.Printf("[scheduler] warmup scheduled for %s at %s",
					j.label, j.at.Add(-warmupLeadTime).Format("15:04:05"))
			}
		}

		// Schedule TV mute muteLeadTime before each Athan so TVs are silent
		// when the Athan starts playing.
		if s.mute != nil && (j.kind == "fajr_athan" || j.kind == "regular_athan") {
			if muteDelay := delay - muteLeadTime; muteDelay > 0 {
				muteLabel := label
				muteTimer := time.AfterFunc(muteDelay, func() {
					log.Printf("[scheduler] muting TVs for %s", muteLabel)
					trace.From(trace.SourceSystem, "tv-mute-due", "", "time to mute the TVs for "+muteLabel)
					if err := s.mute(); err != nil {
						log.Printf("[scheduler] mute failed for %s: %v", muteLabel, err)
					}
				})
				s.timers = append(s.timers, muteTimer)
				log.Printf("[scheduler] TV mute scheduled for %s at %s",
					j.label, j.at.Add(-muteLeadTime).Format("15:04:05"))
			}
		}
	}

	return nil
}

// ishaAfterMidnight reports whether this row's Isha is a time after midnight.
// At Ireland's latitude the evening twilight lasts past midnight in late
// spring and summer, so timetables list Isha as e.g. "00:17" or "01:44": a clock
// time earlier than Maghrib. It then belongs to the *next* calendar day.
func ishaAfterMidnight(t Times) bool {
	mh, mm, err1 := parseHHMM(t.Maghrib)
	ih, im, err2 := parseHHMM(t.Isha)
	return err1 == nil && err2 == nil && ih*60+im < mh*60+mm
}

func (s *Scheduler) fire(prayer string) {
	switch prayer {
	case "pre_fajr":
		if s.playQuran != nil {
			dur := s.cfg.Prayer.PreFajrMinutes*60 - 10 // stop 10 s before Fajr to avoid killing the Athan
			if err := s.playQuran(dur); err != nil {
				log.Printf("[scheduler] pre-fajr quran error: %v", err)
			}
		}
	case "friday_kahf":
		if s.playKahf != nil {
			if err := s.playKahf(); err != nil {
				log.Printf("[scheduler] friday kahf error: %v", err)
			}
		}
	default:
		if s.playAthan != nil {
			filename := s.cfg.Prayer.Media.FileFor(prayer)
			if err := s.playAthan(prayer, filename); err != nil {
				log.Printf("[scheduler] athan error for %s: %v", prayer, err)
			}
		}
	}
}

func (s *Scheduler) cancelTimersLocked() {
	for _, t := range s.timers {
		t.Stop()
	}
	s.timers = nil
	if s.stopQuran != nil {
		s.stopQuran()
	}
	if s.stopKahf != nil {
		s.stopKahf()
	}
}

func parseHHMM(t string) (hour, minute int, err error) {
	_, err = fmt.Sscanf(t, "%d:%d", &hour, &minute)
	return
}
