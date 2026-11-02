package chromecast

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ConfiguredGroup is one group a device says it belongs to.
type ConfiguredGroup struct {
	Name     string `json:"name"`
	UUID     string `json:"uuid"`
	Leader   string `json:"leader"`    // "self", or the leader's "ip:port"
	CastPort int    `json:"cast_port"` // only when Leader is "self"
}

const (
	castGroupModel = "Google Cast Group"
	eurekaTimeout  = 3 * time.Second
	eurekaMaxBytes = 1 << 20
)

// membershipTimeout caps a whole Membership call. It runs before the Athan, and
// a device that hangs must not hold that up.
var membershipTimeout = 6 * time.Second

// eurekaClient reads the devices' status pages. Their certificates are
// self-signed and cannot be verified; this is a read of a status page on the
// LAN, nothing secret is sent and nothing is changed.
var eurekaClient = &http.Client{
	Timeout:   eurekaTimeout,
	Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
}

func defaultEurekaURL(d Device) string {
	// Only the HTTPS port honours params=multizone (measured 2026-11-02: port
	// 8008 ignores it, and no token is needed for this page on 8443).
	return "https://" + net.JoinHostPort(d.Host, strconv.Itoa(8443)) + "/setup/eureka_info?params=multizone,name"
}

// eurekaURL says where a device's status page is (tests replace it).
var eurekaURL = defaultEurekaURL

