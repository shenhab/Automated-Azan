package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestAnExistingConfigWithTheOldSecretKeyStillLoads(t *testing.T) {
	// secret_key was never used, and installs in the field still have it on disk.
	c := &Config{}
	c.setDefaults()
	_, err := toml.Decode("[web]\nport = 1234\nsecret_key = \"automated-azan-secret-key\"\n", c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Web.Port != 1234 {
		t.Errorf("port = %d: the rest of the section was not read", c.Web.Port)
	}
}

func TestANewConfigFileHasNoSecretKey(t *testing.T) {
	c := &Config{}
	c.setDefaults()
	var out bytes.Buffer
	if err := toml.NewEncoder(&out).Encode(c); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "secret_key") {
		t.Errorf("a new config file would still contain secret_key:\n%s", out.String())
	}
}

func TestANewInstallDefaultsToDublinAndTheAthanGroup(t *testing.T) {
	c := &Config{}
	c.setDefaults()
	if c.Prayer.Location != "mai" || !IsValidLocation(c.Prayer.Location) {
		t.Errorf("location = %q, want the live Dublin source", c.Prayer.Location)
	}
	if c.Speaker.GroupName != "athan" {
		t.Errorf("speaker group = %q, want athan", c.Speaker.GroupName)
	}
}

func TestAnExistingConfigKeepsItsOwnLocation(t *testing.T) {
	// The default is for new installs only.
	if got := decode(t, "[prayer]\nlocation = \"naas\"\n").Prayer.Location; got != "naas" {
		t.Errorf("location = %q", got)
	}
}
