package journal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"

	"azan-agent/internal/fakecast"
	"azan-agent/internal/trace"
)

type rig struct {
	j     *Journal
	sp    *fakecast.Speaker
	media string
	host  string
	port  int
}

func newRig(t *testing.T, mutate ...func(*Options)) *rig {
	t.Helper()
	oldRetry := watcherRetry
	watcherRetry = 100 * time.Millisecond
	t.Cleanup(func() { watcherRetry = oldRetry })
	sp := fakecast.New("Test Speaker")
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(append([]byte("ID3"), make([]byte, 4096)...))
	})
	media := httptest.NewServer(mux)
	t.Cleanup(media.Close)
	host, port := sp.Addr()
	r := &rig{sp: sp, media: media.URL + "/media/x.mp3", host: host, port: port}
	j, _ := newJournal(t, append([]func(*Options){func(o *Options) {
		o.PollEvery, o.Tail, o.StartTimeout = 40*time.Millisecond, 300*time.Millisecond, 2*time.Second
		o.Targets = func(Spec) []Target {
			return []Target{{Name: "Test Speaker", Host: host, Port: port, Role: "speaker"}}
		}
		o.Health = func(h string) (HealthSnapshot, error) { return HealthSnapshot{}, fmt.Errorf("none") }
	}}, mutate...)...)
	r.j = j
	return r
}

// agentLoads is what the agent does at prayer time: say so, then ask the speaker.
func (r *rig) agentLoads(t *testing.T) {
	t.Helper()
	trace.SetSink(r.j)
	t.Cleanup(func() { trace.SetSink(nil) })
	trace.Agent("load-sent", "Test Speaker", "Load sent", "host", r.host)
	app := application.NewApplication()
	if err := app.Start(r.host, r.port); err != nil {
		t.Fatal(err)
	}
	defer app.Close(false)
	if err := app.Load(r.media, 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (r *rig) seen(id, kind, contains string) bool {
	d, err := r.j.Detail(id)
	if err != nil {
		return false
	}
	for _, e := range d.Events {
		if e.Kind == kind && strings.Contains(e.Text, contains) {
			return true
		}
	}
	return false
}

func (r *rig) finished(t *testing.T) Summary {
	t.Helper()
	waitFor(t, "the session to end", func() bool {
		s := r.j.Sessions(time.Time{})
		return len(s) == 1 && s[0].Verdict != VerdictRunning
	})
	return r.j.Sessions(time.Time{})[0]
}

func TestTheWatcherRecordsAnAthanFromTheSpeakersOwnReports(t *testing.T) {
	r := newRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "PLAYING on the timeline", func() bool { return r.seen(run.ID(), "player-state", "PLAYING") })
	r.sp.FinishPlayback()

	s := r.finished(t)
	if s.Verdict != VerdictOK || !strings.Contains(s.Headline, "finished normally") {
		t.Fatalf("%s: %s", s.Verdict, s.Headline)
	}
	if s.EndedBy != "playback ended" {
		t.Errorf("ended by %q (the watcher should end a session itself once the playback is over)", s.EndedBy)
	}
	d, _ := r.j.Detail(s.ID)
	if len(d.Raw) != 1 || d.Raw[0].Bytes == 0 {
		t.Fatalf("raw recording missing: %+v", d.Raw)
	}
	rc, err := r.j.Raw(s.ID, "Test Speaker", false)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 400)
	n, _ := rc.Read(buf)
	if !strings.Contains(string(buf[:n]), "RECEIVER_STATUS") && !strings.Contains(string(buf[:n]), "CONNECT") {
		t.Errorf("raw recording does not look like Cast frames: %s", buf[:n])
	}
}

func TestSomeoneElseStoppingTheSpeakerIsMarkedAsOutsideInterference(t *testing.T) {
	// The 23:38 case: the app closed seconds into the Athan and the agent had not stopped it.
	r := newRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "PLAYING", func() bool { return r.seen(run.ID(), "player-state", "PLAYING") })

	other := application.NewApplication() // another sender: a phone, say
	if err := other.Start(r.host, r.port); err != nil {
		t.Fatal(err)
	}
	defer other.Close(false)
	if err := other.Stop(); err != nil {
		t.Fatal(err)
	}

	s := r.finished(t)
	if s.Verdict != VerdictWarning || !strings.Contains(s.Headline, "something else") {
		t.Fatalf("%s: %s", s.Verdict, s.Headline)
	}
	d, _ := r.j.Detail(s.ID)
	external := 0
	for _, e := range d.Events {
		if e.External {
			external++
		}
	}
	if external == 0 {
		t.Error("no event was marked external")
	}
}

