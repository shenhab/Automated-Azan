package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"azan-agent/internal/appdirs"
	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
	"azan-agent/internal/media"
	"azan-agent/internal/prayer"
	"azan-agent/internal/timesync"
	"azan-agent/internal/tray"
	"azan-agent/internal/web"

	"github.com/kardianos/service"
)

// binDir returns the directory containing the running binary.
// All relative paths (Media/, data/, azan.toml) are resolved from here
// so the binary works correctly regardless of the working directory.
func binDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	// Resolve symlinks (e.g. `go run` puts a temp binary in /tmp)
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return filepath.Dir(exe)
	}
	return filepath.Dir(real)
}

// program implements the kardianos/service interface so the agent can run
// as a Windows Service, macOS LaunchAgent, or systemd unit.
type program struct {
	cfg        *config.Config
	mediaSrv   *media.Server
	webSrv     *web.Server
	scheduler  *prayer.Scheduler
	castMgr    *chromecast.Manager
	cfgWatcher *config.Watcher
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cfgWatcher != nil {
		p.cfgWatcher.Stop()
	}
	if p.webSrv != nil {
		p.webSrv.Stop()
	}
	if p.mediaSrv != nil {
		p.mediaSrv.Stop()
	}
	if p.scheduler != nil {
		p.scheduler.Stop()
	}
	return nil
}

func (p *program) run() {
	cfg := config.Get()
	p.cfg = cfg

	// Use OS-standard log path unless config explicitly overrides it
	logPath := cfg.Log.FilePath
	if logPath == "" {
		logPath = filepath.Join(appdirs.Logs(), "azan.log")
	}
	setupLogging(logPath, cfg.Log.Level)
	log.Println("=== Automated Azan Agent starting ===")

	// Initial NTP sync
	if _, err := timesync.Sync(); err != nil {
		log.Printf("[main] time sync warning: %v", err)
	}

	// Ensure all OS-standard directories exist
	if err := appdirs.EnsureAll(); err != nil {
		log.Printf("[main] warning: could not create app dirs: %v", err)
	}

	resolvedDataDir  := appdirs.Data()
	resolvedMediaDir := filepath.Join(binDir(), "Media")
	log.Printf("[main] config dir: %s", appdirs.Config())
	log.Printf("[main] data dir:   %s", resolvedDataDir)
	log.Printf("[main] log dir:    %s", appdirs.Logs())
	log.Printf("[main] media dir:  %s", resolvedMediaDir)

	// Verify media directory exists and list files for debugging
	if entries, err := os.ReadDir(resolvedMediaDir); err != nil {
		log.Printf("[main] WARNING: media dir not found (%s): %v — Chromecast playback will fail", resolvedMediaDir, err)
	} else {
		log.Printf("[main] media dir contains %d file(s):", len(entries))
		for _, e := range entries {
			log.Printf("[main]   %s", e.Name())
		}
	}

	// Prayer time fetcher
	fetcher, err := prayer.NewFetcher(resolvedDataDir)
	if err != nil {
		log.Fatalf("[main] fetcher init: %v", err)
	}

	// Media server — serves MP3s on a dedicated port so Chromecast can fetch them.
	// Uses the same LAN IP as the web server; Chromecast must be able to reach this host.
	mediaSrv := media.NewServer(resolvedMediaDir, cfg.Web.Port+1)
	if err := mediaSrv.Start(); err != nil {
		log.Fatalf("[main] media server: %v", err)
	}
	p.mediaSrv = mediaSrv
	log.Printf("[main] media server: %s", mediaSrv.BaseURL())

	// Chromecast manager
	castMgr := chromecast.NewManager(cfg.Speaker.GroupName)
	castMgr.SetMediaBaseURL(mediaSrv.BaseURL())
	p.castMgr = castMgr
	log.Printf("[main] chromecast media base URL: %s", mediaSrv.BaseURL())

	// Trigger background discovery (non-blocking)
	go func() {
		devs, err := castMgr.Discover(false)
		if err != nil {
			log.Printf("[main] chromecast discovery: %v", err)
			return
		}
		log.Printf("[main] found %d chromecast device(s)", len(devs))
	}()

	// Quran stream state
	quranStop := make(chan struct{}, 1)

	playAthan := func(prayerName string) error {
		return castMgr.PlayAthan(prayerName)
	}

	playQuran := func(durationSec int) error {
		const quranURL = "https://backup.qurango.net/radio/mahmoud_khalil_alhussary_warsh"
		log.Printf("[main] starting pre-Fajr Quran stream (%d seconds)", durationSec)
		go func() {
			if err := castMgr.PlayURL(quranURL, "audio/mpeg"); err != nil {
				log.Printf("[main] quran stream error: %v", err)
				return
			}
			timer := time.NewTimer(time.Duration(durationSec) * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-quranStop:
			}
			castMgr.StopPlayback()
		}()
		return nil
	}

	stopQuran := func() {
		select {
		case quranStop <- struct{}{}:
		default:
		}
	}

	playKahf := func() error {
		const kahfURL = "https://server13.mp3quran.net/husr/018.mp3"
		log.Println("[main] playing Friday Surah Al-Kahf")
		return castMgr.PlayURL(kahfURL, "audio/mpeg")
	}

	// Prayer scheduler
	sched := prayer.NewScheduler(cfg, fetcher, playAthan, playQuran, stopQuran, playKahf, nil)
	if err := sched.Start(); err != nil {
		log.Fatalf("[main] scheduler start: %v", err)
	}
	p.scheduler = sched

	// Web server
	webSrv, err := web.NewServer(cfg, fetcher, sched, castMgr)
	if err != nil {
		log.Fatalf("[main] web server init: %v", err)
	}
	if err := webSrv.Start(); err != nil {
		log.Fatalf("[main] web server start: %v", err)
	}
	p.webSrv = webSrv
	dashURL := fmt.Sprintf("http://localhost:%d", webSrv.Port())
	log.Printf("[main] dashboard at %s", dashURL)

	// When running interactively (terminal / double-click), open the dashboard
	// in the default browser. Wait 1 second so the HTTP server is ready first.
	if service.Interactive() {
		go func() {
			time.Sleep(time.Second)
			openBrowser(dashURL)
		}()
	}

	// Config hot-reload watcher
	watcher := config.NewWatcher(cfg)
	watcher.OnChange(func(old, new config.Config) {
		log.Println("[main] config changed — rescheduling")

		// Update chromecast target if speaker changed
		if old.Speaker.GroupName != new.Speaker.GroupName {
			castMgr := chromecast.NewManager(new.Speaker.GroupName)
			castMgr.SetMediaBaseURL(mediaSrv.BaseURL())
			p.castMgr = castMgr
		}

		// Reschedule prayers
		sched.Reschedule()

		// Push update to WebSocket clients
		webSrv.Broadcast("config_update", new.AsWebDict())
	})
	if err := watcher.Start(); err != nil {
		log.Printf("[main] config watcher: %v", err)
	}
	p.cfgWatcher = watcher

	// Print prayer times on startup
	now := time.Now().In(fetcher.Timezone())
	if times, err := fetcher.Fetch(cfg.Prayer.Location, now, false); err == nil {
		log.Printf("[main] prayer times for %s on %s:", cfg.Prayer.Location, now.Format("2006-01-02"))
		log.Printf("  Fajr=%s  Dhuhr=%s  Asr=%s  Maghrib=%s  Isha=%s",
			times.Fajr, times.Dhuhr, times.Asr, times.Maghrib, times.Isha)
	}

	if name, at, ok := sched.NextPrayer(); ok {
		log.Printf("[main] next prayer: %s at %s", name, at.Format("15:04"))
	}

	// Block forever — scheduler timers and web server run in goroutines
	select {}
}

