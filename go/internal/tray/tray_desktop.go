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
	Version    string // shown in the status item, e.g. "v1.1.0"; omit for dev builds
	NextPrayer func() (name string, at time.Time, ok bool)
	OnQuit     func()

	// Quran streaming controls. When QuranStatus is nil the Quran menu items
	// are hidden entirely (e.g. in stub / no-CGO builds).
	QuranStatus        func() (speakerActive, localActive bool)
	StreamQuranSpeaker func() error
	StopQuranSpeaker   func()
	StreamQuranLocal   func() error
	StopQuranLocal     func()

	// CheckUpdate, when non-nil, adds a "Check for Update" menu item.
	// Returns (newVersion, releaseURL, hasUpdate, err).
	CheckUpdate func() (newVersion, url string, hasUpdate bool, err error)

	// Uninstall, when non-nil, adds an "Uninstall…" menu item.
	// The function must handle confirmation dialog, service teardown, and exit.
	Uninstall func()
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

	statusLabel := "● Running"
	if cfg.Version != "" && cfg.Version != "dev" {
		statusLabel = fmt.Sprintf("● Running  %s", cfg.Version)
	}
	mStatus := systray.AddMenuItem(statusLabel, "Azan Agent is running")
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
		refreshQuranItems(mQuranSpeaker, mQuranLocal, cfg.QuranStatus)
	}

	systray.AddSeparator()
	mDashboard := systray.AddMenuItem("Open Dashboard", fmt.Sprintf("http://localhost:%d", cfg.Port))

	// Optional: Check for Update
	var updateCh <-chan struct{}
	if cfg.CheckUpdate != nil {
		systray.AddSeparator()
		mUpdate := systray.AddMenuItem("Check for Update", "Check GitHub for a newer release")
		updateCh = mUpdate.ClickedCh
	}

	// Optional: Uninstall
	var uninstallCh <-chan struct{}
	if cfg.Uninstall != nil {
		systray.AddSeparator()
		mUninstall := systray.AddMenuItem("Uninstall…", "Remove Azan Agent from this machine")
		uninstallCh = mUninstall.ClickedCh
	}

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Exit", "Stop Azan Agent and exit")

	// Background refresh loop
	go func() {
		pollInterval := time.Minute
		if cfg.QuranStatus != nil {
			pollInterval = 30 * time.Second
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
		case <-updateCh:
			go handleCheckUpdate(cfg.CheckUpdate, cfg.Port)
		case <-uninstallCh:
			cfg.Uninstall()
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func handleCheckUpdate(checkFn func() (string, string, bool, error), port int) {
	newVer, url, hasUpdate, err := checkFn()
	if err != nil {
		log.Printf("[tray] update check error: %v", err)
		return
	}
	if !hasUpdate {
		log.Printf("[tray] already on latest version")
		return
	}
	log.Printf("[tray] update available: %s — opening %s", newVer, url)
	openBrowser(url)
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