func TestAStopTheAgentSentIsNotInterference(t *testing.T) {
	r := newRig(t)
	run := r.j.Begin(Spec{Kind: "quran", Label: "Quran", ExpectPlayback: true})
	r.agentLoads(t)
	waitFor(t, "PLAYING", func() bool { return r.seen(run.ID(), "player-state", "PLAYING") })

	trace.Agent("stop-sent", "Test Speaker", "Stop sent", "host", r.host)
	app := application.NewApplication()
	if err := app.Start(r.host, r.port); err != nil {
		t.Fatal(err)
	}
	defer app.Close(false)
	if err := app.Stop(); err != nil {
		t.Fatal(err)
	}

	s := r.finished(t)
	if s.Verdict != VerdictOK || !s.Facts.StoppedByAgent || len(s.Facts.External) != 0 {
		t.Fatalf("%s: %s (external %v)", s.Verdict, s.Headline, s.Facts.External)
	}
}

func TestAVolumeChangeByAnotherSenderIsFlagged(t *testing.T) {
	r := newRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "PLAYING", func() bool { return r.seen(run.ID(), "player-state", "PLAYING") })

	other := application.NewApplication()
	if err := other.Start(r.host, r.port); err != nil {
		t.Fatal(err)
	}
	defer other.Close(false)
	if err := other.SetVolume(0.2); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the volume change on the timeline", func() bool { return r.seen(run.ID(), "volume", "0.50 → 0.20") })
	r.sp.FinishPlayback()

	s := r.finished(t)
	if s.Verdict != VerdictWarning || len(s.Facts.External) == 0 {
		t.Fatalf("%s: %s", s.Verdict, s.Headline)
	}
}

func TestTheWatcherNeverSendsAnythingButStatusRequests(t *testing.T) {
	r := newRig(t, func(o *Options) { o.StartTimeout = 400 * time.Millisecond })
	r.j.Begin(athanSpec) // watched for a while, nothing plays
	r.finished(t)
	for _, kind := range []string{"launch", "load", "stop", "mute", "unmute", "volume", "pause", "play", "media-stop"} {
		if ev := r.sp.EventsOfKind(kind); len(ev) != 0 {
			t.Errorf("the watcher caused %d %q event(s) on the speaker", len(ev), kind)
		}
	}
}

func TestAnAthanTheAgentNeverStartedFailsAfterTheStartTimeout(t *testing.T) {
	r := newRig(t, func(o *Options) { o.StartTimeout = 300 * time.Millisecond })
	r.j.Begin(athanSpec)
	s := r.finished(t)
	if s.Verdict != VerdictFailed || !strings.Contains(s.Headline, "Never started") {
		t.Fatalf("%s: %s", s.Verdict, s.Headline)
	}
}

func TestTheWatcherSurvivesAConnectionDrop(t *testing.T) {
	r := newRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "PLAYING", func() bool { return r.seen(run.ID(), "player-state", "PLAYING") })

	r.sp.DropConnections()
	waitFor(t, "the outage on the timeline", func() bool { return r.seen(run.ID(), "unreachable", "") })
	waitFor(t, "the reconnection", func() bool { return r.seen(run.ID(), "reconnected", "") })
	r.sp.FinishPlayback()
	s := r.finished(t)
	if !s.Facts.Started || s.Facts.EndReason != "FINISHED" {
		t.Fatalf("the Athan was not followed through the drop: %+v / %s", s.Facts, s.Headline)
	}
}

func TestADeviceThatCannotBeReachedIsReportedAndDoesNotStopTheSession(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.StartTimeout = 300 * time.Millisecond
		o.Targets = func(Spec) []Target { return []Target{{Name: "Ghost", Host: "127.0.0.1", Port: 1, Role: "speaker"}} }
	})
	run := r.j.Begin(athanSpec)
	waitFor(t, "unreachable on the timeline", func() bool { return r.seen(run.ID(), "unreachable", "Ghost") || r.seen(run.ID(), "unreachable", "") })
	s := r.finished(t)
	if s.Verdict != VerdictFailed {
		t.Errorf("verdict = %s", s.Verdict)
	}
}

func TestASessionNeverOutlastsItsTimeLimit(t *testing.T) {
	r := newRig(t, func(o *Options) { o.StartTimeout = time.Hour })
	r.j.Begin(Spec{Kind: "athan", Label: "Capped", ExpectPlayback: true, MaxDuration: 400 * time.Millisecond})
	s := r.finished(t)
	if s.EndedBy != "time limit" {
		t.Errorf("ended by %q", s.EndedBy)
	}
}

func TestTheLiveViewShowsWhatEachWatchedDeviceIsDoingNow(t *testing.T) {
	r := newRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "PLAYING", func() bool { return r.seen(run.ID(), "player-state", "PLAYING") })

	live := r.j.Live()
	if len(live) != 1 || live[0].ID != run.ID() || len(live[0].Devices) != 1 {
		t.Fatalf("live = %+v", live)
	}
	d := live[0].Devices[0]
	if d.Name != "Test Speaker" || d.Role != "speaker" || d.State != "PLAYING" || d.App != "Default Media Receiver" ||
		d.Level != 0.5 || d.Muted || !d.Reachable {
		t.Errorf("device = %+v", d)
	}
	r.sp.FinishPlayback()
	r.finished(t)
	if got := r.j.Live(); len(got) != 0 {
		t.Errorf("a finished session is still live: %+v", got)
	}
}

