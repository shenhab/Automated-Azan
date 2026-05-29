package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"azan-agent/internal/appdirs"
	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
	"azan-agent/internal/media"
	"azan-agent/internal/localplay"
	"azan-agent/internal/notify"
	"azan-agent/internal/prayer"
	"azan-agent/internal/quran"
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
	quranCtrl  *quran.Controller
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

	// Quran controller — shared by scheduler (pre-Fajr) and tray/API (manual).
	quranCtrl := quran.New(castMgr)
	p.quranCtrl = quranCtrl

	fireNotify := func(title, message string) {
		if err := notify.Send(title, message); err != nil {
			log.Printf("[main] desktop notify: %v", err)
		}
	}

	playAthan := func(prayerName, filename string) error {
		ch := cfg.Prayer.Channels.ForPrayer(prayerName)
		if ch.Notify {
			go fireNotify("Automated Azan", prayerName+" prayer time")
		}
		if ch.Speaker {
			if err := castMgr.PlayAthan(prayerName, filename); err != nil {
				log.Printf("[main] speaker athan (%s): %v", prayerName, err)
			}
		}
		if ch.Local {
			go func() {
				if err := localplay.Play(filepath.Join(resolvedMediaDir, filename)); err != nil {
					log.Printf("[main] local athan (%s): %v", prayerName, err)
				}
			}()
		}
		return nil
	}

	playQuran := func(durationSec int) error {
		ch := cfg.Prayer.Channels.PreFajr
		if ch.Notify {
			go fireNotify("Automated Azan", "Pre-Fajr Quran recitation starting")
		}
		dur := time.Duration(durationSec) * time.Second
		if ch.Speaker {
			quranCtrl.StartSpeaker(dur)
		}
		if ch.Local {
			quranCtrl.StartLocal(dur)
		}
		return nil
	}

	stopQuran := quranCtrl.Stop

	playKahf := func() error {
		ch := cfg.Prayer.Channels.FridayKahf
		if ch.Notify {
			go fireNotify("Automated Azan", "Friday Surah Al-Kahf starting")
		}
		if ch.Speaker {
			log.Println("[main] playing Friday Surah Al-Kahf on speaker")
			if err := castMgr.PlayURL(mediaSrv.BaseURL()+"kahf.mp3", "audio/mpeg"); err != nil {
				log.Printf("[main] kahf speaker: %v", err)
			}
		}
		if ch.Local {
			go func() {
				if err := localplay.Play(filepath.Join(resolvedMediaDir, "kahf.mp3")); err != nil {
					log.Printf("[main] kahf local: %v", err)
				}
			}()
		}
		return nil
	}

	// Prayer scheduler
	sched := prayer.NewScheduler(cfg, fetcher, playAthan, playQuran, stopQuran, playKahf, nil)
	if err := sched.Start(); err != nil {
		log.Fatalf("[main] scheduler start: %v", err)
	}
	p.scheduler = sched

	// Web server
	webSrv, err := web.NewServer(cfg, fetcher, sched, castMgr, quranCtrl, resolvedMediaDir)
	if err != nil {
		log.Fatalf("[main] web server init: %v", err)
	}
	if err := webSrv.Start(); err != nil {
		log.Fatalf("[main] web server start: %v", err)
	}
	p.webSrv = webSrv
	log.Printf("[main] dashboard at http://localhost:%d", webSrv.Port())

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
	// Install as a user-level service on Linux and macOS so no root/admin is
	// needed: systemd user service on Linux (~/.config/systemd/user/), and a
	// LaunchAgent on macOS (~/Library/LaunchAgents/) instead of LaunchDaemon.
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		opts := service.KeyValue{"UserService": true}
		if runtime.GOOS == "darwin" {
			// Direct LaunchAgent stdout/stderr to ~/Library/Logs/AzanAgent/
			// instead of the kardianos/service default (~/AzanAgent.*.log).
			if home, err := os.UserHomeDir(); err == nil {
				logDir := filepath.Join(home, "Library", "Logs", "AzanAgent")
				_ = os.MkdirAll(logDir, 0o755)
				opts["StandardOutPath"] = filepath.Join(logDir, "service.log")
				opts["StandardErrPath"] = filepath.Join(logDir, "service.err.log")
			}
		}
		svcCfg.Option = opts
	}

	// On macOS, point the LaunchAgent at a copy of the binary that lives outside
	// the .app bundle.  Without this, launchd associates the service process with
	// AzanAgent.app; a subsequent double-click in Finder activates the service
	// process (which has no NSApplication loop) and shows "not responding."
	helperBin, helperUpdated, helperErr := ensureHelperBinary()
	if helperBin != "" {
		svcCfg.Executable = helperBin
	}
	if helperErr != nil {
		log.Printf("[main] helper binary: %v", helperErr)
	}

	prg := &program{}
	svc, err := service.New(prg, svcCfg)
	if err != nil {
		log.Fatalf("service init: %v", err)
	}

	// Handle explicit service control commands: install, uninstall, start, stop
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

	// Running interactively (double-click or terminal launch)
	if isInteractiveLaunch() {
		// All platforms: prefer running as a native service so the agent persists
		// across terminal sessions and starts automatically on login/boot.
		runAsService(prg, svc, helperUpdated)
		return
	}

	// Running as a system/user service — no tray
	if err := svc.Run(); err != nil {
		log.Fatalf("service run: %v", err)
	}
}

