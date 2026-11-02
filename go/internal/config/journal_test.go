package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func decode(t *testing.T, doc string) *Config {
	t.Helper()
	c := &Config{}
	c.setDefaults()
	if _, err := toml.Decode(doc, c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheJournalIsOnByDefaultForExistingConfigFiles(t *testing.T) {
	// A config file written before the Journal existed has no [journal] section.
	c := decode(t, "[speaker]\ngroup_name = \"athan\"\n")
	j := c.Journal
	if !j.Enabled || j.RetentionDays != 30 || j.MaxMB != 300 || !j.WatchTVs || j.NotifyOn != "problems" {
		t.Errorf("defaults = %+v", j)
	}
}

func TestTheJournalSectionIsRead(t *testing.T) {
	c := decode(t, `[journal]
enabled = false
retention_days = 7
max_mb = 50
watch_tvs = false
watch_devices = ["Kitchen speaker"]
notify_on = "failures"
`)
	j := c.Journal
	if j.Enabled || j.RetentionDays != 7 || j.MaxMB != 50 || j.WatchTVs || j.NotifyOn != "failures" ||
		len(j.WatchDevices) != 1 || j.WatchDevices[0] != "Kitchen speaker" {
		t.Errorf("parsed = %+v", j)
	}
}

func TestTheJournalSettingsAreInTheWebDictionary(t *testing.T) {
	d := decode(t, "").AsWebDict()
	for _, k := range []string{"journal_enabled", "journal_retention_days", "journal_max_mb", "journal_watch_tvs", "journal_watch_devices", "journal_notify_on"} {
		if _, ok := d[k]; !ok {
			t.Errorf("web dictionary lacks %s", k)
		}
	}
}
