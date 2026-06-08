package prayer

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"azan-agent/internal/config"
	"azan-agent/internal/timesync"

	"github.com/robfig/cron/v3"
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
)

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

	mu        sync.Mutex
	todayJobs []JobStatus
	timers    []*time.Timer
	cron      *cron.Cron
	running   bool

	// quran/kahf stop channels
	quranStop chan struct{}
	kahfStop  chan struct{}
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
// 01:00 refresh cron.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	if err := s.scheduleTodayLocked(); err != nil {
		return err
	}

	s.cron = cron.New(cron.WithLocation(s.tz))
	s.cron.AddFunc("0 1 * * *", func() { // 01:00 daily
		log.Println("[scheduler] daily refresh triggered")
		timesync.Sync()
		s.Reschedule()
	})
	s.cron.Start()
	s.running = true
	log.Println("[scheduler] started")
	return nil
}

// Stop cancels all pending timers and the cron job.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelTimersLocked()
	if s.cron != nil {
		s.cron.Stop()
	}
	s.running = false
	log.Println("[scheduler] stopped")
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
	now := time.Now().In(s.tz)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.todayJobs {
		if j.Status != "upcoming" {
			continue
		}
		t, err := time.ParseInLocation("15:04", j.Time, s.tz)
		if err != nil {
			continue
		}
		at = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, s.tz)
		if at.After(now) {
			return j.Prayer, at, true
		}
	}
	return "", time.Time{}, false
}

// --- internal ---

func (s *Scheduler) scheduleTodayLocked() error {
	now := time.Now().In(s.tz)
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
		jobs = append(jobs, job{
			prayer: p.name,
			label:  p.name + " Athan",
			kind:   kind,
			at:     time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, s.tz),
		})
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

		delay := time.Until(j.at)
		prayer := j.prayer
		label := j.label
		idx := len(s.todayJobs) - 1

		timer := time.AfterFunc(delay, func() {
			log.Printf("[scheduler] firing %s", label)
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

		// Schedule a pre-connect warmup warmupLeadTime before every job so
		// the speaker connection is already established when the event fires.
		if s.warmup != nil {
			if warmupDelay := delay - warmupLeadTime; warmupDelay > 0 {
				warmupLabel := label
				warmupTimer := time.AfterFunc(warmupDelay, func() {
					log.Printf("[scheduler] pre-connect warmup for %s", warmupLabel)
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

func (s *Scheduler) fire(prayer string) {
	switch prayer {
	case "pre_fajr":
		if s.playQuran != nil {
			dur := s.cfg.Prayer.PreFajrMinutes * 60
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