// isInteractiveLaunch reports whether the process was started by a user
// (double-click, terminal) rather than by the OS service manager.
//
// kardianos/service uses PPID != 1 to detect this, but on macOS both Finder
// launches and LaunchAgent launches are parented by launchd (PID 1), so
// service.Interactive() always returns false for app-bundle launches. We
// distinguish the two cases by checking the executable path instead.
func isInteractiveLaunch() bool {
	if runtime.GOOS == "darwin" {
		exe, _ := os.Executable()
		// .app bundle launch (Finder or terminal inside the bundle)
		if strings.Contains(exe, ".app/Contents/MacOS/") {
			return true
		}
		// Plain binary run from a terminal (TERM or TERM_PROGRAM is set)
		if os.Getenv("TERM") != "" || os.Getenv("TERM_PROGRAM") != "" {
			return true
		}
		return false // helper binary started by launchd as a LaunchAgent
	}
	return service.Interactive()
}

// runAsService installs (if needed) and starts the native OS service, then
// shows a tray (if a display is available) or exits cleanly (headless).
// Falls back to inline mode if the service cannot be installed or started
// (e.g. no systemd, insufficient privileges on Windows first run).
//
// Service type by platform:
//   - Linux:   systemd user service  (~/.config/systemd/user/)
//   - macOS:   LaunchAgent           (~/Library/LaunchAgents/)
//   - Windows: Windows Service (SCM) — requires admin on first install
func runAsService(prg *program, svc service.Service, helperUpdated bool) {
	port := config.Get().Web.Port

	status, err := svc.Status()
	if err != nil || status == service.StatusUnknown {
		fmt.Println("[azan] Installing service...")
		if installErr := svc.Install(); installErr != nil {
			if runtime.GOOS == "windows" && !isElevated() {
				showServiceInstallMessage()
			} else {
				log.Printf("[main] service install failed (%v) — falling back to inline mode", installErr)
			}
			runInline(prg, svc)
			return
		}
		fmt.Println("[azan] Service installed.")
		status = service.StatusStopped
	}

	if status != service.StatusRunning {
		fmt.Println("[azan] Starting service...")
		if startErr := svc.Start(); startErr != nil {
			log.Printf("[main] service start failed (%v) — falling back to inline mode", startErr)
			runInline(prg, svc)
			return
		}
		fmt.Printf("[azan] Service started. Dashboard: http://localhost:%d\n", port)
	} else if helperUpdated {
		// New binary was copied — restart service so it picks up the upgrade.
		log.Printf("[main] binary updated, restarting service...")
		_ = svc.Stop()
		time.Sleep(500 * time.Millisecond)
		if startErr := svc.Start(); startErr != nil {
			log.Printf("[main] restart failed (%v) — service may still be on old binary", startErr)
		} else {
			fmt.Printf("[azan] Service restarted (upgraded). Dashboard: http://localhost:%d\n", port)
		}
	} else {
		fmt.Printf("[azan] Service already running. Dashboard: http://localhost:%d\n", port)
	}

	if shouldShowTray() {
		runTrayForService(port, svc)
	}
	// Headless: service is running in the background — exit cleanly.
}

// runInline is the fallback when the native service cannot be installed or
// started: runs the agent in-process, shows a tray (if display) or blocks headlessly.
func runInline(prg *program, svc service.Service) {
	go prg.run()
	if shouldShowTray() {
		runTray(prg, svc)
	} else {
		runHeadless(prg)
	}
}

// runTrayForService shows the system tray for a service-mode agent.
// All data is fetched from the web API; Exit stops the service.
func runTrayForService(port int, svc service.Service) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tray] failed: %v — service continues running in background", r)
		}
	}()

	tray.Run(tray.Config{
		Port:    port,
		Version: version,
		NextPrayer: func() (string, time.Time, bool) {
			return nextPrayerFromAPI(port)
		},
		OnQuit: func() {
			if err := svc.Stop(); err != nil {
				log.Printf("[main] stop service: %v", err)
			}
			os.Exit(0)
		},
		QuranStatus: func() (bool, bool) {
			return quranStatusFromAPI(port)
		},
		StreamQuranSpeaker: func() error { return quranActionFromAPI(port, "play", "speaker") },
		StopQuranSpeaker:   func() { quranActionFromAPI(port, "stop", "speaker") },
		StreamQuranLocal:   func() error { return quranActionFromAPI(port, "play", "local") },
		StopQuranLocal:     func() { quranActionFromAPI(port, "stop", "local") },
		CheckUpdate: func() (string, string, bool, error) {
			return checkForUpdate(version)
		},
		Uninstall: func() {
			doUninstall(svc, nil)
		},
	})
}

