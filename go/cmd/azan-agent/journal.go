package main

import (
	"log"
	"strings"
	"time"

	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
	"azan-agent/internal/journal"
)

// deviceSource is what the Journal needs to know about the network; the Cast
// manager provides it.
type deviceSource interface {
	SpeakerDevice() (chromecast.Device, error)
	Devices() []chromecast.Device
	SpeakerGroup() (chromecast.SpeakerGroup, error)
}

const maxWatchedTVs = 8

// journalTargets chooses the devices to record around a prayer: the speaker the
// agent plays on, whatever hosts it (a Cast group lives on one of its members —
// the display's own reports show things the group's do not), the TVs the mute
// feature handles, and any devices named in the settings.
func journalTargets(cfg *config.Config, src deviceSource) []journal.Target {
	var out []journal.Target
	seen := map[string]bool{}
	add := func(d chromecast.Device, role string) {
		key := d.Host + ":" + itoa(d.Port)
		if seen[key] || d.Host == "" {
			return
		}
		seen[key] = true
		out = append(out, journal.Target{Name: d.Name, Host: d.Host, Port: d.Port, Role: role})
	}
	devices := src.Devices()

	speaker, err := src.SpeakerDevice()
	if err == nil {
		add(speaker, "speaker")
		if group, gerr := src.SpeakerGroup(); gerr == nil && len(group.Members) > 0 {
			// The group's own list of members, wherever they are on the network.
			for _, d := range group.Devices {
				add(d, "member")
			}
			for _, mb := range group.Missing {
				log.Printf("[journal] %q is in the group %q but was not found on the network, so it cannot be watched", mb.Name, speaker.Name)
			}
		} else {
			// The group could not be asked: fall back to what shares its address
			// (a group is hosted by one of its members).
			for _, d := range devices {
				if d.Host == speaker.Host && d.Port != speaker.Port {
					add(d, "member")
				}
			}
		}
	}

	if cfg.Journal.WatchTVs && cfg.TVPause.Enabled {
		chosen := map[string]bool{}
		for _, u := range cfg.TVPause.Devices {
			chosen[u] = true
		}
		n := 0
		for _, d := range devices {
			if (err == nil && strings.EqualFold(d.Name, speaker.Name)) || (len(chosen) > 0 && !chosen[d.UUID]) || n >= maxWatchedTVs {
				continue
			}
			before := len(out)
			add(d, "tv")
			if len(out) > before {
				n++
			}
		}
	}

	for _, want := range cfg.Journal.WatchDevices {
		for _, d := range devices {
			if strings.EqualFold(d.Name, want) {
				add(d, "extra")
				break
			}
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// journalSpec describes the recording of one scheduled job.
func journalSpec(cfg *config.Config, prayer, label string, at time.Time) journal.Spec {
	spec := journal.Spec{Prayer: prayer, Label: label, ScheduledFor: at}
	switch prayer {
	case "pre_fajr":
		spec.Kind, spec.Prayer = "quran", ""
		spec.ExpectPlayback = cfg.Prayer.Channels.PreFajr.Speaker
		spec.MaxDuration = time.Duration(cfg.Prayer.PreFajrMinutes)*time.Minute + 3*time.Minute
	case "friday_kahf":
		spec.Kind, spec.Prayer = "kahf", ""
		spec.ExpectPlayback = cfg.Prayer.Channels.FridayKahf.Speaker
		spec.MaxDuration = 60 * time.Minute
	default:
		spec.Kind = "athan"
		spec.ExpectPlayback = cfg.Prayer.Channels.ForPrayer(prayer).Speaker
	}
	return spec
}