func main() {
	svcCfg := &service.Config{
		Name:        "AzanAgent",
		DisplayName: "Automated Azan Agent",
		Description: "Plays Athan at prayer times on local Chromecast devices",
	}

	prg := &program{}
	svc, err := service.New(prg, svcCfg)
	if err != nil {
		log.Fatalf("service init: %v", err)
	}

	// Handle service control commands: install, uninstall, start, stop
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		switch cmd {
		case "install":
			fmt.Println("Installing Azan Agent service...")
			if err := svc.Install(); err != nil {
				log.Fatalf("install: %v", err)
			}
			fmt.Println("Service installed. Run: azan-agent start")
			return
		case "uninstall":
			fmt.Println("Uninstalling Azan Agent service...")
			if err := svc.Uninstall(); err != nil {
				log.Fatalf("uninstall: %v", err)
			}
			fmt.Println("Service uninstalled.")
			return
		case "start":
			if err := svc.Start(); err != nil {
				log.Fatalf("start: %v", err)
			}
			fmt.Println("Service started.")
			return
		case "stop":
			if err := svc.Stop(); err != nil {
				log.Fatalf("stop: %v", err)
			}
			fmt.Println("Service stopped.")
			return
		}
	}

	// Running interactively (double-click or terminal)
	if service.Interactive() {
		// On headless systems (no DISPLAY / no Wayland), skip the tray and
		// just run the agent directly — useful for servers and Raspberry Pi
		// without a desktop environment.
		if !hasDisplay() {
			prg.run() // blocks forever
			return
		}

		// Desktop environment detected → start agent + show system tray
		go prg.run()

		tray.Run(tray.Config{
			Port: func() int {
				if prg.webSrv != nil {
					return prg.webSrv.Port()
				}
				return config.Get().Web.Port
			}(),
			NextPrayer: func() (string, time.Time, bool) {
				if prg.scheduler == nil {
					return "", time.Time{}, false
				}
				return prg.scheduler.NextPrayer()
			},
			OnQuit: func() {
				prg.Stop(svc)
				os.Exit(0)
			},
		})
		return
	}

	// Running as a system service → no tray
	if err := svc.Run(); err != nil {
		log.Fatalf("service run: %v", err)
	}
}

// openBrowser opens url in the default browser.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd, args = "cmd", []string{"/c", "start", url}
	case "darwin":
		cmd, args = "open", []string{url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("[main] could not open browser: %v", err)
	}
}

// hasDisplay returns true if a graphical display is available.
func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func setupLogging(filePath, level string) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		log.Printf("[main] could not create log dir: %v", err)
		return
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[main] could not open log file: %v", err)
		return
	}
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}
