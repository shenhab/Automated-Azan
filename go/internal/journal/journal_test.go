package journal

import (
	"compress/gzip"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"azan-agent/internal/trace"
)

type notes struct {
	mu   sync.Mutex
	list []string
}

func (n *notes) send(title, msg string) {
	n.mu.Lock()
	n.list = append(n.list, title+": "+msg)
	n.mu.Unlock()
}
func (n *notes) count() int { n.mu.Lock(); defer n.mu.Unlock(); return len(n.list) }

func newJournal(t *testing.T, mutate ...func(*Options)) (*Journal, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "journal")
	o := Options{Dir: dir, Logf: func(string, ...any) {}}
	for _, m := range mutate {
		m(&o)
	}
	j := New(o)
	t.Cleanup(j.Close)
	return j, dir
}

// playedAthan feeds a normal Athan's timeline into a run.
func playedAthan(r *Run) {
	r.add(dev(-1, "volume", "level", 0.5, "muted", false))
	r.add(ag(0, "load-sent"))
	r.add(ps(3, "BUFFERING", ""))
	r.add(ps(3.5, "PLAYING", ""))
	r.add(ps(227, "IDLE", "FINISHED"))
}

func TestASessionIsStoredListedAndReadBack(t *testing.T) {
	j, dir := newJournal(t)
	run := j.Begin(athanSpec)
	playedAthan(run)
	run.End("playback ended")

	list := j.Sessions(time.Time{})
	if len(list) != 1 {
		t.Fatalf("got %d sessions", len(list))
	}
	s := list[0]
	if s.Verdict != VerdictOK || !s.Facts.Started || s.EndedBy != "playback ended" {
		t.Errorf("summary = %+v", s)
	}
	for _, f := range []string{"session.json", "events.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, s.ID, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	d, err := j.Detail(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Events) != 5 || !d.Events[0].At.Before(d.Events[4].At) {
		t.Errorf("events not stored in order: %d", len(d.Events))
	}
}

func TestAgentEventsReachEveryOpenSessionAndOtherwiseTheAmbientLog(t *testing.T) {
	j, dir := newJournal(t)
	j.Event(trace.Event{At: time.Now(), Source: trace.SourceSystem, Kind: "agent-start", Text: "agent started"})
	a, b := j.Begin(athanSpec), j.Begin(Spec{Kind: "manual", Label: "Manual play"})
	j.Event(trace.Event{At: time.Now(), Source: trace.SourceAgent, Kind: "load-sent", Text: "x"})
	a.End("test")
	b.End("test")

	for _, id := range []string{a.ID(), b.ID()} {
		d, err := j.Detail(id)
		if err != nil || len(d.Events) != 1 || d.Events[0].Kind != "load-sent" {
			t.Errorf("%s: %v %+v", id, err, d.Events)
		}
	}
	amb, _ := filepath.Glob(filepath.Join(dir, "ambient-*.jsonl"))
	if len(amb) != 1 {
		t.Fatalf("ambient files = %v", amb)
	}
	if data, _ := os.ReadFile(amb[0]); !strings.Contains(string(data), "agent-start") {
		t.Errorf("ambient log lacks the event: %s", data)
	}
}

func TestTwoSessionsInTheSameSecondGetDifferentIDs(t *testing.T) {
	j, _ := newJournal(t)
	a, b := j.Begin(athanSpec), j.Begin(athanSpec)
	if a.ID() == b.ID() {
		t.Fatalf("both sessions are %q", a.ID())
	}
	a.End("x")
	b.End("x")
}

func TestEndingTwiceChangesNothing(t *testing.T) {
	n := &notes{}
	j, _ := newJournal(t, func(o *Options) { o.Notify = n.send })
	r := j.Begin(athanSpec) // never played: a failure
	r.End("first")
	r.End("second")
	if got := j.Sessions(time.Time{})[0].EndedBy; got != "first" {
		t.Errorf("ended by %q", got)
	}
	if n.count() != 1 {
		t.Errorf("notified %d times", n.count())
	}
}

func TestProblemsAreNotifiedAndGoodSessionsAreNot(t *testing.T) {
	n := &notes{}
	j, _ := newJournal(t, func(o *Options) { o.Notify = n.send })
	ok := j.Begin(athanSpec)
	playedAthan(ok)
	ok.End("x")
	if n.count() != 0 {
		t.Fatalf("a good Athan notified: %v", n.list)
	}
	bad := j.Begin(athanSpec)
	bad.End("x")
	if n.count() != 1 || !strings.Contains(n.list[0], "Never started") {
		t.Errorf("a failed Athan was not announced: %v", n.list)
	}
}

func TestNotificationsCanBeSwitchedOff(t *testing.T) {
	n := &notes{}
	j, _ := newJournal(t, func(o *Options) { o.Notify, o.NotifyOn = n.send, "off" })
	j.Begin(athanSpec).End("x")
	if n.count() != 0 {
		t.Error("notified although switched off")
	}
}

