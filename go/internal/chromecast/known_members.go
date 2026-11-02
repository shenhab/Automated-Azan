package chromecast

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The agent remembers who it has seen in each group. Nobody lists them: it is all
// learned from discovery. An unplugged speaker answers nothing and vanishes from
// every source, so this memory is the only way to notice it is missing.

const (
	knownMembersFile = "group_members.json"
	memberProbeLimit = 2 * time.Second
)

// memberForgetAfter is how long an unreachable member is remembered. A speaker
// given away can never say it left, and must not warn at every prayer forever.
var memberForgetAfter = 30 * 24 * time.Hour

type knownMember struct {
	Device   Device    `json:"device"`
	LastSeen time.Time `json:"last_seen"`
}

func sameDevice(a, b Device) bool {
	if a.UUID != "" && normID(a.UUID) == normID(b.UUID) {
		return true
	}
	return strings.EqualFold(a.Name, b.Name)
}

func groupKey(d Device) string {
	if d.UUID != "" {
		return normID(d.UUID)
	}
	return strings.ToLower(d.Name)
}

func hasDevice(list []Device, d Device) bool {
	for _, x := range list {
		if sameDevice(x, d) {
			return true
		}
	}
	return false
}

// reconcileKnownMembers compares what is known about the group now with what was
// learned earlier, fills in ms.Offline and updates the memory.
//
// A remembered member that is seen now stays. One whose own page answers without
// listing the group was taken out of it, and is forgotten. One that cannot be asked
// but still answers on its Cast port is kept. Anything else is offline.
func (m *Manager) reconcileKnownMembers(ms *Membership, devices, answered []Device) {
	m.knownMu.Lock()
	defer m.knownMu.Unlock()
	m.mu.Lock()
	dir := m.cacheDir
	m.mu.Unlock()
	if m.known == nil {
		m.known = loadKnownMembers(dir)
	}
	key := groupKey(ms.Group)
	now := time.Now()

	var next []knownMember
	seen := func(d Device) {
		if sameEndpoint(d, ms.Group) {
			return
		}
		for i := range next {
			if sameDevice(next[i].Device, d) {
				next[i] = knownMember{d, now}
				return
			}
		}
		next = append(next, knownMember{d, now})
	}
	for _, k := range m.known[key] {
		d := k.Device
		for _, cur := range devices {
			if sameDevice(cur, d) {
				d = cur // it may have a new address
				break
			}
		}
		switch {
		case hasDevice(ms.Configured, d) || hasDevice(ms.Joined, d):
			// seen below
		case hasDevice(answered, d):
			log.Printf("[chromecast] %s is no longer in %s", d.Name, ms.Group.Name)
		case answersOnCastPort(d):
			next = append(next, knownMember{d, now})
		case now.Sub(k.LastSeen) >= memberForgetAfter:
			log.Printf("[chromecast] forgetting %s: not seen in %s since %s", d.Name, ms.Group.Name, k.LastSeen.Format("2006-01-02"))
		default:
			ms.Offline = append(ms.Offline, d)
			next = append(next, knownMember{d, k.LastSeen})
		}
	}
	// A plain speaker lists only itself, and "seen" skips it.
	for _, d := range ms.Configured {
		seen(d)
	}
	for _, d := range ms.Joined {
		seen(d)
	}

	if len(next) == 0 {
		if _, had := m.known[key]; !had {
			return
		}
		delete(m.known, key)
	} else {
		m.known[key] = next
	}
	saveKnownMembers(dir, m.known)
}

func answersOnCastPort(d Device) bool {
	ctx, cancel := context.WithTimeout(context.Background(), memberProbeLimit)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(d.Host, strconv.Itoa(d.Port)))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func loadKnownMembers(dir string) map[string][]knownMember {
	out := map[string][]knownMember{}
	if dir == "" {
		return out
	}
	b, err := os.ReadFile(filepath.Join(dir, knownMembersFile))
	if err != nil {
		return out
	}
	if err := json.Unmarshal(b, &out); err != nil || out == nil {
		log.Printf("[chromecast] ignoring a damaged %s: %v", knownMembersFile, err)
		return map[string][]knownMember{}
	}
	return out
}

func saveKnownMembers(dir string, known map[string][]knownMember) {
	if dir == "" {
		return
	}
	b, err := json.MarshalIndent(known, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(dir, knownMembersFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("[chromecast] cannot save %s: %v", knownMembersFile, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("[chromecast] cannot save %s: %v", knownMembersFile, err)
	}
}
