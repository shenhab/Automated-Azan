package journal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"azan-agent/internal/castrecord"
	"azan-agent/internal/trace"
)

// Options configures a Journal.
type Options struct {
	Dir           string        // where sessions are kept
	RetentionDays int           // default 30
	MaxBytes      int64         // default 300 MB
	PollEvery     time.Duration // how often devices are asked their state; default 500 ms
	Tail          time.Duration // how long to keep watching after the playback ended; default 30 s
	StartTimeout  time.Duration // how long after the request a playback may take to start; default 3 min
	NotifyOn      string        // "problems" (default), "failures" or "off"

	// Targets says which devices to watch for a session. Nil = watch none.
	Targets func(Spec) []Target
	// Health reads a device's own status page. Nil = the real one on port 8008.
	Health func(host string) (HealthSnapshot, error)
	// Notify announces a problem (a desktop notification). Nil = none.
	Notify func(title, message string)
	// AfterBegin is called once a session is open and recording, so what it emits
	// through internal/trace lands in the session. Nil = nothing.
	AfterBegin func(Spec)
	// OnChange is called when a session starts or ends (for live dashboards).
	OnChange func()
	Logf     func(format string, args ...any)
	Now      func() time.Time
}

// Journal records sessions. The zero value is not usable; call New.
type Journal struct {
	o Options

	mu       sync.Mutex
	runs     map[string]*Run
	mem      []Summary // finished sessions that could not be written to disk
	ambient  *os.File
	ambDay   string
	ambCount int
	closed   bool
}

var idRe = regexp.MustCompile(`^\d{8}-\d{6}-[a-z0-9-]{1,60}$`)

