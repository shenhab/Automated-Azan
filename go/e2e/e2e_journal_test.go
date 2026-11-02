//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"

	"azan-agent/internal/fakecast"
)

type jrSession struct {
	ID       string `json:"id"`
	Verdict  string `json:"verdict"`
	Headline string `json:"headline"`
	EndedBy  string `json:"ended_by"`
	Heard    string `json:"heard"`
	Spec     struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
	} `json:"spec"`
	Facts struct {
		Started        bool     `json:"started"`
		External       []string `json:"external"`
		StoppedByAgent bool     `json:"stopped_by_agent"`
	} `json:"facts"`
}

type jrEvent struct {
	At       time.Time      `json:"at"`
	Source   string         `json:"source"`
	Kind     string         `json:"kind"`
	Device   string         `json:"device"`
	Text     string         `json:"text"`
	External bool           `json:"external"`
	Fields   map[string]any `json:"fields"`
}

type jrList struct {
	Success  bool        `json:"success"`
	Error    string      `json:"error"`
	Sessions []jrSession `json:"sessions"`
	Stats    struct {
		Total     int `json:"total"`
		OK        int `json:"ok"`
		Warning   int `json:"warning"`
		HeardYes  int `json:"heard_yes"`
		HeardNo   int `json:"heard_no"`
		Mismatchs int `json:"mismatches"`
	} `json:"stats"`
}

type jrDetail struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Detail  struct {
		Summary jrSession `json:"summary"`
		Events  []jrEvent `json:"events"`
		Raw     []struct {
			Device string `json:"device"`
			Bytes  int64  `json:"bytes"`
		} `json:"raw"`
		Notes []struct {
			Heard string `json:"heard"`
			Text  string `json:"text"`
		} `json:"notes"`
	} `json:"detail"`
}

// journalAgent starts an agent whose Athan fires about 100 s from now and returns
// everything a Journal test needs.
func journalAgent(t *testing.T, extraCfg string) (*fakecast.Speaker, *client, time.Time) {
	t.Helper()
	sp := fakecast.New(speakerName)
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
	fireAt := nextFireTime(100 * time.Second)
	writeNaasTimetable(t, home, fireAt, "Asr")
	writeConfig(t, cfgFile, webPort, "Asr", extraCfg)
	startAgent(t, home, cfgFile, sp, withCredentials(testUser, testPass))
	api := newClient(t, webPort)
	api.mustLogin()
	return sp, api, fireAt
}

