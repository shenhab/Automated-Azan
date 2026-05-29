package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const githubRepo = "shenhab/Automated-Azan"

type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// UpdateInfo holds the result of a successful update check.
type UpdateInfo struct {
	NewVersion  string
	DownloadURL string // raw binary URL, or release page URL if no binary found
	HasBinary   bool   // true if DownloadURL points to a downloadable binary
}

// checkForUpdate queries the GitHub releases API.
// Returns nil if already up to date or running a dev build.
func checkForUpdate(currentVersion string) (*UpdateInfo, error) {
	if currentVersion == "dev" || currentVersion == "" {
		return nil, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo))
	if err != nil {
		return nil, fmt.Errorf("update check: %w", err)
	}
	defer resp.Body.Close()

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("update check: %w", err)
	}

	latest := rel.TagName
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}
	current := currentVersion
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}

	if semver.Compare(latest, current) <= 0 {
		return nil, nil // already on latest
	}

	// Look for a downloadable binary asset for this platform/arch.
	want := selfUpdateAssetName()
	for _, a := range rel.Assets {
		if a.Name == want {
			return &UpdateInfo{
				NewVersion:  rel.TagName,
				DownloadURL: a.BrowserDownloadURL,
				HasBinary:   true,
			}, nil
		}
	}

	// No binary asset found — fall back to the release page.
	return &UpdateInfo{
		NewVersion:  rel.TagName,
		DownloadURL: rel.HTMLURL,
		HasBinary:   false,
	}, nil
}

// selfUpdateAssetName returns the GitHub release asset name for this platform.
// Must match the asset names produced by the CI release workflow.
func selfUpdateAssetName() string {
	name := fmt.Sprintf("azan-agent-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// applySelfUpdate downloads the binary at downloadURL, replaces the helper
// binary and (on macOS) the binary inside the .app bundle, removes any
// quarantine attribute, and restarts the background service.
//
// Programmatic downloads via net/http are never assigned com.apple.quarantine
// by macOS — only browser downloads are — so the replacement binary will open
// without any Gatekeeper prompt.
func applySelfUpdate(downloadURL string, restartService func()) error {
	tmp, err := downloadToTemp(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmp)

	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}

	// Replace the helper binary (the one the LaunchAgent/service runs).
	helperBin, _, _ := ensureHelperBinary()
	if helperBin != "" {
		if err := replaceBinary(tmp, helperBin); err != nil {
			return fmt.Errorf("replace helper: %w", err)
		}
		// Set a future modtime so ensureHelperBinary() won't overwrite the
		// updated helper with the older .app binary on the next launch.
		future := time.Now().Add(24 * time.Hour)
		_ = os.Chtimes(helperBin, future, future)
	}

	// Also replace the binary inside the .app bundle (keeps it consistent
	// and ensures the next Finder launch starts the correct version).
	if exe, err := os.Executable(); err == nil && strings.Contains(exe, ".app/Contents/MacOS/") {
		_ = replaceBinary(tmp, exe)
	}

	// Strip quarantine from the .app bundle in /Applications (belt-and-
	// suspenders; programmatic downloads don't get quarantined anyway).
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/Applications/AzanAgent.app"); err == nil {
			exec.Command("xattr", "-rd", "com.apple.quarantine",
				"/Applications/AzanAgent.app").Run() //nolint:errcheck
		}
	}

	restartService()
	return nil
}

// relaunch starts a fresh copy of the (now updated) application and exits the
// current process so the tray is replaced by the new version's tray.
func relaunch() {
	if runtime.GOOS == "darwin" {
		// open -n launches a new instance even if one is already registered.
		exec.Command("open", "-n", "-a", "/Applications/AzanAgent.app").Start() //nolint:errcheck
	} else {
		exe, _ := os.Executable()
		exec.Command(exe, os.Args[1:]...).Start() //nolint:errcheck
	}
	os.Exit(0)
}

func downloadToTemp(url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "azan-update-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
}

func replaceBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".update"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	return os.Rename(tmp, dst)
}
