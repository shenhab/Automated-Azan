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

	mu            sync.Mutex
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
		c.speakerCancel() // stop any previous stream first
	}
	ctx, cancel := c.newCtx(dur)
	c.speakerCancel = cancel
	c.speakerActive = true
	c.mu.Unlock()

	log.Printf("[quran] starting speaker stream (dur=%v)", dur)
	go func() {
		defer c.clearSpeaker()
		if err := c.castMgr.PlayURL(StreamURL, "audio/mpeg"); err != nil {
			log.Printf("[quran] speaker stream error: %v", err)
			return
		}
		<-ctx.Done()
		c.castMgr.StopPlayback()
		log.Println("[quran] speaker stream stopped")
	}()
	return nil
}

// StopSpeaker cancels the active speaker stream, if any.
func (c *Controller) StopSpeaker() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.speakerCancel != nil {
		c.speakerCancel()
	}
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

// StopLocal cancels the active local stream, if any.
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
	defer c.mu.Unlock()
	if c.speakerCancel != nil {
		c.speakerCancel()
	}
	if c.localCancel != nil {
		c.localCancel()
	}
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

func (c *Controller) clearSpeaker() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.speakerActive = false
	c.speakerCancel = nil
}

func (c *Controller) clearLocal() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localActive = false
	c.localCancel = nil
}
