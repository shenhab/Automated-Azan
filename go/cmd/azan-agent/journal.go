package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
	"azan-agent/internal/journal"
	"azan-agent/internal/trace"
)

// deviceSource is what the Journal needs to know about the network; the Cast
// manager provides it.
type deviceSource interface {
	SpeakerDevice() (chromecast.Device, error)
	Devices() []chromecast.Device
	Membership(ctx context.Context) (chromecast.Membership, error)
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
		ms, merr := src.Membership(context.Background())
		if merr == nil && (len(ms.Configured) > 0 || len(ms.Joined) > 0 || len(ms.Offline) > 0) {
			// Everyone configured into the group, or who the group says has
			// joined, wherever they are on the network.
			notJoined := map[string]bool{}
			for _, d := range append(append([]chromecast.Device(nil), ms.NotJoined...), ms.Offline...) {
				notJoined[d.Host+":"+itoa(d.Port)] = true
			}
			// Unreachable members stay watched, so that their return is seen.
			for _, d := range append(append(append([]chromecast.Device(nil), ms.Configured...), ms.Joined...), ms.Offline...) {
				before := len(out)
				add(d, "member")
				if len(out) > before {
					out[before].NotJoined = notJoined[d.Host+":"+itoa(d.Port)]
				}
			}
		} else {
			// Neither the group nor its members could be asked: fall back to what
			// shares the group's address (a group is hosted by one of its members).
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

// announceMembership puts who is in the group on the session's timeline, before
// the cast: who is there, and above all who will not play.
func announceMembership(ms chromecast.Membership) {
	if !ms.IsGroup() {
		return
	}
	live := map[string]chromecast.GroupMember{}
	for _, l := range ms.Live {
		live[strings.ToLower(l.Name)] = l
	}
	var parts, configured, joined, notJoined, offline []string
	for _, d := range ms.Configured {
		configured = append(configured, d.Name)
		part := d.Name
		if l, ok := live[strings.ToLower(d.Name)]; ok {
			part += fmt.Sprintf(" (volume %.2f)", l.Level)
			if l.Muted {
				part = d.Name + fmt.Sprintf(" (volume %.2f, muted)", l.Level)
			}
		} else if ms.LiveKnown {
			part += " (not joined)"
		}
		parts = append(parts, part)
	}
	for _, d := range ms.Joined {
		joined = append(joined, d.Name)
		found := false
		for _, c := range ms.Configured {
			found = found || (c.Host == d.Host && c.Port == d.Port)
		}
		if !found { // its own page could not be read, but the group lists it
			part := d.Name
			if l, ok := live[strings.ToLower(d.Name)]; ok {
				part += fmt.Sprintf(" (volume %.2f)", l.Level)
			}
			parts = append(parts, part)
		}
	}
	for _, d := range ms.NotJoined {
		notJoined = append(notJoined, d.Name)
	}
	for _, d := range ms.Offline {
		offline = append(offline, d.Name)
		parts = append(parts, d.Name+" (cannot be reached)")
	}
	var asked []string
	for name := range ms.Errors {
		asked = append(asked, name)
	}
	sort.Strings(asked)

	text := ms.Group.Name + " is a group of " + strings.Join(parts, ", ")
	if !ms.LiveKnown {
		text += "; the group did not say who has joined"
	}
	trace.Agent("group-members", ms.Group.Name, text, "host", ms.Group.Host,
		"configured", configured, "joined", joined, "not_joined", notJoined, "offline", offline, "could_not_ask", asked)
	for _, d := range ms.Offline {
		trace.Warn(trace.SourceAgent, "member-offline", d.Name,
			d.Name+" is in "+ms.Group.Name+" but cannot be reached — it will not play", "host", d.Host, "group", ms.Group.Name)
	}
	for _, d := range ms.NotJoined {
		trace.Warn(trace.SourceAgent, "member-not-joined", d.Name,
			d.Name+" is in "+ms.Group.Name+" but has not joined — it will not play", "host", d.Host, "group", ms.Group.Name)
	}
	for _, u := range ms.Unknown {
		trace.Warn(trace.SourceAgent, "member-unknown", u.Name,
			u.Name+" has joined "+ms.Group.Name+" but was not found on the network, so it cannot be watched", "group", ms.Group.Name)
	}
}

// cachedMembership answers with what is already known and refreshes in the
// background: for a person who just pressed Play, who should not wait for every
// device on the network to answer.
type cachedMembership struct {
	deviceSource
	last    func() (chromecast.Membership, bool)
	refresh func()
}

func (c cachedMembership) Membership(context.Context) (chromecast.Membership, error) {
	go c.refresh()
	if ms, ok := c.last(); ok {
		return ms, nil
	}
	return chromecast.Membership{}, errors.New("group membership not known yet")
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
