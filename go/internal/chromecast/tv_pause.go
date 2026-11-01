package chromecast

import (
	"log"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
)

// TVPauseManager quiets Cast-capable TVs/screens while the Athan plays, then puts
// them back exactly as they were.
//
// For each device it changes only what it has to and remembers only what it
// changed, so afterwards it undoes only that:
//
//   - media that is playing is paused, and resumed afterwards;
//   - an idle or paused TV is muted, and unmuted afterwards;
//   - a TV the owner had already muted, or media the owner had paused, is left
//     alone and stays as the owner left it.
type TVPauseManager struct {
	castMgr *Manager

	mu          sync.Mutex
	paused      []pausedDevice
	resumeTimer *time.Timer
}

type pausedDevice struct {
	device Device
	app    *application.Application // connection used to pause; closed on resume
	muted  bool                     // we muted it: restore with SetMuted(false)
	paused bool                     // we paused playing media: restore with Unpause
}

// NewTVPauseManager returns a manager backed by the given Chromecast manager.
func NewTVPauseManager(mgr *Manager) *TVPauseManager {
	return &TVPauseManager{castMgr: mgr}
}

// IsPaused reports whether any devices are currently held by this manager.
func (t *TVPauseManager) IsPaused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.paused) > 0
}

// PausedCount returns the number of devices currently held.
func (t *TVPauseManager) PausedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.paused)
}

func (t *TVPauseManager) alreadyHeld() map[string]bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	held := make(map[string]bool, len(t.paused))
	for _, pd := range t.paused {
		held[pd.device.UUID] = true
	}
	return held
}

// PauseForAthan pauses or mutes the given devices (by UUID; empty = all
// discovered), except the Athan speaker itself (excludeName). Devices are
// handled concurrently and one that is unreachable does not hold up the rest.
// A device this manager is already holding is left as is, so calling it again
// before a resume cannot lose the original state.
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
	held := t.alreadyHeld()

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
		if held[dev.UUID] {
			log.Printf("[tv-pause] %q is already held — leaving it as is", dev.Name)
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
			pd := pausedDevice{device: dev, app: app}

			media, vol := app.Media(), app.Volume()
			playing := media != nil && (media.PlayerState == "PLAYING" || media.PlayerState == "BUFFERING")
			switch {
			case playing:
				if err := app.Pause(); err != nil {
					log.Printf("[tv-pause] pause %q failed: %v", dev.Name, err)
					app.Close(false)
					return
				}
				pd.paused = true
				log.Printf("[tv-pause] paused %q (%s)", dev.Name, dev.ModelName)
			case vol != nil && vol.Muted:
				// Muted by its owner: nothing for us to do, nothing to undo.
				log.Printf("[tv-pause] %q is already muted — leaving it alone", dev.Name)
				app.Close(false)
				return
			default:
				if err := app.SetMuted(true); err != nil {
					log.Printf("[tv-pause] mute %q failed: %v", dev.Name, err)
					app.Close(false)
					return
				}
				pd.muted = true
				log.Printf("[tv-pause] muted %q (%s)", dev.Name, dev.ModelName)
			}
			mu.Lock()
			paused = append(paused, pd)
			mu.Unlock()
		}()
	}
	wg.Wait()

	t.mu.Lock()
	t.paused = append(t.paused, paused...)
	n := len(t.paused)
	t.mu.Unlock()
	log.Printf("[tv-pause] %d device(s) held for Athan", n)
}

// ResumeAfterAthan undoes what PauseForAthan did, and only that. It reconnects
// to each device instead of trusting the connection held since the pause: a TV
// that rebooted or dropped off Wi-Fi in the meantime must still be restored.
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
			if pd.app != nil {
				defer pd.app.Close(false)
			}
			app, err := t.castMgr.connectWithRetry(pd.device)
			if err != nil {
				log.Printf("[tv-pause] reconnect to %q failed: %v", pd.device.Name, err)
				return
			}
			defer func() {
				time.Sleep(500 * time.Millisecond) // let the command go out before closing
				app.Close(false)
			}()
			switch {
			case pd.muted:
				if err := app.SetMuted(false); err != nil {
					log.Printf("[tv-pause] unmute %q failed: %v", pd.device.Name, err)
					return
				}
				log.Printf("[tv-pause] unmuted %q", pd.device.Name)
			case pd.paused:
				if err := app.Unpause(); err != nil {
					log.Printf("[tv-pause] unpause %q failed: %v", pd.device.Name, err)
					return
				}
				log.Printf("[tv-pause] resumed %q", pd.device.Name)
			}
		}()
	}
	log.Printf("[tv-pause] sending resume to %d device(s)", len(paused))
}

// ScheduleResume resumes everything held after delay (cancelling any earlier
// scheduled resume).
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