func TestAGroupMemberOnAnotherDeviceJoiningOurPlaybackIsNotInterference(t *testing.T) {
	// The Isha group is hosted by the Nest display; its Kitchen speaker is a
	// different device with its own address. When our Load makes it start the
	// media receiver, that is the agent's doing, not a stranger's.
	member := Target{Name: "Kitchen speaker", Host: "10.9.9.9", Port: 8009, Role: "member"}
	r := newRig(t, func(o *Options) {
		inner := o.Targets
		o.Targets = func(s Spec) []Target { return append(inner(s), member) }
	})
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "PLAYING", func() bool { return r.seen(run.ID(), "player-state", "PLAYING") })

	run.deviceChange(member, change{At: time.Now(), Kind: "app-launched", Fields: map[string]any{"app": "Default Media Receiver"}})

	d, _ := r.j.Detail(run.ID())
	for _, e := range d.Events {
		if e.Device == "Kitchen speaker" && e.Kind == "app-launched" {
			if e.External {
				t.Errorf("the member joining our playback was marked as outside interference: %+v", e)
			}
			return
		}
	}
	t.Error("the member's event was not recorded")
}

// memberRig: a group plus one joined member and one that is not.
func newMemberRig(t *testing.T) (*rig, *fakecast.Speaker, *fakecast.Speaker) {
	t.Helper()
	start := func(name string) *fakecast.Speaker {
		s := fakecast.New(name)
		if err := s.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		return s
	}
	joined, out := start("Kitchen speaker"), start("Playroom Speaker")
	jh, jp := joined.Addr()
	oh, op := out.Addr()
	r := newRig(t, func(o *Options) {
		inner := o.Targets
		o.Targets = func(s Spec) []Target {
			return append(inner(s),
				Target{Name: "Kitchen speaker", Host: jh, Port: jp, Role: "member"},
				Target{Name: "Playroom Speaker", Host: oh, Port: op, Role: "member", NotJoined: true})
		}
	})
	joined.JoinGroup(r.sp)
	return r, joined, out
}

func TestAMemberShowingTheGroupsSessionIsRecordedAsHavingPlayed(t *testing.T) {
	r, _, _ := newMemberRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "the member to be seen playing", func() bool { return r.seen(run.ID(), "member-played", "Kitchen speaker") })
	r.sp.FinishPlayback()
	s := r.finished(t)
	if strings.Join(s.Facts.MembersPlayed, ",") != "Kitchen speaker" {
		t.Errorf("played = %v", s.Facts.MembersPlayed)
	}
	d, _ := r.j.Detail(s.ID)
	n := 0
	for _, e := range d.Events {
		if e.Kind == "member-played" {
			n++
			if e.Device != "Kitchen speaker" {
				t.Errorf("%+v", e)
			}
		}
		if e.External {
			t.Errorf("marked as outside interference: %+v", e)
		}
	}
	if n != 1 {
		t.Errorf("%d member-played events, want 1", n)
	}
}

func TestSessionsDoNotConfuseAMemberPlayingSomethingElse(t *testing.T) {
	r, _, out := newMemberRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	// The Playroom Speaker is busy with something of its own, in another session.
	host, port := out.Addr()
	other := application.NewApplication()
	if err := other.Start(host, port); err != nil {
		t.Fatal(err)
	}
	defer other.Close(false)
	if err := other.Load(r.media, 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the joined member", func() bool { return r.seen(run.ID(), "member-played", "Kitchen speaker") })
	time.Sleep(200 * time.Millisecond)
	if r.seen(run.ID(), "member-played", "Playroom Speaker") {
		t.Error("a speaker in its own session was recorded as having played the Athan")
	}
}

func TestAMembersWatcherKeepsWatchingWhileTheGroupPlays(t *testing.T) {
	// Isha 2026-11-02: as the group started, the Nest display's watcher asked for
	// the media status of an app with no transport, the device closed the channel
	// and the watcher reported "lost contact" and saw nothing more for 3 minutes.
	r, kitchen, _ := newMemberRig(t)
	run := r.j.Begin(athanSpec)
	r.agentLoads(t)
	waitFor(t, "the member to be seen playing", func() bool { return r.seen(run.ID(), "member-played", "Kitchen speaker") })
	time.Sleep(pollTimeout + 500*time.Millisecond)
	if r.seen(run.ID(), "unreachable", "") {
		t.Error("the member's watcher lost contact while the group played")
	}
	kitchen.SetVolumeState(0.1, false)
	waitFor(t, "the member's volume change", func() bool { return r.seen(run.ID(), "volume", "0.10") })
}
