package config

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"azan-agent/internal/appdirs"

	"github.com/BurntSushi/toml"
)

// SpeakerConfig holds speaker/device settings.
type SpeakerConfig struct {
	GroupName        string `toml:"group_name"`
	AthanSpeaker     string `toml:"athan_speaker"`
	PreFajrSpeaker   string `toml:"pre_fajr_speaker"`
	FridayKahfSpeaker string `toml:"friday_kahf_speaker"`
	QuranSpeaker     string `toml:"quran_speaker"`
}

// Resolve returns the effective speaker name for the given audio type,
// falling back to GroupName when no override is set.
func (s SpeakerConfig) Resolve(audioType string) string {
	overrides := map[string]string{
		"athan":        s.AthanSpeaker,
		"pre_fajr":     s.PreFajrSpeaker,
		"friday_kahf":  s.FridayKahfSpeaker,
		"quran":        s.QuranSpeaker,
	}
	if v, ok := overrides[audioType]; ok && v != "" {
		return v
	}
	return s.GroupName
}

// PrayerEnabledConfig controls which prayers trigger Athan playback.
// All default to true; set false to mute a specific prayer.
type PrayerEnabledConfig struct {
	Fajr    bool `toml:"fajr"`
	Dhuhr   bool `toml:"dhuhr"`
	Asr     bool `toml:"asr"`
	Maghrib bool `toml:"maghrib"`
	Isha    bool `toml:"isha"`
}

// IsEnabled returns whether the named prayer should play Athan.
func (e PrayerEnabledConfig) IsEnabled(prayer string) bool {
	switch prayer {
	case "Fajr":    return e.Fajr
	case "Dhuhr":   return e.Dhuhr
	case "Asr":     return e.Asr
	case "Maghrib": return e.Maghrib
	case "Isha":    return e.Isha
	default:        return true
	}
}

// PrayerMediaConfig holds per-prayer audio filenames.
// Empty string means use the built-in default for that prayer.
type PrayerMediaConfig struct {
	Fajr    string `toml:"fajr"`
	Dhuhr   string `toml:"dhuhr"`
	Asr     string `toml:"asr"`
	Maghrib string `toml:"maghrib"`
	Isha    string `toml:"isha"`
}

// FileFor returns the configured media filename for the prayer,
// falling back to the standard defaults when not set.
func (m PrayerMediaConfig) FileFor(prayer string) string {
	var f string
	switch prayer {
	case "Fajr":    f = m.Fajr
	case "Dhuhr":   f = m.Dhuhr
	case "Asr":     f = m.Asr
	case "Maghrib": f = m.Maghrib
	case "Isha":    f = m.Isha
	}
	if f != "" {
		return f
	}
	if prayer == "Fajr" {
		return "media_adhan_al_fajr.mp3"
	}
	return "media_Athan.mp3"
}

// ChannelConfig holds the notification channels for a single prayer job.
// Any combination is valid; all false means the job fires silently.
type ChannelConfig struct {
	Speaker        bool `toml:"speaker"`         // play on Google/Chromecast speaker
	Local          bool `toml:"local"`           // play audio on the server machine
	Notify         bool `toml:"notify"`          // show an OS desktop notification (server-side)
	BrowserNotify  bool `toml:"browser_notify"`  // show a Web Notification in connected browser tabs
}

// JobChannelsConfig holds per-job channel settings for every schedulable job.
type JobChannelsConfig struct {
	Fajr       ChannelConfig `toml:"fajr"`
	Dhuhr      ChannelConfig `toml:"dhuhr"`
	Asr        ChannelConfig `toml:"asr"`
	Maghrib    ChannelConfig `toml:"maghrib"`
	Isha       ChannelConfig `toml:"isha"`
	PreFajr    ChannelConfig `toml:"pre_fajr"`
	FridayKahf ChannelConfig `toml:"friday_kahf"`
}

// ForPrayer returns the ChannelConfig for the named Athan prayer.
func (j JobChannelsConfig) ForPrayer(name string) ChannelConfig {
	switch name {
	case "Fajr":    return j.Fajr
	case "Dhuhr":   return j.Dhuhr
	case "Asr":     return j.Asr
	case "Maghrib": return j.Maghrib
	case "Isha":    return j.Isha
	default:        return ChannelConfig{Speaker: true}
	}
}

// PrayerConfig holds prayer scheduling settings.
type PrayerConfig struct {
	// Location is either a preloaded key ("naas","icci","newbridge")
	// or "aladhan" to use the Aladhan API.
	Location          string             `toml:"location"`
	AladhanCity       string             `toml:"aladhan_city"`
	AladhanCountry    string             `toml:"aladhan_country"`
	AladhanMethod     int                `toml:"aladhan_method"` // 3 = MWL (default)
	PreFajrEnabled    bool               `toml:"pre_fajr_enabled"`
	PreFajrMinutes    int                `toml:"pre_fajr_minutes"`
	FridayKahfEnabled bool               `toml:"friday_kahf_enabled"`
	Enabled           PrayerEnabledConfig `toml:"enabled"`
	Media             PrayerMediaConfig   `toml:"media"`
	Channels          JobChannelsConfig   `toml:"channels"`
}

