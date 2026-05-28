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

// Config holds what the tray needs to show status.
type Config struct {
	Port         int
	NextPrayer   func() (name string, at time.Time, ok bool)
	OnQuit       func()
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

	systray.AddSeparator()

	mDashboard := systray.AddMenuItem("Open Dashboard", fmt.Sprintf("http://localhost:%d", cfg.Port))
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop Azan Agent")

	// Update next prayer display every minute
	go func() {
		for {
			updateNextPrayer(mNext, cfg.NextPrayer)
			time.Sleep(time.Minute)
		}
	}()

	for {
		select {
		case <-mDashboard.ClickedCh:
			openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Port))
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
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
