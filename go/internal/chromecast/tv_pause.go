package chromecast

import (
	"log"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
)

// TVPauseManager pauses Cast-capable screens/TVs during Athan and resumes
// them automatically after a configurable delay.
type TVPauseManager struct {
	castMgr *Manager

	mu          sync.Mutex
	paused      []pausedDevice
	resumeTimer *time.Timer
}

type pausedDevice struct {
	device Device
	app    *application.Application
}

// NewTVPauseManager returns a manager backed by the given Chromecast manager.
func NewTVPauseManager(mgr *Manager) *TVPauseManager {
	return &TVPauseManager{castMgr: mgr}
}

// IsPaused reports whether any devices are currently paused by this manager.
func (t *TVPauseManager) IsPaused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.paused) > 0
}

// PausedCount returns the number of devices currently paused.
func (t *TVPauseManager) PausedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.paused)
}

// PauseForAthan connects to the specified devices (or all discovered devices
// when uuids is empty), skipping any whose name matches excludeName (the Athan
// speaker), and sends a Pause command. Only devices that accept the Pause are
// tracked for resume. Per-device errors are logged but don't abort the rest.
func (t *TVPauseManager) PauseForAthan(uuids []string, excludeName string) {
	devs := t.castMgr.Devices()
	if len(devs) == 0 {
		devs, _ = t.castMgr.Discover(false)
	}
	if len(devs) == 0 {
		log.Println("[tv-pause] no Cast devices found — nothing to pause")
		return
	}

	wantAll := len(uuids) == 0
	uuidSet := make(map[string]bool, len(uuids))
	for _, u := range uuids {
		uuidSet[u] = true
	}

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		paused []pausedDevice
	)

	for _, dev := range devs {
		dev := dev
		if !wantAll && !uuidSet[dev.UUID] {
			continue
		}
		if excludeName != "" && equalFold(dev.Name, excludeName) {
			log.Printf("[tv-pause] skipping athan speaker: %q", dev.Name)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			app, err := t.castMgr.connectWithRetry(dev)
			if err != nil {
				log.Printf("[tv-pause] connect to %q failed: %v", dev.Name, err)
				return
			}
			if err := app.Pause(); err != nil {
				log.Printf("[tv-pause] pause %q skipped (likely idle): %v", dev.Name, err)
				return
			}
			log.Printf("[tv-pause] paused %q (%s)", dev.Name, dev.ModelName)
			mu.Lock()
			paused = append(paused, pausedDevice{device: dev, app: app})
			mu.Unlock()
		}()
	}
	wg.Wait()

	t.mu.Lock()
	t.paused = paused
	t.mu.Unlock()
	log.Printf("[tv-pause] %d device(s) paused for Athan", len(paused))
}

// ResumeAfterAthan unpauses all currently paused devices and cancels any
// pending auto-resume timer. Safe to call when nothing is paused.
func (t *TVPauseManager) ResumeAfterAthan() {
	t.mu.Lock()
	if t.resumeTimer != nil {
		t.resumeTimer.Stop()
		t.resumeTimer = nil
	}
	paused := t.paused
	t.paused = nil
	t.mu.Unlock()

	if len(paused) == 0 {
		return
	}

	for _, pd := range paused {
		pd := pd
		go func() {
			if err := pd.app.Unpause(); err != nil {
				// Connection may have dropped during Athan — reconnect and retry.
				log.Printf("[tv-pause] unpause %q failed, reconnecting: %v", pd.device.Name, err)
				app, err2 := t.castMgr.connectWithRetry(pd.device)
				if err2 != nil {
					log.Printf("[tv-pause] reconnect to %q failed: %v", pd.device.Name, err2)
					return
				}
				if err3 := app.Unpause(); err3 != nil {
					log.Printf("[tv-pause] unpause %q failed after reconnect: %v", pd.device.Name, err3)
					return
				}
			}
			log.Printf("[tv-pause] resumed %q", pd.device.Name)
		}()
	}
	log.Printf("[tv-pause] sending resume to %d device(s)", len(paused))
}

// ScheduleResume cancels any existing auto-resume timer and schedules a new
// one that calls ResumeAfterAthan after delay.
func (t *TVPauseManager) ScheduleResume(delay time.Duration) {
	t.mu.Lock()
	if t.resumeTimer != nil {
		t.resumeTimer.Stop()
	}
	t.resumeTimer = time.AfterFunc(delay, func() {
		log.Printf("[tv-pause] auto-resume triggered after %v", delay)
		t.ResumeAfterAthan()
	})
	t.mu.Unlock()
	log.Printf("[tv-pause] auto-resume scheduled in %v", delay)
}
