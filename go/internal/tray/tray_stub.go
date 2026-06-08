//go:build !cgo

// Stub tray for CGO_ENABLED=0 cross-compiled builds (e.g. building Windows/macOS
// binaries from Linux CI). On the actual target machine, the binary is rebuilt
// with CGO enabled so the real system tray is available.

package tray

import (
	"log"
	"time"
)

// Config is the tray configuration (same shape as the desktop version).
type Config struct {
	Port       int
	Version    string
	NextPrayer func() (name string, at time.Time, ok bool)
	OnQuit     func()

	QuranStatus        func() (speakerActive, localActive bool)
	StreamQuranSpeaker func() error
	StopQuranSpeaker   func()
	StreamQuranLocal   func() error
	StopQuranLocal     func()

	CheckUpdate func()
	Uninstall   func()
}

// Run is a no-op in CGO_ENABLED=0 builds. The agent still runs fully —
// prayer scheduling, Chromecast playback, and the web dashboard all work.
// Only the system tray icon is absent.
func Run(cfg Config) {
	log.Println("[tray] system tray not available in this build (CGO_ENABLED=0)")
	// Block forever so callers don't exit unexpectedly
	select {}
}
