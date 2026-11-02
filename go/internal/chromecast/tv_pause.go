package chromecast

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"

	"azan-agent/internal/trace"
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

// How the TVs are released once the Athan is playing.
var (
	// releaseSettle is the pause between the Athan ending and the TVs coming back,
	// so the last words are not cut off by a TV's sound returning.
	releaseSettle = 2 * time.Second
	// releasePoll is how often the speaker is asked what it is doing.
	releasePoll = time.Second
	// maxAthanHold is the longest the TVs are held after the Athan starts, however
	// the speaker behaves: no failure may leave a TV muted for good.
	maxAthanHold = 15 * time.Minute
)

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
				trace.Error(trace.SourceTV, "tv-error", dev.Name, "could not reach "+dev.Name+" to mute it: "+err.Error(), "host", dev.Host)
				return
			}
			pd := pausedDevice{device: dev, app: app}

			media, vol := app.Media(), app.Volume()
			playing := media != nil && (media.PlayerState == "PLAYING" || media.PlayerState == "BUFFERING")
			switch {
			case playing:
				trace.From(trace.SourceTV, "pause-sent", dev.Name, "pausing "+dev.Name+" (it is playing something)", "host", dev.Host)
				if err := app.Pause(); err != nil {
					log.Printf("[tv-pause] pause %q failed: %v", dev.Name, err)
					trace.Error(trace.SourceTV, "tv-error", dev.Name, "could not pause "+dev.Name+": "+err.Error(), "host", dev.Host)
					app.Close(false)
					return
				}
				pd.paused = true
				log.Printf("[tv-pause] paused %q (%s)", dev.Name, dev.ModelName)
				trace.From(trace.SourceTV, "tv-held", dev.Name, "paused "+dev.Name+" for the Athan", "host", dev.Host, "mode", "paused")
			case vol != nil && vol.Muted:
				// Muted by its owner: nothing for us to do, nothing to undo.
				log.Printf("[tv-pause] %q is already muted — leaving it alone", dev.Name)
				trace.From(trace.SourceTV, "tv-skipped", dev.Name, dev.Name+" is already muted — left alone", "host", dev.Host)
				app.Close(false)
				return
			default:
				trace.From(trace.SourceTV, "mute-sent", dev.Name, "muting "+dev.Name, "host", dev.Host)
				if err := app.SetMuted(true); err != nil {
					log.Printf("[tv-pause] mute %q failed: %v", dev.Name, err)
					trace.Error(trace.SourceTV, "tv-error", dev.Name, "could not mute "+dev.Name+": "+err.Error(), "host", dev.Host)
					app.Close(false)
					return
				}
				pd.muted = true
				log.Printf("[tv-pause] muted %q (%s)", dev.Name, dev.ModelName)
				trace.From(trace.SourceTV, "tv-held", dev.Name, "muted "+dev.Name+" for the Athan", "host", dev.Host, "mode", "muted")
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
				trace.Error(trace.SourceTV, "tv-error", pd.device.Name, "could not reach "+pd.device.Name+" to give its sound back: "+err.Error(), "host", pd.device.Host)
				return
			}
			defer func() {
				time.Sleep(500 * time.Millisecond) // let the command go out before closing
				app.Close(false)
			}()
			switch {
			case pd.muted:
				releaseMute(app, pd.device)
			case pd.paused:
				trace.From(trace.SourceTV, "play-sent", pd.device.Name, "resuming "+pd.device.Name, "host", pd.device.Host)
				if err := app.Unpause(); err != nil {
					log.Printf("[tv-pause] unpause %q failed: %v", pd.device.Name, err)
					trace.Error(trace.SourceTV, "tv-error", pd.device.Name, "could not resume "+pd.device.Name+": "+err.Error(), "host", pd.device.Host)
					return
				}
				log.Printf("[tv-pause] resumed %q", pd.device.Name)
				trace.From(trace.SourceTV, "tv-released", pd.device.Name, "resumed "+pd.device.Name, "host", pd.device.Host)
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

// Releasing a mute must not be a blind "unmute". On an NVIDIA SHIELD (measured
// 2026-11-02) a Cast mute is a toggle key press: its owner can unmute with the
// remote during the Athan while the Cast session still says "muted", and the
// old end-of-Athan unmute then silenced the TV again. A volume change is not a
// toggle, so the release is a volume nudge: a small move away from the current
// level and straight back. If the TV was still muted by us the nudge unmutes
// it; if its owner had already unmuted it, the volume ends up where it was.
// Only a device that still reports itself muted after the nudge gets an
// explicit unmute (devices where the nudge does not clear the flag).
const (
	volumeNudge       = 0.10 // the same for every TV; bigger than any volume step seen on a device
	volumeNudgeSettle = 500 * time.Millisecond
)

func releaseMute(app *application.Application, dev Device) {
	if err := app.Update(); err != nil {
		log.Printf("[tv-pause] %q: cannot read its state (%v) — sending a plain unmute", dev.Name, err)
		plainUnmute(app, dev)
		return
	}
	vol := app.Volume()
	level := float64(0)
	if vol != nil {
		level = float64(vol.Level)
	}
	// The library drops a level of 0 from the message, which would turn the
	// nudge into a bare mute toggle; a TV at volume 0 is silent anyway.
	if vol == nil || level <= 0 {
		plainUnmute(app, dev)
		return
	}
	target := level + volumeNudge
	if target > 1 {
		target = level - volumeNudge
	}
	if target <= 0 {
		plainUnmute(app, dev)
		return
	}
	trace.From(trace.SourceTV, "volume-set", dev.Name, fmt.Sprintf("nudging %s's volume %.2f → %.2f to give its sound back", dev.Name, level, target), "host", dev.Host)
	if err := app.SetVolume(float32(target)); err != nil {
		log.Printf("[tv-pause] volume nudge on %q failed: %v — sending a plain unmute", dev.Name, err)
		plainUnmute(app, dev)
		return
	}
	time.Sleep(volumeNudgeSettle)
	stillMuted := false
	if err := app.Update(); err == nil {
		if v := app.Volume(); v != nil {
			stillMuted = v.Muted
		}
	}
	trace.From(trace.SourceTV, "volume-set", dev.Name, fmt.Sprintf("putting %s's volume back to %.2f", dev.Name, level), "host", dev.Host)
	if err := app.SetVolume(float32(level)); err != nil {
		log.Printf("[tv-pause] restoring the volume of %q failed: %v", dev.Name, err)
		trace.Error(trace.SourceTV, "tv-error", dev.Name, "could not restore the volume of "+dev.Name+": "+err.Error(), "host", dev.Host)
	}
	if stillMuted {
		plainUnmute(app, dev)
		return
	}
	log.Printf("[tv-pause] released %q (volume %.2f restored)", dev.Name, level)
	trace.From(trace.SourceTV, "tv-released", dev.Name, fmt.Sprintf("released %s (volume %.2f restored)", dev.Name, level), "host", dev.Host)
}

func plainUnmute(app *application.Application, dev Device) {
	trace.From(trace.SourceTV, "unmute-sent", dev.Name, "unmuting "+dev.Name, "host", dev.Host)
	if err := app.SetMuted(false); err != nil {
		log.Printf("[tv-pause] unmute %q failed: %v", dev.Name, err)
		trace.Error(trace.SourceTV, "tv-error", dev.Name, "could not unmute "+dev.Name+": "+err.Error(), "host", dev.Host)
		return
	}
	log.Printf("[tv-pause] unmuted %q", dev.Name)
	trace.From(trace.SourceTV, "tv-released", dev.Name, "unmuted "+dev.Name, "host", dev.Host)
}

// ReleaseWhenAthanEnds hands the TVs back when the Athan really stops playing,
// instead of after a fixed time. Call it once the Athan has started (the
// speaker's own report, see Manager.LastPlayback). It replaces any timer set by
// ScheduleResume with a hard limit of maxAthanHold, then watches the speaker;
// releaseSettle after it stops (finished, stopped by hand, error, taken over)
// the TVs are released. Does nothing if no TV is held.
func (t *TVPauseManager) ReleaseWhenAthanEnds() {
	if !t.IsPaused() {
		return
	}
	pb := t.castMgr.LastPlayback()

	t.mu.Lock()
	if t.resumeTimer != nil {
		t.resumeTimer.Stop()
	}
	t.resumeTimer = time.AfterFunc(maxAthanHold, func() {
		log.Printf("[tv-pause] hold limit of %v reached — releasing", maxAthanHold)
		t.ResumeAfterAthan()
	})
	t.mu.Unlock()

	go func() {
		trace.From(trace.SourceTV, "release-armed", pb.Speaker, "watching the speaker; the TVs are released "+releaseSettle.String()+" after it stops")
		why := t.castMgr.WaitForPlaybackEnd(pb, maxAthanHold)
		log.Printf("[tv-pause] Athan is over (%s) — releasing TVs in %v", why, releaseSettle)
		trace.From(trace.SourceTV, "athan-over", pb.Speaker, "the Athan is over ("+why+")")
		time.Sleep(releaseSettle)
		t.ResumeAfterAthan()
	}()
}
