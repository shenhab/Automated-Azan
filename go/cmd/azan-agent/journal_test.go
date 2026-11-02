package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
)

type stubDevices struct {
	speaker  chromecast.Device
	err      error
	all      []chromecast.Device
	group    chromecast.SpeakerGroup
	groupErr error
}

func (s stubDevices) SpeakerDevice() (chromecast.Device, error) { return s.speaker, s.err }
func (s stubDevices) Devices() []chromecast.Device              { return s.all }
func (s stubDevices) SpeakerGroup() (chromecast.SpeakerGroup, error) {
	return s.group, s.groupErr
}

func dev(name, host string, port int, uuid string) chromecast.Device {
	return chromecast.Device{Name: name, Host: host, Port: port, UUID: uuid}
}

func roles(cfg *config.Config, src deviceSource) string {
	var out []string
	for _, t := range journalTargets(cfg, src) {
		out = append(out, fmt.Sprintf("%s=%s", t.Name, t.Role))
	}
	return strings.Join(out, ",")
}

func TestWhenTheGroupCannotBeAskedTheJournalWatchesTheDeviceThatHostsIt(t *testing.T) {
	src := stubDevices{
		groupErr: fmt.Errorf("no answer"),
		speaker:  dev("athan", "10.0.0.5", 32067, "g"),
		all: []chromecast.Device{
			dev("athan", "10.0.0.5", 32067, "g"),
			dev("Nest display", "10.0.0.5", 8009, "n"),
			dev("Kitchen speaker", "10.0.0.6", 8009, "k"),
		},
	}
	cfg := &config.Config{}
	cfg.Journal.Enabled = true
	got := roles(cfg, src)
	if got != "athan=speaker,Nest display=member" {
		t.Errorf("targets = %s", got)
	}
}

func TestTVsAreWatchedOnlyWhenTheMuteFeatureIsOnAndTheyAreChosen(t *testing.T) {
	src := stubDevices{
		speaker: dev("athan", "10.0.0.5", 32067, "g"),
		all: []chromecast.Device{
			dev("athan", "10.0.0.5", 32067, "g"),
			dev("Living Room TV", "10.0.0.7", 8009, "tv1"),
			dev("Master Bedroom TV", "10.0.0.8", 8009, "tv2"),
			dev("Kitchen speaker", "10.0.0.6", 8009, "k"),
		},
	}
	cfg := &config.Config{}
	cfg.Journal.WatchTVs = true
	cfg.TVPause.Enabled = true
	cfg.TVPause.Devices = []string{"tv1", "tv2"}
	if got := roles(cfg, src); got != "athan=speaker,Living Room TV=tv,Master Bedroom TV=tv" {
		t.Errorf("targets = %s", got)
	}
	cfg.Journal.WatchTVs = false
	if got := roles(cfg, src); got != "athan=speaker" {
		t.Errorf("TVs watched although switched off: %s", got)
	}
	cfg.Journal.WatchTVs, cfg.TVPause.Enabled = true, false
	if got := roles(cfg, src); got != "athan=speaker" {
		t.Errorf("TVs watched although the mute feature is off: %s", got)
	}
}

func TestExtraDevicesAreWatchedByName(t *testing.T) {
	src := stubDevices{
		speaker: dev("athan", "10.0.0.5", 32067, "g"),
		all:     []chromecast.Device{dev("athan", "10.0.0.5", 32067, "g"), dev("Kitchen speaker", "10.0.0.6", 8009, "k")},
	}
	cfg := &config.Config{}
	cfg.Journal.WatchDevices = []string{"kitchen speaker", "No Such Device"}
	if got := roles(cfg, src); got != "athan=speaker,Kitchen speaker=extra" {
		t.Errorf("targets = %s", got)
	}
}

func TestWithoutAReachableSpeakerTheOtherDevicesAreStillWatched(t *testing.T) {
	src := stubDevices{err: fmt.Errorf("not found"), all: []chromecast.Device{dev("Living Room TV", "10.0.0.7", 8009, "tv1")}}
	cfg := &config.Config{}
	cfg.Journal.WatchTVs, cfg.TVPause.Enabled, cfg.TVPause.Devices = true, true, []string{"tv1"}
	if got := roles(cfg, src); got != "Living Room TV=tv" {
		t.Errorf("targets = %s", got)
	}
}

func TestEachKindOfJobGetsTheRightSessionSpec(t *testing.T) {
	cfg := &config.Config{}
	cfg.Prayer.PreFajrMinutes = 30
	cfg.Prayer.Channels.Fajr.Speaker = true
	cfg.Prayer.Channels.Asr.Speaker = false // only on the browser, say
	cfg.Prayer.Channels.PreFajr.Speaker = true
	at := time.Date(2026, 11, 2, 5, 39, 0, 0, time.UTC)

	fajr := journalSpec(cfg, "Fajr", "Fajr Athan", at)
	if fajr.Kind != "athan" || fajr.Prayer != "Fajr" || !fajr.ExpectPlayback || !fajr.ScheduledFor.Equal(at) {
		t.Errorf("fajr = %+v", fajr)
	}
	if asr := journalSpec(cfg, "Asr", "Asr Athan", at); asr.ExpectPlayback {
		t.Error("an Athan not sent to the speaker must not be judged a failure for being silent")
	}
	q := journalSpec(cfg, "pre_fajr", "Pre-Fajr Quran", at)
	if q.Kind != "quran" || q.MaxDuration < 30*time.Minute || q.MaxDuration > 40*time.Minute {
		t.Errorf("quran = %+v", q)
	}
	if k := journalSpec(cfg, "friday_kahf", "Friday Surah Al-Kahf", at); k.Kind != "kahf" || k.MaxDuration <= 0 {
		t.Errorf("kahf = %+v", k)
	}
}

func TestTheJournalWatchesTheGroupsRealMembersWhereverTheyAre(t *testing.T) {
	// Measured live 2026-11-02: athan = Nest display + Kitchen speaker. The old
	// rule (same address as the group) missed the Kitchen speaker and took in
	// groups that merely live on the Nest display.
	nest := dev("Nest display", "10.0.0.5", 8009, "n")
	kitchen := dev("Kitchen speaker", "10.0.0.6", 8009, "k")
	src := stubDevices{
		speaker: dev("athan", "10.0.0.5", 32067, "g"),
		all: []chromecast.Device{
			dev("athan", "10.0.0.5", 32067, "g"), nest, kitchen,
			dev("Upstairs", "10.0.0.5", 32003, "u"),
		},
		group: chromecast.SpeakerGroup{
			Members: []chromecast.GroupMember{{Name: "Nest display"}, {Name: "Kitchen speaker"}},
			Devices: []chromecast.Device{nest, kitchen},
		},
	}
	cfg := &config.Config{}
	cfg.Journal.Enabled = true
	if got := roles(cfg, src); got != "athan=speaker,Nest display=member,Kitchen speaker=member" {
		t.Errorf("targets = %s", got)
	}
}