func waitJournal(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func journalSessions(api *client) jrList {
	var l jrList
	api.getJSON("/api/journal/sessions?days=1", &l)
	return l
}

func journalDetail(t *testing.T, api *client, id string) jrDetail {
	t.Helper()
	var d jrDetail
	api.getJSON("/api/journal/session?id="+id, &d)
	if !d.Success {
		t.Fatalf("session %s: %s", id, d.Error)
	}
	return d
}

func hasEvent(evs []jrEvent, source, kind, textHas string) bool {
	for _, e := range evs {
		if e.Source == source && e.Kind == kind && strings.Contains(e.Text, textHas) {
			return true
		}
	}
	return false
}

func indexOf(evs []jrEvent, source, kind, textHas string) int {
	for i, e := range evs {
		if e.Source == source && e.Kind == kind && strings.Contains(e.Text, textHas) {
			return i
		}
	}
	return -1
}

func TestTheJournalRecordsAScheduledAthanEndToEnd(t *testing.T) {
	t.Parallel()
	sp, api, fireAt := journalAgent(t, "")

	// While the Athan plays, the live view shows the speaker playing.
	if _, err := sp.WaitForLoads(1, time.Until(fireAt)+90*time.Second); err != nil {
		t.Fatalf("the Athan never reached the speaker: %v", err)
	}
	var live struct {
		Live []struct {
			Label   string `json:"label"`
			Devices []struct {
				Name  string `json:"name"`
				State string `json:"state"`
				Role  string `json:"role"`
			} `json:"devices"`
		} `json:"live"`
	}
	waitJournal(t, "the live view to show the speaker playing", 30*time.Second, func() bool {
		api.getJSON("/api/journal/live", &live)
		for _, r := range live.Live {
			for _, d := range r.Devices {
				if d.Role == "speaker" && d.State == "PLAYING" {
					return true
				}
			}
		}
		return false
	})
	if len(live.Live) != 1 || !strings.Contains(live.Live[0].Label, "Asr") {
		t.Errorf("live = %+v", live.Live)
	}

	// Let the agent finish verifying that it is playing, then play out a few seconds.
	var status struct {
		Last struct {
			Started bool `json:"started"`
		} `json:"last_playback"`
	}
	waitJournal(t, "the agent to verify the playback", 30*time.Second, func() bool {
		api.getJSON("/api/scheduler-status", &status)
		return status.Last.Started
	})
	time.Sleep(3 * time.Second)

	// The Athan ends; the Journal notices by itself, shortly afterwards judges it.
	sp.FinishPlayback()
	var s jrSession
	waitJournal(t, "the Athan to be judged", 90*time.Second, func() bool {
		for _, x := range journalSessions(api).Sessions {
			if x.Spec.Kind == "athan" && x.Verdict != "running" {
				s = x
				return true
			}
		}
		return false
	})
	if s.Verdict != "ok" || !strings.Contains(s.Headline, "finished normally") || s.EndedBy != "playback ended" {
		t.Fatalf("judged %s: %q (ended by %q)", s.Verdict, s.Headline, s.EndedBy)
	}

	// The timeline holds both sides, in the order things happened.
	d := journalDetail(t, api, s.ID)
	evs := d.Detail.Events
	load := indexOf(evs, "agent", "load-sent", "")
	playing := indexOf(evs, "device", "player-state", "PLAYING")
	ended := indexOf(evs, "device", "player-state", "FINISHED")
	if load < 0 || playing < 0 || ended < 0 || !(load < playing && playing < ended) {
		t.Errorf("timeline order wrong: load-sent=%d playing=%d finished=%d", load, playing, ended)
	}
	for _, want := range [][3]string{
		{"agent", "pre-connect", ""}, {"agent", "verified", "PLAYING"}, {"system", "prayer-fired", ""},
		{"media", "fetch", "media_Athan.mp3"}, {"device", "initial-state", ""},
	} {
		if !hasEvent(evs, want[0], want[1], want[2]) {
			t.Errorf("timeline lacks %v", want)
			for _, e := range evs {
				t.Logf("  %s %-7s %-14s %s", e.At.Format("15:04:05.000"), e.Source, e.Kind, e.Text)
			}
		}
	}
	for _, e := range evs {
		if e.External {
			t.Errorf("an event was blamed on someone else in a clean run: %+v", e)
		}
	}

	// The raw recording downloads, and the shareable copy is scrubbed.
	if len(d.Detail.Raw) != 1 {
		t.Fatalf("raw recordings = %+v", d.Detail.Raw)
	}
	st, _, private := api.get("/api/journal/raw?id=" + s.ID + "&device=" + strings.ReplaceAll(speakerName, " ", "%20"))
	if st != 200 || !strings.Contains(string(private), "RECEIVER_STATUS") {
		t.Errorf("private recording: status %d, %.120s", st, private)
	}
	_, _, shared := api.get("/api/journal/raw?scrub=1&id=" + s.ID + "&device=" + strings.ReplaceAll(speakerName, " ", "%20"))
	if strings.Contains(string(shared), "127.0.0.1") || !strings.Contains(string(shared), "RECEIVER_STATUS") {
		t.Errorf("the shareable recording still identifies the household (or is empty): %.200s", shared)
	}
	if st, _, _ := api.get("/api/journal/raw?id=../../etc&device=x"); st != 404 {
		t.Errorf("a path-traversal id got status %d", st)
	}

	// The person says what they heard; the Journal keeps it.
	var noted jrDetail
	if api.postJSON("/api/journal/note", map[string]string{"id": s.ID, "heard": "yes", "text": "clear"}, &noted); !noted.Success {
		t.Fatalf("note rejected: %s", noted.Error)
	}
	l := journalSessions(api)
	if l.Stats.HeardYes != 1 || l.Sessions[0].Heard != "yes" || len(noted.Detail.Notes) != 1 {
		t.Errorf("note not reflected: stats %+v, notes %+v", l.Stats, noted.Detail.Notes)
	}
	var bad jrDetail
	api.postJSON("/api/journal/note", map[string]string{"id": s.ID, "heard": "maybe"}, &bad)
	if bad.Success {
		t.Error("a nonsense answer was accepted")
	}

	// The pages show it.
	for path, want := range map[string]string{
		"/journal":   "Journal",
		"/":          "lastAthanCard",
		"/settings":  "saveJournalSettings",
		"/scheduler": `href="/journal"`,
	} {
		if status, _, body := api.get(path); status != 200 || !strings.Contains(string(body), want) {
			t.Errorf("GET %s: status %d, missing %q", path, status, want)
		}
	}
}

func TestSomeoneElseStoppingTheAthanIsFlaggedByTheJournal(t *testing.T) {
	t.Parallel()
	sp, api, fireAt := journalAgent(t, "")
	if _, err := sp.WaitForLoads(1, time.Until(fireAt)+90*time.Second); err != nil {
		t.Fatalf("the Athan never reached the speaker: %v", err)
	}
	waitJournal(t, "the speaker to be playing", 30*time.Second, func() bool { return sp.PlayerState() == "PLAYING" })
	time.Sleep(3 * time.Second) // the Journal's watcher sees it playing

	// Another sender, say a phone, stops the speaker.
	host, port := sp.Addr()
	phone := application.NewApplication()
	if err := phone.Start(host, port); err != nil {
		t.Fatal(err)
	}
	defer phone.Close(false)
	if err := phone.Stop(); err != nil {
		t.Fatal(err)
	}

	var s jrSession
	waitJournal(t, "the Athan to be judged", 90*time.Second, func() bool {
		for _, x := range journalSessions(api).Sessions {
			if x.Spec.Kind == "athan" && x.Verdict != "running" {
				s = x
				return true
			}
		}
		return false
	})
	if s.Verdict != "warning" || !strings.Contains(s.Headline, "something else") || len(s.Facts.External) == 0 {
		t.Fatalf("judged %s: %q (external %v)", s.Verdict, s.Headline, s.Facts.External)
	}
	d := journalDetail(t, api, s.ID)
	flagged := false
	for _, e := range d.Detail.Events {
		if e.External && e.Source == "device" {
			flagged = true
		}
	}
	if !flagged {
		t.Error("no device event was marked as not caused by the agent")
	}
	if n := len(sp.EventsOfKind("stop")); n != 1 {
		t.Errorf("the speaker saw %d stop(s); only the phone's should have reached it", n)
	}
}

func TestTheJournalCanBeSwitchedOffAndItsSettingsAreChecked(t *testing.T) {
	t.Parallel()
	sp := fakecast.New(speakerName)
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
	writeNaasTimetable(t, home, nextFireTime(10*time.Minute), "Asr")
	writeConfig(t, cfgFile, webPort, "Asr", "\n[journal]\nenabled = false\n")
	startAgent(t, home, cfgFile, sp, withCredentials(testUser, testPass))
	api := newClient(t, webPort)
	api.mustLogin()

	var l jrList
	api.getJSON("/api/journal/sessions", &l)
	if l.Success || !strings.Contains(l.Error, "switched off") {
		t.Errorf("a switched-off Journal answered: %+v", l)
	}
	if st, _, body := api.get("/journal"); st != 200 || !strings.Contains(string(body), "switched off") {
		t.Errorf("the page does not say the Journal is off (status %d)", st)
	}

	// Settings: bad values are refused, good ones are saved to the config file.
	for _, bad := range []map[string]any{
		{"journal_retention_days": 0}, {"journal_retention_days": 9999}, {"journal_max_mb": 1}, {"journal_notify_on": "sometimes"},
	} {
		var r apiResp
		api.postJSON("/api/config", bad, &r)
		if r.Success {
			t.Errorf("accepted %v", bad)
		}
	}
	var ok apiResp
	api.postJSON("/api/config", map[string]any{"journal_enabled": true, "journal_retention_days": 7, "journal_max_mb": 120,
		"journal_notify_on": "failures", "journal_watch_devices": []string{"Kitchen speaker"}}, &ok)
	if !ok.Success {
		t.Fatalf("good settings refused: %s", ok.Error)
	}
	cfg := readFile(t, cfgFile)
	for _, want := range []string{"retention_days = 7", "max_mb = 120", `notify_on = "failures"`, "Kitchen speaker"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config file lacks %q:\n%s", want, cfg)
		}
	}
}

