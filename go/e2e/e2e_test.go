//go:build e2e

// Package e2e runs the real agent against a fake Google Cast speaker and
// verifies that a scheduled prayer actually results in the right audio being
// played on the speaker at the right time.
//
//	go test -tags e2e ./e2e/ -v            # builds and runs the agent binary
//	E2E_DOCKER_IMAGE=shenhab/athan:dev \
//	    go test -tags e2e ./e2e/ -v        # runs that Docker image instead
//
// Nothing here touches real speakers or the network: timetables are fixtures
// written into the agent's data directory, and the speaker is addressed via
// AZAN_STATIC_DEVICES instead of mDNS.
package e2e

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"azan-agent/internal/fakecast"
)

const speakerName = "Test Speaker"

var dublin = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		panic(err)
	}
	return loc
}()

// ── building / launching the agent ───────────────────────────────────────────

var (
	buildMu  sync.Mutex
	builtBin = map[string]string{} // build tags -> binary path
)

// agentBinary compiles the headless (CGO_ENABLED=0) agent — the same way the
// Docker image is built — once per set of build tags per test run. The
// "azanfaketime" tag is the only difference from the shipped build: it lets a test
// start the agent's clock at a chosen instant (see internal/clock).
func agentBinary(t *testing.T, tags string) string {
	t.Helper()
	buildMu.Lock()
	defer buildMu.Unlock()
	if p, ok := builtBin[tags]; ok {
		return p
	}
	dir, err := os.MkdirTemp("", "azan-e2e-bin")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "azan-agent")
	args := []string{"build", "-o", out}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	cmd := exec.Command("go", append(args, "./cmd/azan-agent")...)
	cmd.Dir = ".." // module root (the go/ directory)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cannot build agent: %v\n%s", err, b)
	}
	builtBin[tags] = out
	return out
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type agent struct {
	logs    *syncBuf
	webPort int
}

var (
	portMu   sync.Mutex
	portUsed = map[int]bool{}
)

// freePorts returns a web port p such that p and p+1 (the media server) are
// free, and that no other test in this run has been given. (Tests run in
// parallel, and a port is only "free" until somebody binds it.)
func freePorts(t *testing.T) int {
	t.Helper()
	portMu.Lock()
	defer portMu.Unlock()
	// Ports come from 20000-29999, below the ephemeral range (32768+ on Linux), so
	// no outbound connection on the machine can take one between the moment it is
	// picked and the moment the agent binds it a few seconds later.
	for i := 0; i < 400; i++ {
		p := 20000 + rand.Intn(10000)
		if portUsed[p] || portUsed[p+1] || portUsed[p-1] {
			continue
		}
		l1, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		l1.Close()
		l2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+1))
		if err != nil {
			continue
		}
		l2.Close()
		portUsed[p], portUsed[p+1] = true, true
		return p
	}
	t.Fatal("no free port pair")
	return 0
}

var containerSeq atomic.Int64

// containerName returns a Docker container name no other test in this run uses.
func containerName() string {
	return fmt.Sprintf("azan-e2e-%d-%d", os.Getpid(), containerSeq.Add(1))
}

// startAgent launches the agent (binary, or Docker image if E2E_DOCKER_IMAGE is
// set) with the given home directory, config file and speaker, and stops it when
// the test ends.
type agentOpt func(*agentCfg)

type agentCfg struct {
	fakeNow string
	env     []string
	extra   []string // additional static devices, "Name=host:port"
}

// withFakeNow starts the agent's clock at the given RFC 3339 instant. It needs
// the "azanfaketime" build, so it only works in binary mode (the Docker image is the
// shipped build and has no fake clock).
// withCredentials seeds the web login (the agent does this at first run from
// AZAN_USERNAME / AZAN_PASSWORD).
func withCredentials(user, pass string) agentOpt {
	return func(c *agentCfg) { c.env = append(c.env, "AZAN_USERNAME="+user, "AZAN_PASSWORD="+pass) }
}

