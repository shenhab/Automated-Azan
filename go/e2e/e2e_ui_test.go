//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"azan-agent/internal/fakecast"
)

// todayFixture writes a timetable whose times are the same every day, with a
// Dhuhr near solar noon on the Dublin wall clock today, and returns them.
func todayFixture(t *testing.T, home string) [6]string {
	t.Helper()
	_, off := time.Now().In(dublin).Zone()
	times := [6]string{"03:10", "05:00", fmt.Sprintf("%02d:25", 12+off/3600), "15:00", "18:00", "20:00"}
	writeYearTimetable(t, home, func(time.Month, int) [6]string { return times })
	return times
}

type apiResp struct {
	Success bool           `json:"success"`
	Error   string         `json:"error"`
	Config  map[string]any `json:"config"`
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestWebUIAndAPI exercises the web interface of a running agent the way a
// person (or the dashboard's JavaScript) would: logging in, loading pages,
// reading status, controlling the speaker, changing settings and fetching media.
func TestWebUIAndAPI(t *testing.T) {
	t.Parallel()
	sp := newSpeaker(t)
	home, cfgFile, webPort := t.TempDir(), filepath.Join(t.TempDir(), "azan.toml"), freePorts(t)
	times := todayFixture(t, home)
	writeConfig(t, cfgFile, webPort, "all")
	startAgent(t, home, cfgFile, sp, withCredentials(testUser, testPass))

	c := newClient(t, webPort)
	c.waitUntilUp()
	dockerMode := os.Getenv("E2E_DOCKER_IMAGE") != ""

	t.Run("authentication is enforced", func(t *testing.T) {
		c := newClient(t, webPort)
		c.t = t
		for _, path := range []string{"/api/scheduler-status", "/api/config", "/api/devices", "/media/media_Athan.mp3"} {
			if status, _, _ := c.get(path); status != http.StatusUnauthorized {
				t.Errorf("GET %s without login = %d, want 401", path, status)
			}
		}
		for _, path := range []string{"/", "/scheduler", "/settings", "/chromecasts", "/logs"} {
			status, hdr, _ := c.get(path)
			if status != http.StatusSeeOther || !strings.HasPrefix(hdr.Get("Location"), "/login") {
				t.Errorf("GET %s without login = %d -> %q, want a redirect to /login", path, status, hdr.Get("Location"))
			}
		}
		// Wrong password: no session.
		if status, gotCookie := c.login(testUser, "wrong-password"); gotCookie || status == http.StatusSeeOther {
			t.Errorf("a wrong password was accepted (status %d, cookie %v)", status, gotCookie)
		}
		if status, _, _ := c.get("/api/config"); status != http.StatusUnauthorized {
			t.Errorf("still not logged in, got %d", status)
		}
		// Unknown user.
		if _, gotCookie := c.login("nobody", testPass); gotCookie {
			t.Error("an unknown user got a session")
		}
		// Right password: session cookie is HttpOnly + SameSite.
		status, hdr, _ := c.do("POST", "/login", strings.NewReader(url.Values{"username": {testUser}, "password": {testPass}}.Encode()),
			"application/x-www-form-urlencoded", nil)
		cookie := hdr.Get("Set-Cookie")
		if status != http.StatusSeeOther || !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
			t.Errorf("login = %d, Set-Cookie %q: want 303 with an HttpOnly, SameSite=Lax session cookie", status, cookie)
		}
		if status, _, _ := c.get("/api/config"); status != http.StatusOK {
			t.Errorf("after login /api/config = %d, want 200", status)
		}
		// Logout ends the session.
		c.get("/logout")
		if status, _, _ := c.get("/api/config"); status != http.StatusUnauthorized {
			t.Errorf("after logout /api/config = %d, want 401", status)
		}
	})

	c.mustLogin()

	t.Run("every page renders", func(t *testing.T) {
		for _, path := range []string{"/", "/scheduler", "/chromecasts", "/settings", "/logs"} {
			status, hdr, body := c.get(path)
			if status != http.StatusOK || !strings.HasPrefix(hdr.Get("Content-Type"), "text/html") || !bytes.Contains(bytes.ToLower(body), []byte("<html")) {
				t.Errorf("GET %s = %d %q (%d bytes), want an HTML page", path, status, hdr.Get("Content-Type"), len(body))
			}
		}
	})

	t.Run("status endpoints reflect the real configuration", func(t *testing.T) {
		var pt struct {
			Success     bool              `json:"success"`
			PrayerTimes map[string]string `json:"prayer_times"`
			Location    string            `json:"location"`
		}
		c.getJSON("/api/prayer-times", &pt)
		want := map[string]string{"Fajr": times[0], "Dhuhr": times[2], "Asr": times[3], "Maghrib": times[4], "Isha": times[5]}
		if !pt.Success || pt.Location != "naas" {
			t.Fatalf("prayer-times = %+v", pt)
		}
		for p, w := range want {
			if pt.PrayerTimes[p] != w {
				t.Errorf("%s = %q, want %q", p, pt.PrayerTimes[p], w)
			}
		}

		var st struct {
			Jobs []struct{ Prayer string } `json:"jobs"`
		}
		c.getJSON("/api/scheduler-status", &st)
		if len(st.Jobs) != 5 {
			t.Errorf("scheduler lists %d jobs, want 5", len(st.Jobs))
		}

		var cfg apiResp
		c.getJSON("/api/config", &cfg)
		if cfg.Config["speakers_group_name"] != speakerName || cfg.Config["location"] != "naas" {
			t.Errorf("config = %v", cfg.Config)
		}

		var media struct{ Files []string }
		c.getJSON("/api/media/files", &media)
		found := false
		for _, f := range media.Files {
			found = found || f == "media_Athan.mp3"
		}
		if !found {
			t.Errorf("media files %v do not include media_Athan.mp3", media.Files)
		}
	})

	t.Run("discovery finds the speaker", func(t *testing.T) {
		var d struct {
			Success bool
			Count   int
			Devices []struct {
				Name string
				Port int
			}
		}
		if status := c.postJSON("/api/discover-devices", map[string]any{}, &d); status != http.StatusOK || !d.Success {
			t.Fatalf("discover-devices = %d %+v", status, d)
		}
		_, port := sp.Addr()
		ok := false
		for _, dev := range d.Devices {
			ok = ok || (dev.Name == speakerName && dev.Port == port)
		}
		if !ok {
			t.Errorf("devices %+v do not include %q on port %d", d.Devices, speakerName, port)
		}
	})

	t.Run("playing and stopping reaches the speaker", func(t *testing.T) {
		before := len(sp.Loads())
		var r apiResp
		if c.postJSON("/api/play-athan", map[string]any{"prayer": "Fajr"}, &r); !r.Success {
			t.Fatalf("play-athan failed: %+v", r)
		}
		loads, err := sp.WaitForLoads(before+1, 15*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		l := loads[before]
		if !strings.HasSuffix(l.URL, "/media/media_adhan_al_fajr.mp3") || l.Fetch.Status != 200 || !l.Fetch.LooksLikeAudio {
			t.Errorf("speaker got %q (fetch %+v), want the Fajr Athan it can download", l.URL, l.Fetch)
		}

		if c.postJSON("/api/stop-playback", map[string]any{}, &r); !r.Success {
			t.Errorf("stop-playback failed: %+v", r)
		}
		deadline := time.Now().Add(5 * time.Second)
		for sp.PlayerState() != "IDLE" && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if st := sp.PlayerState(); st != "IDLE" {
			t.Errorf("speaker still %s after stop-playback", st)
		}

		before = len(sp.Loads())
		if c.postJSON("/api/test-workflow", map[string]any{"prayer": "Dhuhr"}, &r); !r.Success {
			t.Fatalf("test-workflow failed: %+v", r)
		}
		loads, err = sp.WaitForLoads(before+1, 15*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(loads[before].URL, "/media/media_Athan.mp3") {
			t.Errorf("test-workflow played %q", loads[before].URL)
		}

		// POST-only endpoints refuse GET.
		for _, path := range []string{"/api/play-athan", "/api/stop-playback", "/api/test-workflow", "/api/discover-devices"} {
			if status, _, _ := c.get(path); status != http.StatusMethodNotAllowed {
				t.Errorf("GET %s = %d, want 405", path, status)
			}
		}
	})

	t.Run("a request that cannot play is reported as failed, not as success", func(t *testing.T) {
		before := len(sp.Loads())
		for _, name := range []string{"../../etc/passwd", "does-not-exist.mp3", "sub/dir.mp3"} {
			var r apiResp
			c.postJSON("/api/play-athan", map[string]any{"prayer": "Dhuhr", "filename": name}, &r)
			if r.Success {
				t.Errorf("play-athan with filename %q reported success, but the speaker cannot play that", name)
			}
		}
		time.Sleep(time.Second)
		if n := len(sp.Loads()); n != before {
			t.Errorf("the speaker was sent %d bogus LOAD command(s)", n-before)
		}
	})

	t.Run("invalid settings are rejected and change nothing", func(t *testing.T) {
		diskBefore := readFile(t, cfgFile)
		for name, body := range map[string]map[string]any{
			"unknown location":    {"location": "atlantis"},
			"negative pre-fajr":   {"pre_fajr_minutes": -5},
			"huge pre-fajr":       {"pre_fajr_minutes": 100000},
			"empty location":      {"location": ""},
			"empty speaker group": {"speakers_group_name": ""},
		} {
			var r apiResp
			c.postJSON("/api/config", body, &r)
			if r.Success {
				t.Errorf("%s: saved without complaint", name)
			}
		}
		var cfg apiResp
		c.getJSON("/api/config", &cfg)
		if cfg.Config["location"] != "naas" || cfg.Config["speakers_group_name"] != speakerName {
			t.Errorf("invalid input changed the live config: %v", cfg.Config)
		}
		if after := readFile(t, cfgFile); after != diskBefore {
			t.Error("invalid input changed the config file on disk")
		}
		// Garbage must not take the server down.
		if status, _, _ := c.do("POST", "/api/config", strings.NewReader("{not json"), "application/json", nil); status >= 500 {
			t.Errorf("malformed JSON gave %d", status)
		}
		if status, _, _ := c.get("/api/config"); status != http.StatusOK {
			t.Errorf("server unhealthy after bad input: %d", status)
		}
	})

	t.Run("a settings change is saved and re-plans the schedule", func(t *testing.T) {
		jobs := func() map[string]bool {
			var st struct {
				Jobs []struct{ Prayer string } `json:"jobs"`
			}
			c.getJSON("/api/scheduler-status", &st)
			m := map[string]bool{}
			for _, j := range st.Jobs {
				m[j.Prayer] = true
			}
			return m
		}
		wait := func(what string, cond func() bool) {
			t.Helper()
			deadline := time.Now().Add(20 * time.Second)
			for !cond() {
				if time.Now().After(deadline) {
					t.Fatalf("timed out waiting for: %s", what)
				}
				time.Sleep(300 * time.Millisecond)
			}
		}
		var r apiResp
		if c.postJSON("/api/config", map[string]any{"fajr_enabled": false}, &r); !r.Success {
			t.Fatalf("saving failed: %+v", r)
		}
		if !strings.Contains(readFile(t, cfgFile), "fajr = false") {
			t.Errorf("the change was not written to the config file:\n%s", readFile(t, cfgFile))
		}
		wait("Fajr to leave the schedule", func() bool { return !jobs()["Fajr"] })
		if !jobs()["Dhuhr"] {
			t.Error("disabling Fajr removed other prayers too")
		}
		c.postJSON("/api/config", map[string]any{"fajr_enabled": true}, &r)
		wait("Fajr to return to the schedule", func() bool { return jobs()["Fajr"] })
	})

	t.Run("TV-mute settings round-trip", func(t *testing.T) {
		var r apiResp
		if c.postJSON("/api/tv-pause/config", map[string]any{"enabled": true, "resume_delay_secs": 120, "devices": []string{"abc"}}, &r); !r.Success {
			t.Fatalf("saving failed: %+v", r)
		}
		var st struct {
			Enabled bool
			Delay   int `json:"resume_delay_secs"`
			Devices []string
			Paused  bool
		}
		c.getJSON("/api/tv-pause/status", &st)
		if !st.Enabled || st.Delay != 120 || len(st.Devices) != 1 || st.Paused {
			t.Errorf("tv-pause status = %+v", st)
		}
		if !strings.Contains(readFile(t, cfgFile), "resume_delay_seconds = 120") {
			t.Error("TV-mute settings were not saved to the config file")
		}
		c.postJSON("/api/tv-pause/config", map[string]any{"enabled": false, "devices": []string{}}, &r)
	})

	t.Run("media is served, with ranges, and cannot be used to read other files", func(t *testing.T) {
		status, hdr, body := c.get("/media/media_Athan.mp3")
		if status != http.StatusOK || hdr.Get("Content-Type") != "audio/mpeg" || len(body) < 50_000 {
			t.Errorf("media_Athan.mp3 = %d %q %d bytes", status, hdr.Get("Content-Type"), len(body))
		}
		status, hdr, body = c.do("GET", "/media/media_Athan.mp3", nil, "", map[string]string{"Range": "bytes=0-99"})
		if status != http.StatusPartialContent || len(body) != 100 || !strings.HasPrefix(hdr.Get("Content-Range"), "bytes 0-99/") {
			t.Errorf("range request = %d, %d bytes, Content-Range %q; want 206 with 100 bytes", status, len(body), hdr.Get("Content-Range"))
		}
		for _, path := range []string{"/media/../azan.toml", "/media/%2e%2e/azan.toml", "/media/..%2fazan.toml", "/media/%2e%2e%2f%2e%2e%2fetc/passwd", "/media/"} {
			if status, _, body := c.get(path); status == http.StatusOK && len(body) > 0 {
				t.Errorf("GET %s returned %d bytes with 200 — path traversal", path, len(body))
			}
		}
	})

	t.Run("uploads are limited to audio and saved under a safe name", func(t *testing.T) {
		if dockerMode {
			t.Skip("the container runs as a non-root user in this harness, so /app/Media is read-only")
		}
		upload := func(filename string, data []byte) apiResp {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
			part, _ := mw.CreatePart(h)
			part.Write(data)
			mw.Close()
			_, _, body := c.do("POST", "/api/media/upload", &buf, mw.FormDataContentType(), nil)
			var r apiResp
			_ = jsonUnmarshal(body, &r)
			return r
		}
		if r := upload("notes.txt", []byte("hello")); r.Success {
			t.Error("a .txt file was accepted as media")
		}
		if r := upload("../../escape.mp3", append([]byte("ID3"), make([]byte, 2048)...)); !r.Success {
			t.Errorf("a valid mp3 upload failed: %+v", r)
		}
		_, _, body := c.get("/media/escape.mp3")
		if len(body) != 2051 {
			t.Errorf("the upload was not stored under its base name (served %d bytes)", len(body))
		}
		var media struct{ Files []string }
		c.getJSON("/api/media/files", &media)
		listed := false
		for _, f := range media.Files {
			listed = listed || f == "escape.mp3"
			if strings.Contains(f, "/") || strings.Contains(f, "..") {
				t.Errorf("unsafe name in media list: %q", f)
			}
		}
		if !listed {
			t.Errorf("the uploaded file is not listed: %v", media.Files)
		}
	})

	t.Run("websocket needs a session and stays open", func(t *testing.T) {
		wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", webPort)
		if _, resp, err := websocket.DefaultDialer.Dial(wsURL, nil); err == nil {
			t.Error("an unauthenticated websocket connection was accepted")
		} else if resp != nil && resp.StatusCode == http.StatusSwitchingProtocols {
			t.Errorf("unauthenticated upgrade returned 101")
		}
		u, _ := url.Parse(c.base)
		var cookies []string
		for _, ck := range c.http.Jar.Cookies(u) {
			cookies = append(cookies, ck.Name+"="+ck.Value)
		}
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Cookie": {strings.Join(cookies, "; ")}})
		if err != nil {
			t.Fatalf("authenticated websocket failed: %v", err)
		}
		defer conn.Close()
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			t.Errorf("connection not usable: %v", err)
		}
	})

	_ = fakecast.New // keep the import honest if subtests are skipped
}
