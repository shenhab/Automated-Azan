package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
	"azan-agent/internal/trace"
)

type stubDevices struct {
	speaker  chromecast.Device
	err      error
	all      []chromecast.Device
	ms       chromecast.Membership
	groupErr error
}

func (s stubDevices) SpeakerDevice() (chromecast.Device, error) { return s.speaker, s.err }
func (s stubDevices) Devices() []chromecast.Device              { return s.all }
func (s stubDevices) Membership(context.Context) (chromecast.Membership, error) {
	return s.ms, s.groupErr
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

func TestTheJournalWatchesEveryMemberConfiguredOrJoinedWhereverTheyAre(t *testing.T) {
	// Measured live 2026-11-02: athan is configured with the Nest display, the
	// Kitchen speaker and the Playroom Speaker; only the first two had joined. The
	// old rule (same address as the group) missed both Nest Minis and took in
	// groups that merely live on the Nest display.
	nest := dev("Nest display", "10.0.0.5", 8009, "n")
	kitchen := dev("Kitchen speaker", "10.0.0.6", 8009, "k")
	playroom := dev("Playroom Speaker", "10.0.0.7", 8009, "p")
	src := stubDevices{
		speaker: dev("athan", "10.0.0.5", 32067, "g"),
		all: []chromecast.Device{
			dev("athan", "10.0.0.5", 32067, "g"), nest, kitchen, playroom,
			dev("Upstairs", "10.0.0.5", 32003, "u"),
		},
		ms: chromecast.Membership{
			Configured: []chromecast.Device{kitchen, nest, playroom},
			Live:       []chromecast.GroupMember{{Name: "Nest display"}, {Name: "Kitchen speaker"}},
			LiveKnown:  true,
			Joined:     []chromecast.Device{kitchen, nest},
			NotJoined:  []chromecast.Device{playroom},
		},
	}
	cfg := &config.Config{}
	cfg.Journal.Enabled = true
	targets := journalTargets(cfg, src)
	if got := roles(cfg, src); got != "athan=speaker,Kitchen speaker=member,Nest display=member,Playroom Speaker=member" {
		t.Errorf("targets = %s", got)
	}
	for _, x := range targets {
		if x.NotJoined != (x.Name == "Playroom Speaker") {
			t.Errorf("%s: NotJoined = %v", x.Name, x.NotJoined)
		}
	}
}

func TestAMemberKnownOnlyFromOneSourceIsStillWatched(t *testing.T) {
	kitchen := dev("Kitchen speaker", "10.0.0.6", 8009, "k")
	playroom := dev("Playroom Speaker", "10.0.0.7", 8009, "p")
	src := stubDevices{
		speaker: dev("athan", "10.0.0.5", 32067, "g"),
		all:     []chromecast.Device{dev("athan", "10.0.0.5", 32067, "g"), kitchen, playroom},
		ms: chromecast.Membership{
			Configured: []chromecast.Device{kitchen}, // the Playroom's page could not be read…
			LiveKnown:  true,
			Live:       []chromecast.GroupMember{{Name: "Playroom Speaker"}},
			Joined:     []chromecast.Device{playroom}, // …but the group says it has joined
		},
	}
	cfg := &config.Config{}
	if got := roles(cfg, src); got != "athan=speaker,Kitchen speaker=member,Playroom Speaker=member" {
		t.Errorf("targets = %s", got)
	}
}

func recordEvents(t *testing.T) *eventLog {
	t.Helper()
	l := &eventLog{}
	trace.SetSink(l)
	t.Cleanup(func() { trace.SetSink(nil) })
	return l
}

type eventLog struct {
	mu  sync.Mutex
	evs []trace.Event
}

func (l *eventLog) Event(e trace.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evs = append(l.evs, e)
}

func (l *eventLog) kind(k string) []trace.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []trace.Event
	for _, e := range l.evs {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

func TestTheOwnerIsToldBeforeTheCastWhoWillNotPlay(t *testing.T) {
	l := recordEvents(t)
	nest := dev("Nest display", "10.0.0.5", 8009, "n")
	kitchen := dev("Kitchen speaker", "10.0.0.6", 8009, "k")
	playroom := dev("Playroom Speaker", "10.0.0.7", 8009, "p")
	announceMembership(chromecast.Membership{
		Group:      dev("athan", "10.0.0.5", 32067, "g"),
		Configured: []chromecast.Device{kitchen, nest, playroom},
		Live: []chromecast.GroupMember{
			{Name: "Nest display", Level: 0.5},
			{Name: "Kitchen speaker", Level: 0.8},
			{Name: "Stranger", Level: 0.3},
		},
		LiveKnown: true,
		Joined:    []chromecast.Device{kitchen, nest},
		NotJoined: []chromecast.Device{playroom},
		Unknown:   []chromecast.GroupMember{{Name: "Stranger"}},
	})

	gm := l.kind("group-members")
	if len(gm) != 1 || gm[0].Source != trace.SourceAgent || gm[0].Level == "warn" {
		t.Fatalf("group-members = %+v", gm)
	}
	for _, want := range []string{"athan", "Kitchen speaker (volume 0.80)", "Nest display (volume 0.50)", "Playroom Speaker (not joined)"} {
		if !strings.Contains(gm[0].Text, want) {
			t.Errorf("%q does not mention %q", gm[0].Text, want)
		}
	}
	if got := gm[0].Fields["configured"]; !reflect.DeepEqual(got, []string{"Kitchen speaker", "Nest display", "Playroom Speaker"}) {
		t.Errorf("configured = %v", got)
	}
	if got := gm[0].Fields["joined"]; !reflect.DeepEqual(got, []string{"Kitchen speaker", "Nest display"}) {
		t.Errorf("joined = %v", got)
	}
	nj := l.kind("member-not-joined")
	if len(nj) != 1 || nj[0].Device != "Playroom Speaker" || nj[0].Level != "warn" || !strings.Contains(nj[0].Text, "will not play") {
		t.Errorf("member-not-joined = %+v", nj)
	}
	if u := l.kind("member-unknown"); len(u) != 1 || u[0].Device != "Stranger" || u[0].Level != "warn" {
		t.Errorf("member-unknown = %+v", u)
	}
}

func TestAMemberThatCannotBeReachedIsAnnouncedAndWatched(t *testing.T) {
	l := recordEvents(t)
	nest := dev("Nest display", "10.0.0.5", 8009, "n")
	playroom := dev("Playroom Speaker", "10.0.0.7", 8009, "p")
	ms := chromecast.Membership{
		Group:      dev("athan", "10.0.0.5", 32067, "g"),
		Configured: []chromecast.Device{nest},
		Live:       []chromecast.GroupMember{{Name: "Nest display", Level: 0.5}},
		LiveKnown:  true,
		Joined:     []chromecast.Device{nest},
		Offline:    []chromecast.Device{playroom},
	}
	announceMembership(ms)
	gm := l.kind("group-members")
	if len(gm) != 1 || !strings.Contains(gm[0].Text, "Playroom Speaker (cannot be reached)") {
		t.Errorf("group-members = %+v", gm)
	}
	off := l.kind("member-offline")
	if len(off) != 1 || off[0].Device != "Playroom Speaker" || off[0].Level != "warn" || !strings.Contains(off[0].Text, "will not play") {
		t.Errorf("member-offline = %+v", off)
	}

	cfg := &config.Config{}
	src := stubDevices{speaker: ms.Group, all: []chromecast.Device{ms.Group, nest}, ms: ms}
	targets := journalTargets(cfg, src)
	found := false
	for _, x := range targets {
		if x.Name == "Playroom Speaker" {
			found = x.Role == "member" && x.NotJoined
		}
	}
	if !found {
		t.Errorf("targets = %+v: an unreachable member must stay watched, so its return is seen", targets)
	}
}

func TestAPlainSpeakerIsNotAnnouncedAsAGroup(t *testing.T) {
	l := recordEvents(t)
	sp := dev("Kitchen speaker", "10.0.0.6", 8009, "k")
	announceMembership(chromecast.Membership{Group: sp, Configured: []chromecast.Device{sp}, LiveKnown: true,
		Live: []chromecast.GroupMember{{Name: "Kitchen speaker"}}, Joined: []chromecast.Device{sp}})
	if n := len(l.kind("group-members")); n != 0 {
		t.Errorf("%d group-members events", n)
	}
}

func TestWithoutTheGroupsAnswerTheAnnouncementClaimsNothingAboutWhoJoined(t *testing.T) {
	l := recordEvents(t)
	nest := dev("Nest display", "10.0.0.5", 8009, "n")
	announceMembership(chromecast.Membership{Group: dev("athan", "10.0.0.5", 32067, "g"),
		Configured: []chromecast.Device{nest}, Errors: map[string]string{"athan": "no answer"}})
	gm := l.kind("group-members")
	if len(gm) != 1 || strings.Contains(gm[0].Text, "not joined") || !strings.Contains(gm[0].Text, "did not say") {
		t.Errorf("group-members = %+v", gm)
	}
	if n := len(l.kind("member-not-joined")); n != 0 {
		t.Errorf("%d member-not-joined events", n)
	}
}

func TestAManualPlayIsNotHeldUpByTheMembershipCheck(t *testing.T) {
	// A person pressed Play: use what is known, refresh in the background.
	fresh := make(chan struct{}, 1)
	known := chromecast.Membership{LiveKnown: true, Live: []chromecast.GroupMember{{Name: "Nest display"}}}
	src := cachedMembership{last: func() (chromecast.Membership, bool) { return known, true },
		refresh: func() { fresh <- struct{}{} }}
	ms, err := src.Membership(context.Background())
	if err != nil || len(ms.Live) != 1 {
		t.Errorf("ms=%+v err=%v", ms, err)
	}
	select {
	case <-fresh:
	case <-time.After(2 * time.Second):
		t.Error("no refresh in the background")
	}
	none := cachedMembership{last: func() (chromecast.Membership, bool) { return chromecast.Membership{}, false }, refresh: func() {}}
	if _, err := none.Membership(context.Background()); err == nil {
		t.Error("no error when nothing is known yet")
	}
}