// New opens (creating if needed) the journal directory, closes any session a
// crash left running, and prunes.
func New(o Options) *Journal {
	if o.RetentionDays <= 0 {
		o.RetentionDays = 30
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 300 << 20
	}
	if o.PollEvery <= 0 {
		o.PollEvery = 500 * time.Millisecond
	}
	if o.Tail <= 0 {
		o.Tail = 30 * time.Second
	}
	if o.StartTimeout <= 0 {
		o.StartTimeout = 3 * time.Minute
	}
	if o.NotifyOn == "" {
		o.NotifyOn = "problems"
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	j := &Journal{o: o, runs: map[string]*Run{}}
	if o.Dir != "" {
		if err := os.MkdirAll(o.Dir, 0o755); err != nil {
			o.Logf("[journal] cannot create %s: %v — recording to memory only", o.Dir, err)
		}
	}
	j.recoverCrashed()
	j.Prune()
	return j
}

// Settings are the options that can change while the Journal runs.
type Settings struct {
	RetentionDays int    `json:"retention_days"`
	MaxBytes      int64  `json:"max_bytes"`
	NotifyOn      string `json:"notify_on"`
}

// Configure changes retention, the size cap and when to notify. A zero or empty
// value leaves that setting as it is.
func (j *Journal) Configure(retentionDays int, maxBytes int64, notifyOn string) {
	j.mu.Lock()
	if retentionDays > 0 {
		j.o.RetentionDays = retentionDays
	}
	if maxBytes > 0 {
		j.o.MaxBytes = maxBytes
	}
	if notifyOn == "problems" || notifyOn == "failures" || notifyOn == "off" {
		j.o.NotifyOn = notifyOn
	}
	j.mu.Unlock()
	j.Prune()
}

// Settings returns the current settings.
func (j *Journal) Settings() Settings {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Settings{RetentionDays: j.o.RetentionDays, MaxBytes: j.o.MaxBytes, NotifyOn: j.o.NotifyOn}
}

// Close ends every open session.
func (j *Journal) Close() {
	j.mu.Lock()
	j.closed = true
	runs := make([]*Run, 0, len(j.runs))
	for _, r := range j.runs {
		runs = append(runs, r)
	}
	if j.ambient != nil {
		j.ambient.Close()
		j.ambient = nil
	}
	j.mu.Unlock()
	for _, r := range runs {
		r.End("the agent shut down")
	}
}

// Event implements trace.Sink: agent events go to every open session, or to the
// day's ambient log when none is open.
func (j *Journal) Event(e trace.Event) {
	if e.At.IsZero() {
		e.At = j.o.Now()
	}
	j.mu.Lock()
	runs := make([]*Run, 0, len(j.runs))
	for _, r := range j.runs {
		runs = append(runs, r)
	}
	j.mu.Unlock()
	if len(runs) == 0 {
		j.ambientEvent(e)
		return
	}
	for _, r := range runs {
		r.add(e)
	}
}

const ambientMaxPerDay = 5000

func (j *Journal) ambientEvent(e trace.Event) {
	if j.o.Dir == "" {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	day := e.At.Format("2006-01-02")
	if j.ambDay != day || j.ambient == nil {
		if j.ambient != nil {
			j.ambient.Close()
		}
		f, err := os.OpenFile(filepath.Join(j.o.Dir, "ambient-"+day+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			j.ambient = nil
			return
		}
		j.ambient, j.ambDay, j.ambCount = f, day, 0
	}
	if j.ambCount >= ambientMaxPerDay {
		return
	}
	j.ambCount++
	b, _ := json.Marshal(e)
	j.ambient.Write(append(b, '\n'))
}

// Run is one open session.
type Run struct {
	j       *Journal
	id      string
	dir     string // "" when the disk is unusable
	spec    Spec
	started time.Time
	targets []Target

	mu     sync.Mutex
	events []trace.Event
	evFile *os.File
	raw    map[string]*rawFile
	attr   *attribution
	ended  bool // End has begun (makes it idempotent)
	closed bool // the files are final: nothing more is accepted

	// What the session has seen, to tell the agent's doing from an outsider's.
	loadAt       time.Time // the agent's first request to play
	playing      bool      // the speaker reported playback after that request
	playEndAt    time.Time // when that playback ended
	tvHeld       bool
	tvReleasedAt time.Time
	live         map[string]*DeviceLive
	endedBy      string

	stop chan struct{}
	wg   sync.WaitGroup
	hs   *healthSession

	// Who really played: the session the speaker's app is in, the one each member
	// shows, and the members already recorded as playing. Guarded by mu.
	speakerSession string
	memberSession  map[string]string
	memberPlayed   map[string]bool
}

type rawFile struct {
	f *os.File
	w *bufio.Writer
}

const maxEventsInMemory = 50000

// ID is the session's identifier (also its directory name).
func (r *Run) ID() string { return r.id }

// Begin opens a session. It never fails: if the disk is unusable the session is
// kept in memory.
func (j *Journal) Begin(spec Spec) *Run {
	now := j.o.Now()
	r := &Run{j: j, spec: spec, started: now, raw: map[string]*rawFile{}, attr: newAttribution(), stop: make(chan struct{}), live: map[string]*DeviceLive{},
		memberSession: map[string]string{}, memberPlayed: map[string]bool{}}
	if j.o.Targets != nil {
		r.targets = j.o.Targets(spec)
	}

	j.mu.Lock()
	base := now.Format("20060102-150405") + "-" + slug(spec.Label)
	id := base
	for n := 2; ; n++ {
		_, taken := j.runs[id]
		if !taken && j.o.Dir != "" {
			if _, err := os.Stat(filepath.Join(j.o.Dir, id)); err == nil {
				taken = true
			}
		}
		if !taken {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
	r.id = id
	j.runs[id] = r
	j.mu.Unlock()

	if j.o.Dir != "" {
		dir := filepath.Join(j.o.Dir, id)
		if err := os.MkdirAll(dir, 0o755); err == nil {
			r.dir = dir
			if f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				r.evFile = f
			}
			writeSummary(dir, r.summary(Result{Verdict: VerdictRunning, Headline: "In progress…"}, time.Time{}, ""))
		}
	}
	j.o.Logf("[journal] recording %s (%s)", spec.Label, id)
	r.start()
	if j.o.AfterBegin != nil {
		j.o.AfterBegin(spec)
	}
	if j.o.OnChange != nil {
		j.o.OnChange()
	}
	return r
}

func (r *Run) summary(res Result, ended time.Time, by string) Summary {
	r.mu.Lock()
	n := len(r.events)
	r.mu.Unlock()
	return Summary{ID: r.id, Spec: r.spec, Started: r.started, Ended: ended, EndedBy: by,
		Verdict: res.Verdict, Headline: res.Headline, Facts: res.Facts, Targets: r.targets, Events: n}
}

// add records one event in the session.
func (r *Run) add(e trace.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if len(r.events) < maxEventsInMemory {
		r.events = append(r.events, e)
	}
	r.attr.Add(e)
	if e.Source == trace.SourceAgent && e.Kind == "load-sent" && r.loadAt.IsZero() {
		r.loadAt = e.At
	}
	if e.Source == trace.SourceTV {
		switch e.Kind {
		case "tv-held":
			r.tvHeld = true
		case "tv-released":
			r.tvHeld, r.tvReleasedAt = false, e.At
		}
	}
	if r.evFile != nil {
		if b, err := json.Marshal(e); err == nil {
			r.evFile.Write(append(b, '\n'))
		}
	}
}

// writeRaw appends one raw Cast frame (a JSON line) to the device's raw file.
func (r *Run) writeRaw(t Target, line string) {
	if r.dir == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	key := slug(t.Name)
	rf := r.raw[key]
	if rf == nil {
		f, err := os.OpenFile(filepath.Join(r.dir, "raw-"+key+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		rf = &rawFile{f: f, w: bufio.NewWriterSize(f, 64<<10)}
		r.raw[key] = rf
	}
	rf.w.WriteString(line)
	rf.w.WriteByte('\n')
	rf.w.Flush()
}

// End closes the session and judges it. It is safe to call more than once.
func (r *Run) End(by string) {
	r.mu.Lock()
	if r.ended {
		r.mu.Unlock()
		return
	}
	r.ended, r.endedBy = true, by
	r.mu.Unlock()

	close(r.stop)
	r.wg.Wait()
	issues := r.finishHealth()

	r.mu.Lock()
	events := append([]trace.Event(nil), r.events...)
	for _, rf := range r.raw {
		rf.w.Flush()
		rf.f.Close()
	}
	if r.evFile != nil {
		r.evFile.Close()
	}
	r.closed = true
	r.mu.Unlock()

	res := Analyze(r.spec, events, issues, true)
	now := r.j.o.Now()
	sum := r.summary(res, now, by)
	if r.dir != "" {
		r.compressRaw()
		sum.Bytes = dirSize(r.dir)
		if err := writeSummary(r.dir, sum); err != nil {
			r.j.o.Logf("[journal] cannot save %s: %v", r.id, err)
		}
	}

	r.j.mu.Lock()
	delete(r.j.runs, r.id)
	if r.dir == "" {
		r.j.mem = append(r.j.mem, sum)
		if len(r.j.mem) > 200 {
			r.j.mem = r.j.mem[1:]
		}
	}
	r.j.mu.Unlock()

	tag := map[string]string{VerdictOK: "OK", VerdictWarning: "WARNING", VerdictFailed: "FAILED"}[res.Verdict]
	r.j.o.Logf("[journal] %s: %s — %s", r.spec.Label, tag, res.Headline)
	notifyOn := r.j.Settings().NotifyOn
	if r.j.o.Notify != nil && r.spec.ExpectPlayback {
		if (res.Verdict == VerdictFailed && notifyOn != "off") ||
			(res.Verdict == VerdictWarning && notifyOn == "problems") {
			r.j.o.Notify("Automated Azan — "+r.spec.Label, res.Headline)
		}
	}
	r.j.Prune()
	if r.j.o.OnChange != nil {
		r.j.o.OnChange()
	}
}

func (r *Run) compressRaw() {
	files, _ := filepath.Glob(filepath.Join(r.dir, "raw-*.jsonl"))
	for _, p := range files {
		in, err := os.Open(p)
		if err != nil {
			continue
		}
		out, err := os.Create(p + ".gz")
		if err != nil {
			in.Close()
			continue
		}
		zw := gzip.NewWriter(out)
		_, cerr := io.Copy(zw, in)
		zerr := zw.Close()
		oerr := out.Close()
		in.Close()
		if cerr == nil && zerr == nil && oerr == nil {
			os.Remove(p)
		} else {
			os.Remove(p + ".gz")
		}
	}
}

// --- reading back ---

// Sessions lists sessions that started at or after since, newest first.
func (j *Journal) Sessions(since time.Time) []Summary {
	var out []Summary
	if j.o.Dir != "" {
		entries, _ := os.ReadDir(j.o.Dir)
		for _, e := range entries {
			if !e.IsDir() || !idRe.MatchString(e.Name()) {
				continue
			}
			s, err := readSummary(filepath.Join(j.o.Dir, e.Name()))
			if err != nil || s.Started.Before(since) {
				continue
			}
			out = append(out, s)
		}
	}
	j.mu.Lock()
	for _, s := range j.mem {
		if !s.Started.Before(since) {
			out = append(out, s)
		}
	}
	j.mu.Unlock()
	sort.Slice(out, func(a, b int) bool { return out[a].Started.After(out[b].Started) })
	return out
}

// Detail is a session with its timeline.
type Detail struct {
	Summary Summary          `json:"summary"`
	Events  []trace.Event    `json:"events"`
	Raw     []RawInfo        `json:"raw"`
	Health  []HealthSnapshot `json:"health,omitempty"`
	Notes   []Note           `json:"notes,omitempty"`
}

// RawInfo describes a downloadable raw recording.
type RawInfo struct {
	Device string `json:"device"`
	Role   string `json:"role"`
	Bytes  int64  `json:"bytes"`
}

// Detail returns one session. While it is still running the live state is returned.
func (j *Journal) Detail(id string) (Detail, error) {
	if !idRe.MatchString(id) {
		return Detail{}, fmt.Errorf("bad session id")
	}
	j.mu.Lock()
	run := j.runs[id]
	j.mu.Unlock()
	if run != nil {
		run.mu.Lock()
		evs := append([]trace.Event(nil), run.events...)
		run.mu.Unlock()
		res := Analyze(run.spec, evs, nil, false)
		return Detail{Summary: run.summary(res, time.Time{}, ""), Events: sortEvents(evs)}, nil
	}
	dir := filepath.Join(j.o.Dir, id)
	s, err := readSummary(dir)
	if err != nil {
		for _, m := range j.memCopy() {
			if m.ID == id {
				return Detail{Summary: m}, nil
			}
		}
		return Detail{}, fmt.Errorf("no such session")
	}
	d := Detail{Summary: s, Events: sortEvents(readEvents(filepath.Join(dir, "events.jsonl")))}
	for _, t := range s.Targets {
		for _, ext := range []string{".jsonl.gz", ".jsonl"} {
			if st, err := os.Stat(filepath.Join(dir, "raw-"+slug(t.Name)+ext)); err == nil {
				d.Raw = append(d.Raw, RawInfo{Device: t.Name, Role: t.Role, Bytes: st.Size()})
				break
			}
		}
	}
	d.Health = readHealthFile(dir)
	d.Notes = readNotes(dir)
	return d, nil
}

func (j *Journal) memCopy() []Summary {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]Summary(nil), j.mem...)
}

// Raw returns a device's raw recording, optionally scrubbed of addresses, ids and
// names so it can be shared.
func (j *Journal) Raw(id, device string, scrub bool) (io.ReadCloser, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("bad session id")
	}
	dir := filepath.Join(j.o.Dir, id)
	s, err := readSummary(dir)
	if err != nil {
		return nil, fmt.Errorf("no such session")
	}
	var data []byte
	found := false
	for _, t := range s.Targets {
		if !strings.EqualFold(t.Name, device) {
			continue
		}
		base := filepath.Join(dir, "raw-"+slug(t.Name)+".jsonl")
		if f, err := os.Open(base + ".gz"); err == nil {
			zr, zerr := gzip.NewReader(f)
			if zerr == nil {
				data, _ = io.ReadAll(zr)
				found = true
			}
			f.Close()
		} else if b, err := os.ReadFile(base); err == nil {
			data, found = b, true
		}
		break
	}
	if !found {
		return nil, fmt.Errorf("no raw recording for %q", device)
	}
	if scrub {
		names := map[string]string{}
		for i, t := range s.Targets {
			names[t.Name] = fmt.Sprintf("Device %d", i+1)
		}
		var sb strings.Builder
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			sb.WriteString(castrecord.Scrub(line, names))
			sb.WriteByte('\n')
		}
		data = []byte(sb.String())
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// --- storage helpers ---

func writeSummary(dir string, s Summary) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "session.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "session.json"))
}

func readSummary(dir string) (Summary, error) {
	var s Summary
	b, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

func readEvents(path string) []trace.Event {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []trace.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e trace.Event
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func sortEvents(evs []trace.Event) []trace.Event {
	sort.SliceStable(evs, func(i, k int) bool { return evs[i].At.Before(evs[k].At) })
	return evs
}

func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	dash := false
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "session"
	}
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	return out
}

func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// recoverCrashed closes sessions left "running" by a crash or restart, keeping
// what was recorded.
func (j *Journal) recoverCrashed() {
	if j.o.Dir == "" {
		return
	}
	entries, _ := os.ReadDir(j.o.Dir)
	for _, e := range entries {
		if !e.IsDir() || !idRe.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(j.o.Dir, e.Name())
		s, err := readSummary(dir)
		if err != nil || s.Verdict != VerdictRunning {
			continue
		}
		evs := sortEvents(readEvents(filepath.Join(dir, "events.jsonl")))
		res := Analyze(s.Spec, evs, nil, true)
		s.Verdict = VerdictWarning
		s.Headline = "The agent restarted before this recording ended — " + res.Headline
		s.Facts, s.Events, s.EndedBy = res.Facts, len(evs), "the agent restarted"
		if len(evs) > 0 {
			s.Ended = evs[len(evs)-1].At
		}
		// finish any raw files the crash left uncompressed
		(&Run{dir: dir}).compressRaw()
		s.Bytes = dirSize(dir)
		writeSummary(dir, s)
	}
}

// Prune deletes sessions older than the retention period, then the oldest ones
// until the journal fits MaxBytes. A running session is never deleted.
func (j *Journal) Prune() {
	if j.o.Dir == "" {
		return
	}
	entries, err := os.ReadDir(j.o.Dir)
	if err != nil {
		return
	}
	set := j.Settings()
	cutoff := j.o.Now().Add(-time.Duration(set.RetentionDays) * 24 * time.Hour)
	type item struct {
		id      string
		started time.Time
		size    int64
	}
	var keep []item
	var total int64
	j.mu.Lock()
	open := map[string]bool{}
	for id := range j.runs {
		open[id] = true
	}
	j.mu.Unlock()
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(j.o.Dir, name)
		if strings.HasPrefix(name, "ambient-") && strings.HasSuffix(name, ".jsonl") {
			if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
				os.Remove(path)
			}
			continue
		}
		if !e.IsDir() || !idRe.MatchString(name) || open[name] {
			continue
		}
		s, err := readSummary(path)
		if err != nil {
			continue
		}
		if s.Started.Before(cutoff) {
			os.RemoveAll(path)
			continue
		}
		it := item{id: name, started: s.Started, size: dirSize(path)}
		total += it.size
		keep = append(keep, it)
	}
	sort.Slice(keep, func(a, b int) bool { return keep[a].started.Before(keep[b].started) })
	// The newest finished session is never deleted for size: a cap smaller than one
	// recording must not empty the journal.
	for i := 0; i < len(keep)-1; i++ {
		if total <= set.MaxBytes {
			break
		}
		os.RemoveAll(filepath.Join(j.o.Dir, keep[i].id))
		total -= keep[i].size
	}
}

// Live returns the sessions in progress and what their devices are doing now.
func (j *Journal) Live() []LiveRun {
	j.mu.Lock()
	runs := make([]*Run, 0, len(j.runs))
	for _, r := range j.runs {
		runs = append(runs, r)
	}
	j.mu.Unlock()
	var out []LiveRun
	for _, r := range runs {
		r.mu.Lock()
		lr := LiveRun{ID: r.id, Label: r.spec.Label, Kind: r.spec.Kind, Started: r.started}
		evs := append([]trace.Event(nil), r.events...)
		for _, t := range r.targets {
			if d := r.live[t.Name]; d != nil {
				lr.Devices = append(lr.Devices, *d)
			} else {
				lr.Devices = append(lr.Devices, DeviceLive{Name: t.Name, Role: t.Role})
			}
		}
		r.mu.Unlock()
		lr.Headline = Analyze(r.spec, evs, nil, false).Headline
		out = append(out, lr)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Started.Before(out[b].Started) })
	return out
}