func TestAPlayFromTheWebAppIsRecordedLikeAScheduledOne(t *testing.T) {
	t.Parallel()
	sp := fakecast.New(speakerName)
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
	writeNaasTimetable(t, home, nextFireTime(30*time.Minute), "Asr")
	writeConfig(t, cfgFile, webPort, "Asr")
	startAgent(t, home, cfgFile, sp, withCredentials(testUser, testPass))
	api := newClient(t, webPort)
	api.mustLogin()

	var played apiResp
	if api.postJSON("/api/play-athan", map[string]string{"prayer": "Dhuhr"}, &played); !played.Success {
		t.Fatalf("play failed: %s", played.Error)
	}
	time.Sleep(3 * time.Second)
	sp.FinishPlayback()

	var s jrSession
	waitJournal(t, "the manual play to be judged", 90*time.Second, func() bool {
		for _, x := range journalSessions(api).Sessions {
			if x.Spec.Kind == "manual" && x.Verdict != "running" {
				s = x
				return true
			}
		}
		return false
	})
	if s.Verdict != "ok" || !strings.Contains(s.Spec.Label, "Play button") || !strings.Contains(s.Headline, "finished normally") {
		t.Fatalf("judged %s: %q (%q)", s.Verdict, s.Headline, s.Spec.Label)
	}
	d := journalDetail(t, api, s.ID)
	if !hasEvent(d.Detail.Events, "system", "manual-action", "Play button") || !hasEvent(d.Detail.Events, "agent", "load-sent", "") {
		t.Errorf("the request and the Load are not on the timeline: %d events", len(d.Detail.Events))
	}
}