// DeviceGroups reads the groups a device is configured into from its status
// page at url. A page without a multizone section is an error: that is "unknown",
// not "in no group". Read-only: one GET.
func DeviceGroups(ctx context.Context, url string) (string, []ConfiguredGroup, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := eurekaClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("status page answered %d", resp.StatusCode)
	}
	var page struct {
		Name      string `json:"name"`
		Multizone *struct {
			Groups []struct {
				Name          string `json:"name"`
				UUID          string `json:"uuid"`
				ElectedLeader string `json:"elected_leader"`
				CastPort      int    `json:"cast_port"`
			} `json:"groups"`
		} `json:"multizone"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, eurekaMaxBytes)).Decode(&page); err != nil {
		return "", nil, fmt.Errorf("status page: %w", err)
	}
	if page.Multizone == nil {
		return "", nil, fmt.Errorf("status page has no multizone section")
	}
	var out []ConfiguredGroup
	for _, g := range page.Multizone.Groups {
		out = append(out, ConfiguredGroup{Name: g.Name, UUID: g.UUID, Leader: g.ElectedLeader, CastPort: g.CastPort})
	}
	return page.Name, out, nil
}

// Membership is everything known about who is in the speaker's group.
type Membership struct {
	Group      Device            `json:"group"`
	CheckedAt  time.Time         `json:"checked_at"`
	Configured []Device          `json:"configured"`       // devices whose own page lists the group
	Live       []GroupMember     `json:"live"`             // who the group says has joined, with volumes
	LiveKnown  bool              `json:"live_known"`       // the group answered
	Joined     []Device          `json:"joined"`           // the live members found on the network
	NotJoined  []Device          `json:"not_joined"`       // configured but absent from the live list: will not play
	Unknown    []GroupMember     `json:"unknown"`          // live members discovery does not know: cannot be watched
	Errors     map[string]string `json:"errors,omitempty"` // device name → why it could not be asked
}

// IsGroup says whether the speaker is really made of other devices. A plain
// speaker lists only itself.
func (ms Membership) IsGroup() bool {
	for _, l := range ms.Live {
		if !strings.EqualFold(l.Name, ms.Group.Name) {
			return true
		}
	}
	for _, d := range ms.Configured {
		if !sameEndpoint(d, ms.Group) {
			return true
		}
	}
	return false
}

func sameEndpoint(a, b Device) bool { return a.Host == b.Host && a.Port == b.Port }

func listsGroup(groups []ConfiguredGroup, speaker Device) bool {
	for _, g := range groups {
		if g.UUID != "" && normID(g.UUID) == normID(speaker.UUID) {
			return true
		}
	}
	for _, g := range groups {
		if strings.EqualFold(g.Name, speaker.Name) {
			return true
		}
	}
	return false
}

func sortDevices(ds []Device) {
	sort.Slice(ds, func(i, j int) bool { return strings.ToLower(ds[i].Name) < strings.ToLower(ds[j].Name) })
}

// Membership works out who is in the speaker's group: each device's own page
// says who is configured into it, the group says who has joined. The difference
// is a speaker the Athan will not reach. Read-only and bounded by
// membershipTimeout; a device that cannot be asked goes into Errors and the rest
// of the answer stands. It opens its own connections, so it is never part of the
// Athan's play path: call it before (session start) or in the background.
func (m *Manager) Membership(ctx context.Context) (Membership, error) {
	speaker, err := m.resolveDevice()
	if err != nil {
		return Membership{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, membershipTimeout)
	defer cancel()

	ms := Membership{Group: speaker, CheckedAt: time.Now()}
	devices := m.Devices()
	var mu sync.Mutex
	fail := func(name string, err error) {
		mu.Lock()
		defer mu.Unlock()
		if ms.Errors == nil {
			ms.Errors = map[string]string{}
		}
		ms.Errors[name] = err.Error()
	}

	var wg sync.WaitGroup
	for _, d := range devices {
		if d.ModelName == castGroupModel || sameEndpoint(d, speaker) {
			continue // a group has no page of its own: it lives on a member's
		}
		wg.Add(1)
		go func(d Device) {
			defer wg.Done()
			_, groups, err := DeviceGroups(ctx, eurekaURL(d))
			if err != nil {
				fail(d.Name, err)
				return
			}
			if listsGroup(groups, speaker) {
				mu.Lock()
				ms.Configured = append(ms.Configured, d)
				mu.Unlock()
			}
		}(d)
	}
	type liveReply struct {
		members []GroupMember
		err     error
	}
	reply := make(chan liveReply, 1) // buffered: an abandoned query can still finish
	go func() {
		members, err := GroupMembers(speaker.Host, speaker.Port)
		reply <- liveReply{members, err}
	}()
	select {
	case r := <-reply:
		if r.err != nil {
			fail(speaker.Name, r.err)
		} else {
			ms.Live, ms.LiveKnown = r.members, true
		}
	case <-ctx.Done():
		fail(speaker.Name, fmt.Errorf("did not list its members in time"))
	}
	wg.Wait()

	if ms.LiveKnown {
		ms.Joined, ms.Unknown = MatchMembers(ms.Live, devices)
		for _, d := range ms.Configured {
			joined := false
			for _, j := range ms.Joined {
				joined = joined || sameEndpoint(j, d)
			}
			if !joined {
				ms.NotJoined = append(ms.NotJoined, d)
			}
		}
		if len(ms.Configured) == 0 && !ms.IsGroup() && len(ms.Live) > 0 {
			ms.Configured = []Device{speaker} // a plain speaker
		}
	}
	sortDevices(ms.Configured)
	sortDevices(ms.Joined)
	sortDevices(ms.NotJoined)

	m.mu.Lock()
	m.lastMembership, m.hasMembership = ms, true
	m.mu.Unlock()
	if ms.IsGroup() {
		log.Printf("[chromecast] %s", ms.Summary())
	}
	return ms, nil
}

// LastMembership returns the latest Membership result, if there has been one.
func (m *Manager) LastMembership() (Membership, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastMembership, m.hasMembership
}

// Summary says it in a sentence, for the log.
func (ms Membership) Summary() string {
	s := fmt.Sprintf("%s: %d configured (%s), %d joined", ms.Group.Name, len(ms.Configured), joinNames(ms.Configured), len(ms.Joined))
	if !ms.LiveKnown {
		s = fmt.Sprintf("%s: %d configured (%s), the group did not say who has joined", ms.Group.Name, len(ms.Configured), joinNames(ms.Configured))
	}
	if len(ms.NotJoined) > 0 {
		s += " — NOT joined: " + joinNames(ms.NotJoined)
	}
	return s
}

func joinNames(ds []Device) string {
	names := make([]string, 0, len(ds))
	for _, d := range ds {
		names = append(names, d.Name)
	}
	return strings.Join(names, ", ")
}