// withExtraDevice makes another fake device (e.g. a TV) visible to the agent.
func withExtraDevice(name string, sp *fakecast.Speaker) agentOpt {
	return func(c *agentCfg) {
		h, p := sp.Addr()
		c.extra = append(c.extra, fmt.Sprintf("%s=%s:%d", name, h, p))
	}
}

// withEnv passes extra environment variables ("KEY=value") to the agent.
func withEnv(kv ...string) agentOpt { return func(c *agentCfg) { c.env = append(c.env, kv...) } }

func withFakeNow(rfc3339 string) agentOpt { return func(c *agentCfg) { c.fakeNow = rfc3339 } }

func startAgent(t *testing.T, home, cfgFile string, sp *fakecast.Speaker, opts ...agentOpt) *agent {
	t.Helper()
	var ac agentCfg
	for _, o := range opts {
		o(&ac)
	}
	host, port := sp.Addr()
	static := strings.Join(append([]string{fmt.Sprintf("%s=%s:%d", speakerName, host, port)}, ac.extra...), ";")
	logs := &syncBuf{}

	var cmd *exec.Cmd
	var stop func()
	if image := os.Getenv("E2E_DOCKER_IMAGE"); image != "" {
		if ac.fakeNow != "" {
			t.Skip("needs the azanfaketime build; run in binary mode (unset E2E_DOCKER_IMAGE)")
		}
		u, _ := user.Current()
		name := containerName()
		cmd = exec.Command("docker", "run", "--rm", "--name", name,
			"--network", "host", "--user", u.Uid+":"+u.Gid,
			"-e", "HOME=/home/e2e",
			"-e", "AZAN_CONFIG_FILE=/cfg/azan.toml",
			"-e", "AZAN_STATIC_DEVICES="+static,
			"-e", "AZAN_DISABLE_MDNS=1", // never touch real devices on the LAN
			"-v", home+":/home/e2e",
			"-v", filepath.Dir(cfgFile)+":/cfg")
		for _, kv := range ac.env {
			cmd.Args = append(cmd.Args, "-e", kv)
		}
		cmd.Args = append(cmd.Args, image)
		stop = func() { exec.Command("docker", "stop", "-t", "3", name).Run() }
	} else {
		tags := ""
		if ac.fakeNow != "" {
			tags = "azanfaketime"
		}
		cmd = exec.Command(agentBinary(t, tags))
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			"AZAN_CONFIG_FILE="+cfgFile,
			"AZAN_STATIC_DEVICES="+static,
			"AZAN_DISABLE_MDNS=1", // never touch real devices on the LAN
		)
		if ac.fakeNow != "" {
			cmd.Env = append(cmd.Env, "AZAN_FAKE_NOW="+ac.fakeNow)
		}
		cmd.Env = append(cmd.Env, ac.env...)
		stop = func() { cmd.Process.Signal(syscall.SIGTERM) }
	}
	cmd.Stdout, cmd.Stderr = logs, logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		stop()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			cmd.Process.Kill()
			<-done
		}
		if t.Failed() {
			t.Logf("──── agent log ────\n%s", logs.String())
		}
	})
	return &agent{logs: logs}
}

// ── fixtures ─────────────────────────────────────────────────────────────────

