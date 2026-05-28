//go:build cgo

package tray

import (
	_ "embed"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"

	"github.com/getlantern/systray"
)

//go:embed azan.ico
var iconData []byte

// Config holds what the tray needs to show status and control streams.
type Config struct {
	Port       int
	NextPrayer func() (name string, at time.Time, ok bool)
	OnQuit     func()

	// Quran streaming controls. When QuranStatus is nil the Quran menu items
	// are hidden entirely (e.g. in stub / no-CGO builds).
	QuranStatus        func() (speakerActive, localActive bool)
	StreamQuranSpeaker func() error
	StopQuranSpeaker   func()
	StreamQuranLocal   func() error
	StopQuranLocal     func()
}

// Run starts the system tray. This call blocks — it must be the last call
// in main() or run on the main goroutine (required by some OS backends).
func Run(cfg Config) {
	systray.Run(
		func() { onReady(cfg) },
		func() {
			if cfg.OnQuit != nil {
				cfg.OnQuit()
			}
		},
	)
}

func onReady(cfg Config) {
	systray.SetTooltip("Automated Azan Agent")
	if len(iconData) > 0 {
		systray.SetIcon(iconData)
	} else {
		systray.SetTitle("Azan") // fallback text when icon fails
	}

	mStatus := systray.AddMenuItem("● Running", "Azan Agent is running")
	mStatus.Disable()

	systray.AddSeparator()

	mNext := systray.AddMenuItem("Next prayer: loading...", "")
	mNext.Disable()

	// Quran streaming items — only added when Quran controls are wired up.
	// A nil channel in a select case is never chosen, so these are safe to
	// declare even when the menu items aren't created.
	var quranSpeakerCh, quranLocalCh <-chan struct{}
	var mQuranSpeaker, mQuranLocal *systray.MenuItem

	if cfg.QuranStatus != nil {
		systray.AddSeparator()
		mQuranSpeaker = systray.AddMenuItem("▶  Quran on Speaker", "Stream Quran on Google speaker")
		mQuranLocal   = systray.AddMenuItem("▶  Quran on This PC", "Stream Quran on this machine's audio")
		quranSpeakerCh = mQuranSpeaker.ClickedCh
		quranLocalCh   = mQuranLocal.ClickedCh

		// Prime the labels with current state immediately
		refreshQuranItems(mQuranSpeaker, mQuranLocal, cfg.QuranStatus)
	}

	systray.AddSeparator()
	mDashboard := systray.AddMenuItem("Open Dashboard", fmt.Sprintf("http://localhost:%d", cfg.Port))
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Exit", "Stop Azan Agent and exit")

	// Background refresh loop
	go func() {
		pollInterval := time.Minute
		if cfg.QuranStatus != nil {
			pollInterval = 30 * time.Second // faster when Quran state can change
		}
		for {
			time.Sleep(pollInterval)
			updateNextPrayer(mNext, cfg.NextPrayer)
			if cfg.QuranStatus != nil {
				refreshQuranItems(mQuranSpeaker, mQuranLocal, cfg.QuranStatus)
			}
		}
	}()

	for {
		select {
		case <-quranSpeakerCh:
			handleQuranClick(mQuranSpeaker, true, cfg)
		case <-quranLocalCh:
			handleQuranClick(mQuranLocal, false, cfg)
		case <-mDashboard.ClickedCh:
			openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Port))
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func refreshQuranItems(speaker, local *systray.MenuItem, status func() (bool, bool)) {
	spkActive, locActive := status()
	if spkActive {
		speaker.SetTitle("■  Stop Quran on Speaker")
	} else {
		speaker.SetTitle("▶  Quran on Speaker")
	}
	if locActive {
		local.SetTitle("■  Stop Quran on This PC")
	} else {
		local.SetTitle("▶  Quran on This PC")
	}
}

func handleQuranClick(item *systray.MenuItem, isSpeaker bool, cfg Config) {
	if cfg.QuranStatus == nil {
		return
	}
	spkActive, locActive := cfg.QuranStatus()
	active := isSpeaker && spkActive || !isSpeaker && locActive

	if active {
		if isSpeaker {
			cfg.StopQuranSpeaker()
			item.SetTitle("▶  Quran on Speaker")
		} else {
			cfg.StopQuranLocal()
			item.SetTitle("▶  Quran on This PC")
		}
	} else {
		var err error
		if isSpeaker {
			err = cfg.StreamQuranSpeaker()
			if err == nil {
				item.SetTitle("■  Stop Quran on Speaker")
			}
		} else {
			err = cfg.StreamQuranLocal()
			if err == nil {
				item.SetTitle("■  Stop Quran on This PC")
			}
		}
		if err != nil {
			log.Printf("[tray] quran start error: %v", err)
		}
	}
}

func updateNextPrayer(item *systray.MenuItem, fn func() (string, time.Time, bool)) {
	if fn == nil {
		return
	}
	name, at, ok := fn()
	if !ok {
		item.SetTitle("No prayer scheduled")
		return
	}
	item.SetTitle(fmt.Sprintf("Next: %s at %s", name, at.Format("15:04")))
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("[tray] failed to open browser: %v", err)
	}
}
