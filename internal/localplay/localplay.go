// Package localplay plays audio files and streams on the local machine.
//
// File playback uses OS-native tools (afplay on macOS, mpg123/ffplay on Linux,
// PowerShell WMF on Windows).
//
// HTTP stream playback uses the bundled pure-Go oto/go-mp3 stack with automatic
// fallback to external tools (mpg123, ffplay, cvlc) when no audio device is
// present (e.g. headless Docker container).
//
// macOS/Linux stream fallback — install one if needed:
//
//	macOS: brew install mpg123  |  brew install ffmpeg
//	Linux: apt  install mpg123  |  apt  install ffmpeg
package localplay

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Play plays a local audio file, blocking until playback finishes.
func Play(filePath string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("afplay", filePath).Run()
	case "linux":
		return runFirst(filePath,
			[]string{"mpg123", "-q", filePath},
			[]string{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", filePath},
			[]string{"cvlc", "--intf", "dummy", "--play-and-exit", filePath},
		)
	case "windows":
		return windowsPlay(fmt.Sprintf("file:///%s",
			strings.ReplaceAll(filepath.ToSlash(filePath), " ", "%20")))
	}
	return nil
}

// runFirst tries each command in order and runs the first one whose binary exists.
func runFirst(target string, cmds ...[]string) error {
	for _, args := range cmds {
		if _, err := exec.LookPath(args[0]); err == nil {
			return exec.Command(args[0], args[1:]...).Run()
		}
	}
	return fmt.Errorf("no audio player found for %q — %s", target, installHint())
}

// startStreamExternal is the fallback when oto has no audio device available.
// It tries external CLI players in order: mpg123 → ffplay → cvlc.
func startStreamExternal(url string) (func(), error) {
	var cmd *exec.Cmd
	var err error
	switch runtime.GOOS {
	case "windows":
		ps := buildWindowsScript(url)
		cmd = exec.Command("powershell",
			"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
			"-Command", ps)
		err = cmd.Start()
	default: // darwin + linux
		for _, args := range [][]string{
			{"mpg123", "-q", url},
			{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", url},
			{"cvlc", "--intf", "dummy", url},
		} {
			if _, lookErr := exec.LookPath(args[0]); lookErr == nil {
				cmd = exec.Command(args[0], args[1:]...)
				err = cmd.Start()
				break
			}
		}
		if cmd == nil {
			return func() {}, fmt.Errorf("no streaming player found — %s", installHint())
		}
	}
	if err != nil {
		return func() {}, err
	}
	return func() {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}, nil
}

func installHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install mpg123  or  brew install ffmpeg"
	case "linux":
		return "apt install mpg123  or  apt install ffmpeg"
	default:
		return "install mpg123 or ffmpeg"
	}
}

// windowsPlay runs audio via PowerShell's WMF MediaPlayer (Windows 7+).
// Works for both file:/// URIs and HTTP URLs.
func windowsPlay(uri string) error {
	return exec.Command("powershell",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", buildWindowsScript(uri),
	).Run()
}

func buildWindowsScript(uri string) string {
	safe := strings.ReplaceAll(uri, "'", "''") // escape PS single-quotes
	return fmt.Sprintf(
		`$ErrorActionPreference='SilentlyContinue';`+
			`Add-Type -AssemblyName presentationCore;`+
			`$mp=[System.Windows.Media.MediaPlayer]::new();`+
			`$mp.Open([Uri]'%s');$mp.Play();`+
			`$t=[DateTime]::Now.AddMinutes(10);`+
			`while(-not $mp.NaturalDuration.HasTimeSpan -and [DateTime]::Now -lt $t){Start-Sleep -ms 200};`+
			`if($mp.NaturalDuration.HasTimeSpan){`+
			`  $e=[DateTime]::Now.Add($mp.NaturalDuration.TimeSpan);`+
			`  while([DateTime]::Now -lt $e){Start-Sleep -ms 200}`+
			`};$mp.Close()`,
		safe,
	)
}
