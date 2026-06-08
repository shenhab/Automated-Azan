// Package quran manages manual and scheduled Quran streaming on both the
// Google/Chromecast speaker and the local machine's audio output.
// A single Controller is shared between the scheduler (pre-Fajr timed streams)
// and the web API / tray icon (manual, indefinite streams).
package quran

import (
	"context"
	"log"
	"sync"
	"time"

	"azan-agent/internal/chromecast"
	"azan-agent/internal/localplay"
)

const StreamURL = "https://backup.qurango.net/radio/mahmoud_khalil_alhussary_warsh"

// Controller tracks and controls Quran streaming on speaker and local audio.
type Controller struct {
	castMgr *chromecast.Manager

	mu          sync.Mutex
	speakerGen  uint64 // incremented on every StartSpeaker* call
	speakerActive bool
	speakerCancel context.CancelFunc
	localActive   bool
	localCancel   context.CancelFunc
}

// New returns a Controller backed by the given Chromecast manager.
func New(castMgr *chromecast.Manager) *Controller {
	return &Controller{castMgr: castMgr}
}

// StartSpeaker starts (or restarts) the Quran stream on the Chromecast speaker.
// dur > 0 stops the stream automatically after that duration; dur == 0 runs
// until StopSpeaker or Stop is called.
func (c *Controller) StartSpeaker(dur time.Duration) error {
	c.mu.Lock()
	if c.speakerCancel != nil {
		c.speakerCancel()
	}
	c.speakerGen++
	myGen := c.speakerGen
	ctx, cancel := c.newCtx(dur)
	c.speakerCancel = cancel
	c.speakerActive = true
	c.mu.Unlock()

	log.Printf("[quran] starting speaker stream (dur=%v)", dur)
	go func() {
		defer c.clearSpeakerIfGen(myGen)

		if superseded(ctx) {
			return
		}
		if err := c.castMgr.PlayURL(StreamURL, "audio/mpeg"); err != nil {
			log.Printf("[quran] speaker stream error: %v", err)
			return
		}
		log.Println("[quran] speaker stream started")
		<-ctx.Done()
		// Only send Stop to Chromecast when the timer expired naturally.
		// StopSpeaker/Stop call StopPlayback explicitly; a replacement call
		// must not kill the new stream.
		if ctx.Err() == context.DeadlineExceeded {
			c.castMgr.StopPlayback()
		}
		log.Println("[quran] speaker stream stopped")
	}()
	return nil
}

// StartSpeakerOnDevice starts the Quran stream on a specific named Chromecast
// device, regardless of the manager's configured default group.
func (c *Controller) StartSpeakerOnDevice(deviceName string, dur time.Duration) error {
	c.mu.Lock()
	if c.speakerCancel != nil {
		c.speakerCancel()
	}
	c.speakerGen++
	myGen := c.speakerGen
	ctx, cancel := c.newCtx(dur)
	c.speakerCancel = cancel
	c.speakerActive = true
	c.mu.Unlock()

	log.Printf("[quran] starting stream on device %q (dur=%v)", deviceName, dur)
	go func() {
		defer c.clearSpeakerIfGen(myGen)

		// Bail immediately if a newer call already superseded this one
		// (can happen when rapid clicks pile up during mDNS discovery).
		if superseded(ctx) {
			return
		}
		if err := c.castMgr.PlayURLOnDevice(deviceName, StreamURL, "audio/mpeg"); err != nil {
			log.Printf("[quran] device stream error (%s): %v", deviceName, err)
			return
		}
		log.Printf("[quran] stream started on device %q", deviceName)
		<-ctx.Done()
		// Only send Stop when this timed stream expired naturally.
		// When cancelled (by StopSpeaker or a replacement call), the caller
		// is responsible for stopping — otherwise we'd kill a newer stream.
		if ctx.Err() == context.DeadlineExceeded {
			c.castMgr.StopPlayback()
		}
		log.Printf("[quran] stream stopped on device %q", deviceName)
	}()
	return nil
}

// StopSpeaker cancels the active speaker stream and stops Chromecast playback.
func (c *Controller) StopSpeaker() {
	c.mu.Lock()
	if c.speakerCancel != nil {
		c.speakerCancel()
		c.speakerCancel = nil
	}
	c.mu.Unlock()
	c.castMgr.StopPlayback()
}

// StartLocal starts (or restarts) the Quran stream on the local audio output.
// dur > 0 stops it automatically; dur == 0 runs until StopLocal or Stop.
func (c *Controller) StartLocal(dur time.Duration) error {
	c.mu.Lock()
	if c.localCancel != nil {
		c.localCancel()
	}
	ctx, cancel := c.newCtx(dur)
	c.localCancel = cancel
	c.localActive = true
	c.mu.Unlock()

	log.Printf("[quran] starting local stream (dur=%v)", dur)
	go func() {
		defer c.clearLocal()
		stopFn, err := localplay.StartStream(StreamURL)
		if err != nil {
			log.Printf("[quran] local stream error: %v", err)
			return
		}
		<-ctx.Done()
		stopFn()
		log.Println("[quran] local stream stopped")
	}()
	return nil
}

// StopLocal cancels the active local stream.
func (c *Controller) StopLocal() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.localCancel != nil {
		c.localCancel()
	}
}

// Stop cancels both speaker and local streams.
func (c *Controller) Stop() {
	c.mu.Lock()
	if c.speakerCancel != nil {
		c.speakerCancel()
		c.speakerCancel = nil
	}
	if c.localCancel != nil {
		c.localCancel()
		c.localCancel = nil
	}
	c.mu.Unlock()
	c.castMgr.StopPlayback()
}

// Status returns whether each stream is currently active.
func (c *Controller) Status() (speakerActive, localActive bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.speakerActive, c.localActive
}

// --- helpers ---

func (c *Controller) newCtx(dur time.Duration) (context.Context, context.CancelFunc) {
	if dur > 0 {
		return context.WithTimeout(context.Background(), dur)
	}
	return context.WithCancel(context.Background())
}

// clearSpeakerIfGen only resets speaker state when myGen is still the current
// generation — superseded goroutines must not clear a newer stream's state.
func (c *Controller) clearSpeakerIfGen(myGen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.speakerGen == myGen {
		c.speakerActive = false
		c.speakerCancel = nil
	}
}

func (c *Controller) clearLocal() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localActive = false
	c.localCancel = nil
}

// superseded reports whether ctx is already done (cancelled by a newer call).
func superseded(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
