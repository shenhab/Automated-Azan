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
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

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
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// agentBinary compiles the headless (CGO_ENABLED=0) agent — the same way the
// Docker image is built — once per test run.
func agentBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "azan-e2e-bin")
		if err != nil {
			buildErr = err
			return
		}
		binPath = filepath.Join(dir, "azan-agent")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/azan-agent")
		cmd.Dir = ".." // module root (the go/ directory)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatalf("cannot build agent: %v", buildErr)
	}
	return binPath
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
	logs *syncBuf
}

// freePorts returns a web port p such that p and p+1 (the media server) are free.
func freePorts(t *testing.T) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		l1, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		p := l1.Addr().(*net.TCPAddr).Port
		l2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+1))
		l1.Close()
		if err != nil {
			continue
		}
		l2.Close()
		return p
	}
	t.Fatal("no free port pair")
	return 0
}

// startAgent launches the agent (binary, or Docker image if E2E_DOCKER_IMAGE is
// set) with the given home directory, config file and speaker, and stops it when
// the test ends.
func startAgent(t *testing.T, home, cfgFile string, sp *fakecast.Speaker) *agent {
	t.Helper()
	host, port := sp.Addr()
	static := fmt.Sprintf("%s=%s:%d", speakerName, host, port)
	logs := &syncBuf{}

	var cmd *exec.Cmd
	var stop func()
	if image := os.Getenv("E2E_DOCKER_IMAGE"); image != "" {
		u, _ := user.Current()
		name := fmt.Sprintf("azan-e2e-%d-%d", os.Getpid(), time.Now().UnixNano()%100000)
		cmd = exec.Command("docker", "run", "--rm", "--name", name,
			"--network", "host", "--user", u.Uid+":"+u.Gid,
			"-e", "HOME=/home/e2e",
			"-e", "AZAN_CONFIG_FILE=/cfg/azan.toml",
			"-e", "AZAN_STATIC_DEVICES="+static,
			"-v", home+":/home/e2e",
			"-v", filepath.Dir(cfgFile)+":/cfg",
			image)
		stop = func() { exec.Command("docker", "stop", "-t", "3", name).Run() }
	} else {
		cmd = exec.Command(agentBinary(t))
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			"AZAN_CONFIG_FILE="+cfgFile,
			"AZAN_STATIC_DEVICES="+static,
		)
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

func writeConfig(t *testing.T, path string, webPort int, enabledPrayer string) {
	t.Helper()
	flag := func(p string) string {
		if strings.EqualFold(p, enabledPrayer) {
			return "true"
		}
		return "false"
	}
	cfg := fmt.Sprintf(`[speaker]
group_name = %q

[prayer]
location = "naas"

[prayer.enabled]
fajr = %s
dhuhr = %s
asr = %s
maghrib = %s
isha = %s

[web]
host = "127.0.0.1"
port = %d
`, speakerName, flag("Fajr"), flag("Dhuhr"), flag("Asr"), flag("Maghrib"), flag("Isha"), webPort)
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
	for _, tc := range []struct {
		prayer   string
		wantFile string
	}{
		{"Isha", "media_Athan.mp3"},         // regular prayers use the standard Athan
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
			writeConfig(t, cfgFile, webPort, tc.prayer)

			ag := startAgent(t, home, cfgFile, sp)

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

			// 3. Played once, not repeatedly.
			time.Sleep(10 * time.Second)
			if n := len(sp.Loads()); n != 1 {
				t.Errorf("speaker received %d LOAD commands, want exactly 1", n)
			}

			// 4. The agent stayed healthy.
			if out := ag.logs.String(); strings.Contains(out, "panic:") {
				t.Errorf("agent panicked")
			}
		})
	}
}