// writeNaasTimetable writes a full-year Naas-format timetable (the format
// downloaded from Mawaqit) into the agent's data directory. Every day has the
// same times: a realistic Dhuhr (so the DST sanity check leaves it alone) and
// `trigger` set to fireAt. Other prayers are disabled in the config.
func writeNaasTimetable(t *testing.T, home string, fireAt time.Time, trigger string) {
	t.Helper()
	// Dhuhr ≈ solar noon on the Dublin wall clock (12:25 GMT / 13:25 IST).
	_, off := fireAt.In(dublin).Zone()
	noon := fmt.Sprintf("%02d:25", 12+off/3600)
	idx := map[string]int{"Fajr": 0, "Dhuhr": 2, "Asr": 3, "Maghrib": 4, "Isha": 5}
	times := []string{"03:00", "05:00", noon, "15:00", "18:00", "20:00"}
	times[idx[trigger]] = fireAt.In(dublin).Format("15:04")

	var months []string
	for m := 0; m < 12; m++ {
		var days []string
		for d := 1; d <= 31; d++ {
			days = append(days, fmt.Sprintf("%q:[%s]", fmt.Sprint(d), quoteJoin(times)))
		}
		months = append(months, "{"+strings.Join(days, ",")+"}")
	}
	dir := filepath.Join(home, ".local", "share", "azan-agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "[" + strings.Join(months, ",") + "]"
	if err := os.WriteFile(filepath.Join(dir, "naas_prayers_timetable.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quoteJoin(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ",")
}

func writeConfig(t *testing.T, path string, webPort int, enabledPrayer string, extra ...string) {
	t.Helper()
	writeConfigAt(t, path, webPort, "naas", enabledPrayer, extra...)
}

func writeConfigAt(t *testing.T, path string, webPort int, location, enabledPrayer string, extra ...string) {
	t.Helper()
	flag := func(p string) string {
		if enabledPrayer == "all" || strings.EqualFold(p, enabledPrayer) {
			return "true"
		}
		return "false"
	}
	cfg := fmt.Sprintf(`[speaker]
group_name = %q

[prayer]
location = %q

[prayer.enabled]
fajr = %s
dhuhr = %s
asr = %s
maghrib = %s
isha = %s

[web]
host = "127.0.0.1"
port = %d
`, speakerName, location, flag("Fajr"), flag("Dhuhr"), flag("Asr"), flag("Maghrib"), flag("Isha"), webPort)
	cfg += strings.Join(extra, "\n")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

// nextFireTime returns the Dublin-wall-clock minute (>= minLead from now) at
// which the prayer should fire, avoiding the midnight rollover.
func nextFireTime(minLead time.Duration) time.Time {
	for {
		now := time.Now().In(dublin)
		fire := now.Add(minLead).Truncate(time.Minute).Add(time.Minute)
		if fire.Day() == now.Day() {
			return fire
		}
		time.Sleep(time.Until(fire.Truncate(24*time.Hour)) + 5*time.Second)
	}
}

// ── the scenarios ────────────────────────────────────────────────────────────

func TestScheduledAthanReachesSpeaker(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		prayer   string
		wantFile string
	}{
		// A regular prayer plays the standard Athan. (Asr, not Isha: Isha must come
		// after Maghrib, and an Isha earlier than Maghrib now means "after midnight".)
		{"Asr", "media_Athan.mp3"},
		{"Fajr", "media_adhan_al_fajr.mp3"}, // Fajr has its own Athan
	} {
		tc := tc
		t.Run(tc.prayer, func(t *testing.T) {
			t.Parallel()

			sp := fakecast.New(speakerName)
			if err := sp.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(sp.Close)

			home := t.TempDir()
			cfgDir := t.TempDir()
			cfgFile := filepath.Join(cfgDir, "azan.toml")
			webPort := freePorts(t)

			fireAt := nextFireTime(100 * time.Second)
			writeNaasTimetable(t, home, fireAt, tc.prayer)
			// Also notify connected browsers, so the websocket event can be checked.
			writeConfig(t, cfgFile, webPort, tc.prayer,
				fmt.Sprintf("\n[prayer.channels.%s]\nspeaker = true\nbrowser_notify = true\nbrowser_athan = true\n", strings.ToLower(tc.prayer)))

			ag := startAgent(t, home, cfgFile, sp, withCredentials(testUser, testPass))
			events := watchWebsocket(t, webPort)

			// Wait for the Athan to arrive at the speaker.
			wait := time.Until(fireAt) + 90*time.Second
			loads, err := sp.WaitForLoads(1, wait)
			if err != nil {
				t.Fatalf("%s Athan never reached the speaker: %v", tc.prayer, err)
			}
			l := loads[0]

			// 1. Right time.
			if early := fireAt.Sub(l.At); early > 3*time.Second {
				t.Errorf("played %v too early (scheduled %s)", early, fireAt.Format("15:04:05"))
			}
			if late := l.At.Sub(fireAt); late > 20*time.Second {
				t.Errorf("played %v late (scheduled %s)", late, fireAt.Format("15:04:05"))
			}

			// 2. Right audio, at a URL the speaker could actually reach.
			wantSuffix := fmt.Sprintf(":%d/media/%s", webPort+1, tc.wantFile)
			if !strings.HasPrefix(l.URL, "http://") || !strings.HasSuffix(l.URL, wantSuffix) {
				t.Errorf("media URL = %q, want http://<host>%s", l.URL, wantSuffix)
			}
			if l.ContentType != "audio/mpeg" {
				t.Errorf("content type = %q, want audio/mpeg", l.ContentType)
			}
			f := l.Fetch
			if f.Status != 200 || f.Err != "" {
				t.Errorf("speaker could not download the audio: status %d err %q", f.Status, f.Err)
			}
			if f.ContentType != "audio/mpeg" || !f.LooksLikeAudio || f.Bytes < 50_000 {
				t.Errorf("downloaded file is not a plausible MP3: %+v", f)
			}

			// 3. A browser connected to the dashboard is told, with the audio to play.
			select {
			case ev := <-events:
				if ev.Event != "prayer_fired" || ev.Data.Prayer != tc.prayer || ev.Data.AudioURL != "/media/"+tc.wantFile ||
					!ev.Data.BrowserNotify || !ev.Data.BrowserAthan {
					t.Errorf("websocket event = %+v, want prayer_fired for %s with /media/%s", ev, tc.prayer, tc.wantFile)
				}
			case <-time.After(10 * time.Second):
				t.Errorf("a connected browser never received the prayer_fired event")
			}

			// 3b. The agent checked, by itself, that the speaker really started playing.
			api := newClient(t, webPort)
			api.mustLogin()
			var status struct {
				Last struct {
					Started bool   `json:"started"`
					State   string `json:"state"`
					Speaker string `json:"speaker"`
				} `json:"last_playback"`
			}
			verifyBy := time.Now().Add(15 * time.Second)
			for !status.Last.Started && time.Now().Before(verifyBy) {
				api.getJSON("/api/scheduler-status", &status)
				time.Sleep(500 * time.Millisecond)
			}
			if !status.Last.Started || status.Last.State != "PLAYING" || status.Last.Speaker != speakerName {
				t.Errorf("the agent never recorded verified playback after the scheduled Athan: %+v", status.Last)
			}

			// 4. Played once, not repeatedly.
			time.Sleep(10 * time.Second)
			if n := len(sp.Loads()); n != 1 {
				t.Errorf("speaker received %d LOAD commands, want exactly 1", n)
			}

			// 5. The agent stayed healthy.
			if out := ag.logs.String(); strings.Contains(out, "panic:") {
				t.Errorf("agent panicked")
			}
		})
	}
}

type wsEvent struct {
	Event string `json:"event"`
	Data  struct {
		Prayer        string `json:"prayer"`
		BrowserNotify bool   `json:"browser_notify"`
		BrowserAthan  bool   `json:"browser_athan"`
		AudioURL      string `json:"audio_url"`
	} `json:"data"`
}

// watchWebsocket logs in like a browser, opens the dashboard websocket and
// returns a channel of the prayer_fired events it receives.
func watchWebsocket(t *testing.T, webPort int) <-chan wsEvent {
	t.Helper()
	c := newClient(t, webPort)
	c.mustLogin()
	u, _ := url.Parse(c.base)
	var cookies []string
	for _, ck := range c.http.Jar.Cookies(u) {
		cookies = append(cookies, ck.Name+"="+ck.Value)
	}
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", webPort), http.Header{"Cookie": {strings.Join(cookies, "; ")}})
	if err != nil {
		t.Fatalf("websocket: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	out := make(chan wsEvent, 8)
	go func() {
		for {
			var ev wsEvent
			if err := conn.ReadJSON(&ev); err != nil {
				return
			}
			if ev.Event == "prayer_fired" {
				out <- ev
			}
		}
	}()
	return out
}
