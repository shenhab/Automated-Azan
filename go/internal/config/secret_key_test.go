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