// quranStatusFromAPI queries /api/quran/status on the running service.
func quranStatusFromAPI(port int) (speakerActive, localActive bool) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/quran/status", port))
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	var result struct {
		SpeakerActive bool `json:"speaker_active"`
		LocalActive   bool `json:"local_active"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.SpeakerActive, result.LocalActive
}

// quranActionFromAPI calls /api/quran/{action} on the running service.
// action is "play" or "stop"; target is "speaker", "local", or "all".
func quranActionFromAPI(port int, action, target string) error {
	body := fmt.Sprintf(`{"target":%q}`, target)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://localhost:%d/api/quran/%s", port, action),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// nextPrayerFromAPI queries /api/next-prayer on the running service.
func nextPrayerFromAPI(port int) (string, time.Time, bool) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/next-prayer", port))
	if err != nil {
		return "", time.Time{}, false
	}
	defer resp.Body.Close()

	var result struct {
		Found  bool   `json:"found"`
		Prayer string `json:"prayer"`
		Time   string `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.Found {
		return "", time.Time{}, false
	}

	now := time.Now()
	t, err := time.ParseInLocation("15:04", result.Time, now.Location())
	if err != nil {
		return result.Prayer, time.Time{}, true
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	return result.Prayer, at, true
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

// runTray attempts to start the system tray on the main goroutine (required by
// macOS/Windows). If the tray panics or fails for any reason (missing GTK on
// Linux, CGO stub, etc.) it recovers and falls back to headless mode so the
// agent keeps running.
func runTray(prg *program, svc service.Service) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tray] failed to start: %v — falling back to headless", r)
			runHeadless(prg)
		}
	}()

	tray.Run(tray.Config{
		Port: func() int {
			if prg.webSrv != nil {
				return prg.webSrv.Port()
			}
			return config.Get().Web.Port
		}(),
		Version: version,
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
		QuranStatus: func() (bool, bool) {
			if prg.quranCtrl == nil {
				return false, false
			}
			return prg.quranCtrl.Status()
		},
		StreamQuranSpeaker: func() error {
			if prg.quranCtrl == nil {
				return nil
			}
			return prg.quranCtrl.StartSpeaker(0)
		},
		StopQuranSpeaker: func() {
			if prg.quranCtrl != nil {
				prg.quranCtrl.StopSpeaker()
			}
		},
		StreamQuranLocal: func() error {
			if prg.quranCtrl == nil {
				return nil
			}
			return prg.quranCtrl.StartLocal(0)
		},
		StopQuranLocal: func() {
			if prg.quranCtrl != nil {
				prg.quranCtrl.StopLocal()
			}
		},
		CheckUpdate: func() (string, string, bool, error) {
			return checkForUpdate(version)
		},
		Uninstall: func() {
			doUninstall(svc, func() { prg.Stop(svc) })
		},
	})
}

// doUninstall shows a confirmation dialog, then stops and uninstalls the
// service, removes platform-specific helper files, and exits the process.
// stopFn is called before service.Stop() (nil is fine).
func doUninstall(svc service.Service, stopFn func()) {
	if !confirmDialog(
		"Uninstall Automated Azan?",
		"This will stop the service and remove it from automatic startup.\n\nYour config and prayer data will be kept.",
	) {
		return
	}

	log.Println("[main] uninstalling service...")
	if stopFn != nil {
		stopFn()
	}
	_ = svc.Stop()
	_ = svc.Uninstall()

	// Remove the helper binary that was copied outside the .app bundle (macOS).
	if helperBin, _, _ := ensureHelperBinary(); helperBin != "" {
		_ = os.Remove(helperBin)
	}

	os.Exit(0)
}

// runHeadless opens the dashboard in the default browser and blocks forever.
// Used when there is no graphical environment or the tray failed to start.
func runHeadless(prg *program) {
	go func() {
		time.Sleep(time.Second) // wait for web server to be ready
		if prg.webSrv != nil {
			openBrowser(fmt.Sprintf("http://localhost:%d", prg.webSrv.Port()))
		}
	}()
	select {} // block forever — the agent runs in goroutines
}

// shouldShowTray returns true when the system tray should be attempted.
// macOS and Windows always have a GUI environment so we always show it.
// On Linux we require DISPLAY or WAYLAND_DISPLAY (headless servers don't).
func shouldShowTray() bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	default: // linux
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	}
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
