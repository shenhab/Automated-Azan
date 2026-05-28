package config

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

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

// PrayerConfig holds prayer scheduling settings.
type PrayerConfig struct {
	Location           string `toml:"location"`
	PreFajrEnabled     bool   `toml:"pre_fajr_enabled"`
	PreFajrMinutes     int    `toml:"pre_fajr_minutes"`
	FridayKahfEnabled  bool   `toml:"friday_kahf_enabled"`
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
	c.Prayer = PrayerConfig{
		Location:       "naas",
		PreFajrMinutes: 30,
	}
	c.Web = WebConfig{Host: "0.0.0.0", Port: 28426, SecretKey: "automated-azan-secret-key"}
	c.Log = LogConfig{Level: "INFO", FilePath: "logs/azan.log"}
}

func (c *Config) load() error {
	path := c.resolvePath()
	if path == "" {
		return fmt.Errorf("no config file found")
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
		"pre_fajr_enabled":      c.Prayer.PreFajrEnabled,
		"pre_fajr_minutes":      c.Prayer.PreFajrMinutes,
		"friday_kahf_enabled":   c.Prayer.FridayKahfEnabled,
		"web_port":              c.Web.Port,
		"log_level":             c.Log.Level,
	}
}

func (c *Config) resolvePath() string {
	if env := os.Getenv("AZAN_CONFIG_FILE"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	candidates := []string{"/app/config/azan.toml", "azan.toml"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (c *Config) writablePath() string {
	if env := os.Getenv("AZAN_CONFIG_FILE"); env != "" {
		return env
	}
	return "azan.toml"
}