func TestOldAndOversizedSessionsArePrunedButNeverARunningOne(t *testing.T) {
	now := time.Now()
	j, dir := newJournal(t, func(o *Options) {
		o.RetentionDays, o.MaxBytes = 2, 1<<20
		o.Now = func() time.Time { return now }
	})
	old := j.Begin(Spec{Kind: "manual", Label: "Old"})
	old.End("x")
	// age it by rewriting its start time
	age(t, dir, old.ID(), now.Add(-5*24*time.Hour))

	running := j.Begin(athanSpec)
	age(t, dir, running.ID(), now.Add(-10*24*time.Hour)) // ancient but still running

	j.Prune()
	if _, err := os.Stat(filepath.Join(dir, old.ID())); err == nil {
		t.Error("a 5-day-old session survived 2-day retention")
	}
	if _, err := os.Stat(filepath.Join(dir, running.ID())); err != nil {
		t.Error("a running session was pruned")
	}
	running.End("x")
}

// noise is truly incompressible. (Hex text was not: gzip stored some of it at full
// size and halved the rest, so "the size of one session" varied by run.)
func noise(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return string(b)
}

func TestTheSizeCapDeletesTheOldestFirst(t *testing.T) {
	j, dir := newJournal(t, func(o *Options) { o.MaxBytes = 1 << 30 })
	var ids []string
	for i := 0; i < 4; i++ {
		r := j.Begin(Spec{Kind: "manual", Label: "S"})
		r.writeRaw(Target{Name: "x"}, noise(3000)) // incompressible, or gzip would hide the size
		r.End("x")
		ids = append(ids, r.ID())
		time.Sleep(1100 * time.Millisecond) // distinct start seconds
	}
	one := dirSize(filepath.Join(dir, ids[3]))
	j.Configure(0, one*5/2, "") // room for two sessions, not three
	left := 0
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(dir, id)); err == nil {
			left++
		}
	}
	if left != 2 {
		t.Fatalf("%d of 4 sessions left; a cap of 2.5 sessions should leave 2", left)
	}
	if _, err := os.Stat(filepath.Join(dir, ids[3])); err != nil {
		t.Error("the newest session was deleted instead of the oldest")
	}
}

func age(t *testing.T, dir, id string, started time.Time) {
	t.Helper()
	s, err := readSummary(filepath.Join(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	s.Started = started
	if err := writeSummary(filepath.Join(dir, id), s); err != nil {
		t.Fatal(err)
	}
}

func TestASessionLeftRunningByACrashIsClosedWhenTheAgentComesBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "journal")
	j1 := New(Options{Dir: dir, Logf: func(string, ...any) {}})
	r := j1.Begin(athanSpec)
	playedAthan(r)
	// no End: the agent "crashed"; a new process opens the same directory
	j2 := New(Options{Dir: dir, Logf: func(string, ...any) {}})
	t.Cleanup(j2.Close)
	got := j2.Sessions(time.Time{})
	if len(got) != 1 || got[0].Verdict == VerdictRunning {
		t.Fatalf("sessions = %+v", got)
	}
	if !strings.Contains(got[0].Headline, "restarted") {
		t.Errorf("the restart is not explained: %q", got[0].Headline)
	}
	if !got[0].Facts.Started {
		t.Error("what was recorded before the crash was thrown away")
	}
}

func TestRawRecordingsAreCompressedAndCanBeExportedScrubbed(t *testing.T) {
	tg := Target{Name: "Nest display", Host: "192.168.86.187", Port: 8009, Role: "speaker"}
	j, dir := newJournal(t, func(o *Options) { o.Targets = func(Spec) []Target { return []Target{tg} } })
	r := j.Begin(athanSpec)
	line := `{"dir":"in","payload":{"name":"Nest display","host":"192.168.86.187"}}`
	r.writeRaw(tg, line)
	r.End("x")

	id := r.ID()
	if _, err := os.Stat(filepath.Join(dir, id, "raw-nest-display.jsonl")); err == nil {
		t.Error("the plain raw file was left behind")
	}
	f, err := os.Open(filepath.Join(dir, id, "raw-nest-display.jsonl.gz"))
	if err != nil {
		t.Fatalf("no compressed raw file: %v", err)
	}
	zr, _ := gzip.NewReader(f)
	data, _ := io.ReadAll(zr)
	f.Close()
	if strings.TrimSpace(string(data)) != line {
		t.Errorf("compressed content = %q", data)
	}

	plain, err := j.Raw(id, "Nest display", false)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(plain)
	if !strings.Contains(string(b), "192.168.86.187") {
		t.Errorf("unscrubbed export lost the address: %s", b)
	}
	scr, _ := j.Raw(id, "Nest display", true)
	b, _ = io.ReadAll(scr)
	if strings.Contains(string(b), "192.168.86.187") || strings.Contains(string(b), "Nest display") {
		t.Errorf("scrubbed export still identifies the household: %s", b)
	}
}

