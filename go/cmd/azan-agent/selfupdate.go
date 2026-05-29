package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const githubRepo = "shenhab/Automated-Azan"

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// checkForUpdate queries the GitHub releases API and returns the latest tag and
// release page URL when a newer version is available.
// Returns ("", "", false, nil) when already up to date or on a dev build.
func checkForUpdate(currentVersion string) (newVersion, releaseURL string, hasUpdate bool, err error) {
	if currentVersion == "dev" || currentVersion == "" {
		return "", "", false, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	resp, err := client.Get(url)
	if err != nil {
		return "", "", false, fmt.Errorf("update check: %w", err)
	}
	defer resp.Body.Close()

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", false, fmt.Errorf("update check: %w", err)
	}

	latest := rel.TagName
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}
	current := currentVersion
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}

	// semver.Compare requires canonical "vX.Y.Z" form.
	if semver.Compare(latest, current) > 0 {
		return rel.TagName, rel.HTMLURL, true, nil
	}
	return "", "", false, nil
}