// WebConfig holds web server settings.
type WebConfig struct {
	Host      string `toml:"host"`
	Port      int    `toml:"port"`
	SecretKey string `toml:"secret_key"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level    string `toml:"level"`
	FilePath string `toml:"file_path"`
}

// Config is the top-level configuration structure.
type Config struct {
	Speaker SpeakerConfig `toml:"speaker"`
	Prayer  PrayerConfig  `toml:"prayer"`
	Web     WebConfig     `toml:"web"`
	Log     LogConfig     `toml:"log"`

	mu       sync.RWMutex
	filePath string
}

var (
	instance *Config
	once     sync.Once
)

// Get returns the global singleton Config, loading from disk on first call.
func Get() *Config {
	once.Do(func() {
		instance = &Config{}
		instance.setDefaults()
		if err := instance.load(); err != nil {
			log.Printf("[config] failed to load, using defaults: %v", err)
		}
	})
	return instance
}

func (c *Config) setDefaults() {
	c.Speaker = SpeakerConfig{GroupName: "athan"}
	defaultCh := ChannelConfig{Speaker: true, Local: false, Notify: false}
	c.Prayer = PrayerConfig{
		Location:      "naas",
		AladhanMethod: 3, // Muslim World League
		PreFajrMinutes: 30,
		Enabled: PrayerEnabledConfig{
			Fajr: true, Dhuhr: true, Asr: true, Maghrib: true, Isha: true,
		},
		Channels: JobChannelsConfig{
			Fajr: defaultCh, Dhuhr: defaultCh, Asr: defaultCh,
			Maghrib: defaultCh, Isha: defaultCh,
			PreFajr: defaultCh, FridayKahf: defaultCh,
		},
	}
	c.Web = WebConfig{Host: "0.0.0.0", Port: 28426, SecretKey: "automated-azan-secret-key"}
	c.Log = LogConfig{
		Level:    "INFO",
		FilePath: filepath.Join(appdirs.Logs(), "azan.log"),
	}
}

func (c *Config) load() error {
	path := c.resolvePath()
	if path == "" {
		// First run — write default config to OS standard location
		path = c.writablePath()
		if err := c.writeDefaults(path); err != nil {
			log.Printf("[config] could not write default config: %v", err)
		} else {
			log.Printf("[config] created default config at %s", path)
		}
		c.filePath = path
		return nil
	}
	c.filePath = path

	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := toml.DecodeFile(path, c)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	log.Printf("[config] loaded from %s", path)
	return nil
}

func (c *Config) writeDefaults(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// Reload re-reads the config file from disk.
func (c *Config) Reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.filePath == "" {
		return fmt.Errorf("no config file path set")
	}
	_, err := toml.DecodeFile(c.filePath, c)
	if err != nil {
		return fmt.Errorf("reload %s: %w", c.filePath, err)
	}
	log.Printf("[config] reloaded from %s", c.filePath)
	return nil
}

// Save writes current config to disk.
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	path := c.filePath
	if path == "" {
		path = c.writablePath()
		c.filePath = path
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(c)
}

// FilePath returns the active config file path.
func (c *Config) FilePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.filePath
}

// Hash returns an MD5 of the config file contents (used for change detection).
func (c *Config) Hash() string {
	path := c.FilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", md5.Sum(data))
}

// AsWebDict returns a flat map suitable for JSON API responses.
func (c *Config) AsWebDict() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return map[string]interface{}{
		"speakers_group_name":   c.Speaker.GroupName,
		"athan_speaker":         c.Speaker.AthanSpeaker,
		"pre_fajr_speaker":      c.Speaker.PreFajrSpeaker,
		"friday_kahf_speaker":   c.Speaker.FridayKahfSpeaker,
		"quran_speaker":         c.Speaker.QuranSpeaker,
		"location":              c.Prayer.Location,
		"location_label":        locationLabel(c.Prayer.Location, c.Prayer.AladhanCity, c.Prayer.AladhanCountry),
		"aladhan_city":          c.Prayer.AladhanCity,
		"aladhan_country":       c.Prayer.AladhanCountry,
		"aladhan_method":        c.Prayer.AladhanMethod,
		"aladhan_ireland_warn":  c.Prayer.Location == "aladhan" && isIreland(c.Prayer.AladhanCountry),
		"pre_fajr_enabled":      c.Prayer.PreFajrEnabled,
		"pre_fajr_minutes":      c.Prayer.PreFajrMinutes,
		"friday_kahf_enabled":   c.Prayer.FridayKahfEnabled,
		"web_port":              c.Web.Port,
		"log_level":             c.Log.Level,
		// Per-prayer enabled flags
		"fajr_enabled":    c.Prayer.Enabled.Fajr,
		"dhuhr_enabled":   c.Prayer.Enabled.Dhuhr,
		"asr_enabled":     c.Prayer.Enabled.Asr,
		"maghrib_enabled": c.Prayer.Enabled.Maghrib,
		"isha_enabled":    c.Prayer.Enabled.Isha,
		// Per-prayer media files
		"fajr_media":    c.Prayer.Media.Fajr,
		"dhuhr_media":   c.Prayer.Media.Dhuhr,
		"asr_media":     c.Prayer.Media.Asr,
		"maghrib_media": c.Prayer.Media.Maghrib,
		"isha_media":    c.Prayer.Media.Isha,
		// Per-job notification channels  (ch_ prefix avoids collision with speaker-device overrides)
		"ch_fajr_speaker": c.Prayer.Channels.Fajr.Speaker, "ch_fajr_local": c.Prayer.Channels.Fajr.Local, "ch_fajr_notify": c.Prayer.Channels.Fajr.Notify, "ch_fajr_browser": c.Prayer.Channels.Fajr.BrowserNotify,
		"ch_dhuhr_speaker": c.Prayer.Channels.Dhuhr.Speaker, "ch_dhuhr_local": c.Prayer.Channels.Dhuhr.Local, "ch_dhuhr_notify": c.Prayer.Channels.Dhuhr.Notify, "ch_dhuhr_browser": c.Prayer.Channels.Dhuhr.BrowserNotify,
		"ch_asr_speaker": c.Prayer.Channels.Asr.Speaker, "ch_asr_local": c.Prayer.Channels.Asr.Local, "ch_asr_notify": c.Prayer.Channels.Asr.Notify, "ch_asr_browser": c.Prayer.Channels.Asr.BrowserNotify,
		"ch_maghrib_speaker": c.Prayer.Channels.Maghrib.Speaker, "ch_maghrib_local": c.Prayer.Channels.Maghrib.Local, "ch_maghrib_notify": c.Prayer.Channels.Maghrib.Notify, "ch_maghrib_browser": c.Prayer.Channels.Maghrib.BrowserNotify,
		"ch_isha_speaker": c.Prayer.Channels.Isha.Speaker, "ch_isha_local": c.Prayer.Channels.Isha.Local, "ch_isha_notify": c.Prayer.Channels.Isha.Notify, "ch_isha_browser": c.Prayer.Channels.Isha.BrowserNotify,
		"ch_pre_fajr_speaker": c.Prayer.Channels.PreFajr.Speaker, "ch_pre_fajr_local": c.Prayer.Channels.PreFajr.Local, "ch_pre_fajr_notify": c.Prayer.Channels.PreFajr.Notify, "ch_pre_fajr_browser": c.Prayer.Channels.PreFajr.BrowserNotify,
		"ch_friday_kahf_speaker": c.Prayer.Channels.FridayKahf.Speaker, "ch_friday_kahf_local": c.Prayer.Channels.FridayKahf.Local, "ch_friday_kahf_notify": c.Prayer.Channels.FridayKahf.Notify, "ch_friday_kahf_browser": c.Prayer.Channels.FridayKahf.BrowserNotify,
	}
}

func (c *Config) resolvePath() string {
	// 1. Explicit env override (Docker / CI)
	if env := os.Getenv("AZAN_CONFIG_FILE"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	// 2. Docker path
	if _, err := os.Stat("/app/config/azan.toml"); err == nil {
		return "/app/config/azan.toml"
	}
	// 3. OS-standard user config dir
	osCfg := filepath.Join(appdirs.Config(), "azan.toml")
	if _, err := os.Stat(osCfg); err == nil {
		return osCfg
	}
	// 4. Local directory (development / legacy)
	if _, err := os.Stat("azan.toml"); err == nil {
		return "azan.toml"
	}
	return ""
}

func (c *Config) writablePath() string {
	if env := os.Getenv("AZAN_CONFIG_FILE"); env != "" {
		return env
	}
	return filepath.Join(appdirs.Config(), "azan.toml")
}

// locationLabel returns a human-readable label for the active location.
func locationLabel(key, aladhanCity, aladhanCountry string) string {
	if key == "aladhan" {
		if aladhanCity != "" && aladhanCountry != "" {
			return aladhanCountry + " — " + aladhanCity
		}
		return "Aladhan API"
	}
	labels := map[string]string{
		"naas":      "Ireland — Naas",
		"icci":      "Ireland — Dublin",
		"newbridge": "Ireland — Newbridge",
		"cork":      "Ireland — Cork",
		"galway":    "Ireland — Galway",
	}
	if l, ok := labels[key]; ok {
		return l
	}
	return key
}

// isIreland reports whether the country string refers to Ireland.
func isIreland(country string) bool {
	switch strings.ToLower(strings.TrimSpace(country)) {
	case "ireland", "ie", "irl", "republic of ireland":
		return true
	}
	return false
}
