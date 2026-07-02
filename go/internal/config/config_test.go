package config

import (
	"os"
	"path/filepath"
	"testing"
)

const validTOML = `
[speaker]
group_name = "athan"
athan_speaker = "Living Room speaker"
pre_fajr_speaker = "Bedroom speaker"
friday_kahf_speaker = "Living Room speaker"
quran_speaker = "Kitchen speaker"

[prayer]
location = "icci"
pre_fajr_enabled = true
pre_fajr_minutes = 20
friday_kahf_enabled = true

[prayer.enabled]
fajr = true
dhuhr = true
asr = false
maghrib = true
isha = true

[web]
host = "127.0.0.1"
port = 9090
secret_key = "test-secret"

[web.auth]
username = "admin"
password_hash = "hash"

[log]
level = "DEBUG"
file_path = "/tmp/azan.log"

[tv_pause]
enabled = true
resume_delay_seconds = 120
devices = ["uuid-1", "uuid-2"]
`

func writeTemp(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "azan.toml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadFrom_ValidConfig(t *testing.T) {
	path := writeTemp(t, validTOML)

	c, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned unexpected error: %v", err)
	}

	if c.Prayer.Location != "icci" {
		t.Errorf("Prayer.Location = %q, want %q", c.Prayer.Location, "icci")
	}
	if c.Prayer.PreFajrMinutes != 20 {
		t.Errorf("Prayer.PreFajrMinutes = %d, want 20", c.Prayer.PreFajrMinutes)
	}
	if !c.Prayer.PreFajrEnabled {
		t.Errorf("Prayer.PreFajrEnabled = false, want true")
	}
	if !c.Prayer.FridayKahfEnabled {
		t.Errorf("Prayer.FridayKahfEnabled = false, want true")
	}
	if c.Prayer.Enabled.Asr {
		t.Errorf("Prayer.Enabled.Asr = true, want false")
	}
	if !c.Prayer.Enabled.Fajr {
		t.Errorf("Prayer.Enabled.Fajr = false, want true")
	}

	if c.Speaker.GroupName != "athan" {
		t.Errorf("Speaker.GroupName = %q, want %q", c.Speaker.GroupName, "athan")
	}
	if c.Speaker.AthanSpeaker != "Living Room speaker" {
		t.Errorf("Speaker.AthanSpeaker = %q, want %q", c.Speaker.AthanSpeaker, "Living Room speaker")
	}
	if c.Speaker.QuranSpeaker != "Kitchen speaker" {
		t.Errorf("Speaker.QuranSpeaker = %q, want %q", c.Speaker.QuranSpeaker, "Kitchen speaker")
	}

	if c.Web.Host != "127.0.0.1" {
		t.Errorf("Web.Host = %q, want %q", c.Web.Host, "127.0.0.1")
	}
	if c.Web.Port != 9090 {
		t.Errorf("Web.Port = %d, want 9090", c.Web.Port)
	}
	if c.Web.Auth.Username != "admin" {
		t.Errorf("Web.Auth.Username = %q, want %q", c.Web.Auth.Username, "admin")
	}

	if c.Log.Level != "DEBUG" {
		t.Errorf("Log.Level = %q, want %q", c.Log.Level, "DEBUG")
	}

	if !c.TVPause.Enabled {
		t.Errorf("TVPause.Enabled = false, want true")
	}
	if c.TVPause.ResumeDelaySecs != 120 {
		t.Errorf("TVPause.ResumeDelaySecs = %d, want 120", c.TVPause.ResumeDelaySecs)
	}
	if len(c.TVPause.Devices) != 2 || c.TVPause.Devices[0] != "uuid-1" {
		t.Errorf("TVPause.Devices = %v, want [uuid-1 uuid-2]", c.TVPause.Devices)
	}

	if c.filePath != path {
		t.Errorf("filePath = %q, want %q", c.filePath, path)
	}
}

func TestLoadFrom_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.toml")

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatalf("LoadFrom(%q) returned nil error, want error for missing file", path)
	}
}

func TestLoadFrom_MalformedTOML(t *testing.T) {
	path := writeTemp(t, `this is not [ valid toml`)

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatalf("LoadFrom returned nil error, want error for malformed TOML")
	}
}

func TestLoadFrom_UnknownLocationRejected(t *testing.T) {
	path := writeTemp(t, `
[prayer]
location = "atlantis"
`)

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatalf("LoadFrom returned nil error, want error for unrecognized location")
	}
}

func TestLoadFrom_AladhanRequiresCityAndCountry(t *testing.T) {
	path := writeTemp(t, `
[prayer]
location = "aladhan"
`)

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatalf("LoadFrom returned nil error, want error for aladhan location missing city/country")
	}
}

func TestLoadFrom_AladhanWithCityAndCountrySucceeds(t *testing.T) {
	path := writeTemp(t, `
[prayer]
location = "aladhan"
aladhan_city = "Dublin"
aladhan_country = "Ireland"
`)

	c, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned unexpected error: %v", err)
	}
	if c.Prayer.AladhanCity != "Dublin" {
		t.Errorf("Prayer.AladhanCity = %q, want %q", c.Prayer.AladhanCity, "Dublin")
	}
}

func TestLoadFrom_InvalidPortRejected(t *testing.T) {
	path := writeTemp(t, `
[prayer]
location = "naas"

[web]
port = 70000
`)

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatalf("LoadFrom returned nil error, want error for out-of-range port")
	}
}

func TestLoadFrom_DefaultsAppliedWhenSectionsOmitted(t *testing.T) {
	path := writeTemp(t, `
[prayer]
location = "naas"
`)

	c, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned unexpected error: %v", err)
	}
	if c.Web.Port != 28426 {
		t.Errorf("Web.Port = %d, want default 28426", c.Web.Port)
	}
	if c.Speaker.GroupName != "athan" {
		t.Errorf("Speaker.GroupName = %q, want default %q", c.Speaker.GroupName, "athan")
	}
}