// updateLive folds a device's report into the live view. Call with r.mu held.
func (r *Run) updateLive(t Target, kind string, f map[string]any) {
	d := r.live[t.Name]
	if d == nil {
		d = &DeviceLive{Name: t.Name, Role: t.Role}
		r.live[t.Name] = d
	}
	switch kind {
	case "initial-state", "reconnected":
		d.Reachable = true
	case "unreachable":
		d.Reachable = false
	}
	if v, ok := f["app"].(string); ok && (kind == "initial-state" || kind == "app-launched") {
		d.App = v
	}
	switch kind {
	case "app-closed":
		d.App, d.State = "", ""
	case "player-state":
		d.State, _ = f["state"].(string)
	case "initial-state", "volume":
		d.Level, _ = f["level"].(float64)
		d.Muted, _ = f["muted"].(bool)
	}
}

const maxNoteText = 500

// AddNote records what a person says about a finished session: whether they
// heard it, and anything else worth remembering. Only a person can know whether
// sound really came out of the room.
func (j *Journal) AddNote(id, heard, text string) error {
	if !idRe.MatchString(id) {
		return fmt.Errorf("bad session id")
	}
	if heard != "" && heard != "yes" && heard != "no" && heard != "partly" {
		return fmt.Errorf("heard must be yes, no or partly")
	}
	if len(text) > maxNoteText {
		return fmt.Errorf("the note is too long (%d characters, at most %d)", len(text), maxNoteText)
	}
	j.mu.Lock()
	_, running := j.runs[id]
	j.mu.Unlock()
	if running {
		return fmt.Errorf("that session is still being recorded")
	}
	dir := filepath.Join(j.o.Dir, id)
	s, err := readSummary(dir)
	if err != nil {
		return fmt.Errorf("no such session")
	}
	notes := append(readNotes(dir), Note{At: j.o.Now(), Heard: heard, Text: text})
	b, _ := json.MarshalIndent(notes, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "notes.json"), b, 0o644); err != nil {
		return err
	}
	if heard != "" {
		s.Heard = heard
		return writeSummary(dir, s)
	}
	return nil
}

func readNotes(dir string) []Note {
	b, err := os.ReadFile(filepath.Join(dir, "notes.json"))
	if err != nil {
		return nil
	}
	var out []Note
	json.Unmarshal(b, &out)
	return out
}