func TestIDsFromOutsideCannotEscapeTheJournalDirectory(t *testing.T) {
	j, _ := newJournal(t)
	for _, bad := range []string{"../etc", "..", "a/b", "", "20261102-051800-fajr/../../x"} {
		if _, err := j.Detail(bad); err == nil {
			t.Errorf("Detail(%q) accepted", bad)
		}
		if _, err := j.Raw(bad, "x", false); err == nil {
			t.Errorf("Raw(%q) accepted", bad)
		}
	}
}

func TestAJournalThatCannotWriteNeverBreaksTheAgent(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, []byte("x"), 0o644)
	j := New(Options{Dir: filepath.Join(blocker, "journal"), Logf: func(string, ...any) {}}) // a file where a directory must be
	defer j.Close()
	r := j.Begin(athanSpec)
	playedAthan(r)
	j.Event(trace.Event{Kind: "x"})
	r.End("x") // must not panic
	if got := j.Sessions(time.Time{}); got != nil && len(got) > 0 {
		t.Logf("sessions despite no disk: %d", len(got))
	}
}

func TestConcurrentEventsAreSafe(t *testing.T) {
	j, _ := newJournal(t)
	r := j.Begin(athanSpec)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 100; k++ {
				j.Event(trace.Event{At: time.Now(), Source: trace.SourceAgent, Kind: "x", Text: "y"})
			}
		}()
	}
	wg.Wait()
	r.End("x")
	if d, _ := j.Detail(r.ID()); len(d.Events) != 800 {
		t.Errorf("stored %d of 800 events", len(d.Events))
	}
}

func TestSettingsCanBeChangedWhileRunning(t *testing.T) {
	n := &notes{}
	j, _ := newJournal(t, func(o *Options) { o.Notify, o.NotifyOn = n.send, "off" })
	j.Begin(athanSpec).End("x") // a failure, but notifications are off
	if n.count() != 0 {
		t.Fatal("notified while off")
	}
	j.Configure(1, 10<<20, "failures")
	j.Begin(athanSpec).End("x")
	if n.count() != 1 {
		t.Errorf("a changed setting did not apply: %d notifications", n.count())
	}
	if got := j.Settings(); got.RetentionDays != 1 || got.MaxBytes != 10<<20 || got.NotifyOn != "failures" {
		t.Errorf("settings = %+v", got)
	}
	j.Configure(0, 0, "") // nonsense values are ignored, not applied
	if got := j.Settings(); got.RetentionDays != 1 || got.MaxBytes != 10<<20 || got.NotifyOn != "failures" {
		t.Errorf("nonsense changed the settings: %+v", got)
	}
}

func TestAPersonCanSayWhetherTheyHeardTheAthan(t *testing.T) {
	j, _ := newJournal(t)
	r := j.Begin(athanSpec)
	playedAthan(r)
	r.End("x")
	id := r.ID()

	if err := j.AddNote(id, "no", "the speaker was silent"); err != nil {
		t.Fatal(err)
	}
	if err := j.AddNote(id, "yes", "actually, a bit later it played"); err != nil {
		t.Fatal(err)
	}
	d, _ := j.Detail(id)
	if len(d.Notes) != 2 || d.Notes[0].Heard != "no" || d.Notes[1].Text != "actually, a bit later it played" {
		t.Errorf("notes = %+v", d.Notes)
	}
	if got := j.Sessions(time.Time{})[0].Heard; got != "yes" {
		t.Errorf("the summary shows heard=%q, want the latest answer (yes)", got)
	}
}

func TestNotesAreValidated(t *testing.T) {
	j, _ := newJournal(t)
	run := j.Begin(athanSpec)
	if err := j.AddNote(run.ID(), "yes", ""); err == nil {
		t.Error("a note on a session still running was accepted")
	}
	playedAthan(run)
	run.End("x")
	for _, bad := range []struct{ id, heard, text string }{
		{"../x", "yes", ""}, {run.ID(), "maybe", ""}, {run.ID(), "yes", strings.Repeat("x", 501)}, {"20990101-000000-nope", "yes", ""},
	} {
		if err := j.AddNote(bad.id, bad.heard, bad.text); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestACapSmallerThanOneRecordingStillKeepsTheNewest(t *testing.T) {
	j, dir := newJournal(t, func(o *Options) { o.MaxBytes = 1 }) // absurdly small
	var ids []string
	for i := 0; i < 3; i++ {
		r := j.Begin(Spec{Kind: "manual", Label: "S"})
		r.writeRaw(Target{Name: "x"}, noise(2000))
		r.End("x")
		ids = append(ids, r.ID())
		time.Sleep(1100 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, ids[2])); err != nil {
		t.Error("the newest session was deleted to satisfy the size cap")
	}
	for _, id := range ids[:2] {
		if _, err := os.Stat(filepath.Join(dir, id)); err == nil {
			t.Errorf("older session %s survived a 1-byte cap", id)
		}
	}
}
