// Package localplay plays audio files and streams on the local machine using
// OS-native tools. No additional libraries are required:
//   - macOS:   afplay (built-in)
//   - Linux:   mpg123 → ffplay → cvlc  (first found wins)
//   - Windows: PowerShell WMF MediaPlayer (built-in on Windows 7+)
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

// StartStream begins streaming audio from url and returns a stop function.
// The caller is responsible for calling stop() when the stream should end.
func StartStream(url string) (stop func(), err error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// afplay doesn't support HTTP; prefer mpg123/ffplay
		cmd, err = firstAvailableCmd(url,
			[]string{"mpg123", "-q", url},
			[]string{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", url},
		)
	case "linux":
		cmd, err = firstAvailableCmd(url,
			[]string{"mpg123", "-q", url},
			[]string{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", url},
			[]string{"cvlc", "--intf", "dummy", url},
		)
	case "windows":
		// WMF MediaPlayer also handles HTTP streams
		ps := buildWindowsScript(url)
		cmd = exec.Command("powershell",
			"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
			"-Command", ps)
		err = cmd.Start()
	default:
		return func() {}, nil
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

// runFirst tries each command in order and runs the first one whose binary exists.
func runFirst(target string, cmds ...[]string) error {
	for _, args := range cmds {
		if _, err := exec.LookPath(args[0]); err == nil {
			return exec.Command(args[0], args[1:]...).Run()
		}
	}
	return fmt.Errorf("no audio player found for %q; install mpg123 or ffmpeg", target)
}

// firstAvailableCmd builds and starts the first command whose binary exists.
func firstAvailableCmd(target string, cmds ...[]string) (*exec.Cmd, error) {
	for _, args := range cmds {
		if _, err := exec.LookPath(args[0]); err == nil {
			cmd := exec.Command(args[0], args[1:]...)
			return cmd, cmd.Start()
		}
	}
	return nil, fmt.Errorf("no streaming player found for %q; install mpg123 or ffmpeg", target)
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
