package journal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"azan-agent/internal/trace"
)

// HealthSnapshot is what a device's own status page (port 8008) says: only the
// parts that help diagnose, never the MAC address, keys or network ids.
type HealthSnapshot struct {
	Host      string    `json:"host"`
	Name      string    `json:"name,omitempty"`
	Phase     string    `json:"phase,omitempty"` // start | end
	At        time.Time `json:"at"`
	Reachable bool      `json:"reachable"`
	Uptime    float64   `json:"uptime_sec,omitempty"`
	Build     string    `json:"build,omitempty"`
	Connected bool      `json:"connected,omitempty"`
	HasUpdate bool      `json:"has_update,omitempty"`
	SSID      string    `json:"ssid,omitempty"`
	Ethernet  bool      `json:"ethernet,omitempty"`
}

// fetchHealth reads base+"/setup/eureka_info".
func fetchHealth(base string, timeout time.Duration) (HealthSnapshot, error) {
	c := &http.Client{Timeout: timeout}
	resp, err := c.Get(base + "/setup/eureka_info")
	if err != nil {
		return HealthSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return HealthSnapshot{}, fmt.Errorf("status page answered %d", resp.StatusCode)
	}
	var raw struct {
		Name      string  `json:"name"`
		Uptime    float64 `json:"uptime"`
		Build     string  `json:"cast_build_revision"`
		Connected bool    `json:"connected"`
		HasUpdate bool    `json:"has_update"`
		SSID      string  `json:"ssid"`
		Ethernet  bool    `json:"ethernet_connected"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return HealthSnapshot{}, err
	}
	return HealthSnapshot{At: time.Now(), Reachable: true, Name: raw.Name, Uptime: raw.Uptime, Build: raw.Build,
		Connected: raw.Connected, HasUpdate: raw.HasUpdate, SSID: raw.SSID, Ethernet: raw.Ethernet}, nil
}

func (j *Journal) health(host string) (HealthSnapshot, error) {
	if j.o.Health != nil {
		return j.o.Health(host)
	}
	return fetchHealth("http://"+host+":8008", 3*time.Second)
}

type healthSession struct {
	mu     sync.Mutex
	before map[string]HealthSnapshot
	after  map[string]HealthSnapshot
	prev   map[string]HealthSnapshot // the last end-of-recording snapshot from an earlier session
}

var healthStateMu sync.Mutex

func (r *Run) hosts() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range r.targets {
		if t.Host != "" && !seen[t.Host] {
			seen[t.Host] = true
			out = append(out, t.Host)
		}
	}
	sort.Strings(out)
	return out
}

// snapshotAll reads every distinct host's status page in parallel.
func (r *Run) snapshotAll(phase string) map[string]HealthSnapshot {
	hosts := r.hosts()
	out := make(map[string]HealthSnapshot, len(hosts))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, h := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			s, err := r.j.health(host)
			s.Host, s.Phase = host, phase
			if s.At.IsZero() {
				s.At = r.j.o.Now()
			}
			if err != nil {
				s.Reachable = false
			}
			mu.Lock()
			out[host] = s
			mu.Unlock()
			text := fmt.Sprintf("status page (%s): ", phase)
			if s.Reachable {
				text += fmt.Sprintf("up %s, firmware %s", humanDuration(time.Duration(s.Uptime)*time.Second), s.Build)
			} else {
				text += "no answer"
			}
			r.add(trace.Event{At: s.At, Source: trace.SourceHealth, Kind: "snapshot", Device: nameOr(s), Text: text,
				Fields: map[string]any{"host": host, "phase": phase, "reachable": s.Reachable, "uptime_sec": s.Uptime, "build": s.Build, "connected": s.Connected}})
		}(h)
	}
	wg.Wait()
	return out
}

func nameOr(s HealthSnapshot) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Host
}

// start reads the first status pages without delaying the session.
func (r *Run) start() {
	if len(r.targets) == 0 {
		r.startWatchers() // still enforces the time limit
		return
	}
	hs := &healthSession{prev: r.j.loadHealthState()}
	r.hs = hs
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		b := r.snapshotAll("start")
		hs.mu.Lock()
		hs.before = b
		hs.mu.Unlock()
	}()
	r.startWatchers()
}

// finishHealth reads the last status pages, compares them with the start and
// with the previous recording, saves them, and returns the problems found.
func (r *Run) finishHealth() []string {
	hs := r.hs
	if hs == nil {
		return nil
	}
	after := r.snapshotAll("end")
	hs.mu.Lock()
	hs.after = after
	before, prev := hs.before, hs.prev
	hs.mu.Unlock()

	var issues []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			issues = append(issues, s)
		}
	}
	for _, host := range r.hosts() {
		b, a, p := before[host], after[host], prev[host]
		name := nameOr(b)
		if !b.Reachable {
			name = nameOr(a)
		}
		if p.Reachable && b.Reachable && b.Uptime < p.Uptime {
			add(name + " restarted since the last recording")
		}
		if p.Reachable && b.Reachable && p.Build != "" && b.Build != "" && p.Build != b.Build {
			add(fmt.Sprintf("%s firmware changed (%s → %s)", name, p.Build, b.Build))
		}
		if p.Reachable && !b.Reachable {
			add(name + "'s status page stopped answering")
		}
		if b.Reachable && a.Reachable && a.Uptime < b.Uptime {
			add(name + " restarted during the recording")
		}
		if a.Reachable && !a.Connected {
			add(name + " reports it is not connected")
		}
	}

	if r.dir != "" {
		var all []HealthSnapshot
		for _, host := range r.hosts() {
			all = append(all, before[host], after[host])
		}
		if b, err := json.MarshalIndent(all, "", "  "); err == nil {
			os.WriteFile(filepath.Join(r.dir, "health.json"), b, 0o644)
		}
	}
	// remember the last good picture of each device for the next recording
	state := r.j.loadHealthState()
	for host, a := range after {
		if a.Reachable {
			state[host] = a
		} else if b, ok := before[host]; ok && b.Reachable {
			state[host] = b
		}
	}
	r.j.saveHealthState(state)
	return issues
}

func readHealthFile(dir string) []HealthSnapshot {
	b, err := os.ReadFile(filepath.Join(dir, "health.json"))
	if err != nil {
		return nil
	}
	var out []HealthSnapshot
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

func (j *Journal) loadHealthState() map[string]HealthSnapshot {
	healthStateMu.Lock()
	defer healthStateMu.Unlock()
	state := map[string]HealthSnapshot{}
	if j.o.Dir == "" {
		return state
	}
	if b, err := os.ReadFile(filepath.Join(j.o.Dir, "health-state.json")); err == nil {
		json.Unmarshal(b, &state)
	}
	return state
}

func (j *Journal) saveHealthState(state map[string]HealthSnapshot) {
	if j.o.Dir == "" {
		return
	}
	healthStateMu.Lock()
	defer healthStateMu.Unlock()
	if b, err := json.MarshalIndent(state, "", "  "); err == nil {
		os.WriteFile(filepath.Join(j.o.Dir, "health-state.json"), b, 0o644)
	}
}
