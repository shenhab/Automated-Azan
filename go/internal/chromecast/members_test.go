package chromecast

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"azan-agent/internal/fakecast"
)

func newGroup(t *testing.T, name string, members ...fakecast.GroupMember) *fakecast.Speaker {
	t.Helper()
	g := fakecast.New(name)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	g.SetGroupMembers(members...)
	return g
}

func TestAGroupListsItsRealMembersAndTheirVolumes(t *testing.T) {
	g := newGroup(t, "athan",
		fakecast.GroupMember{DeviceID: "e5651925-f19b-385f-80bb-b46e2ca681ed", Name: "Nest display", Level: 0.5},
		fakecast.GroupMember{DeviceID: "de377bef-485a-f47a-4141-8d94175b3160", Name: "Kitchen speaker", Level: 0.8, Muted: true},
	)
	host, port := g.Addr()
	got, err := GroupMembers(host, port)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Nest display" || got[1].Name != "Kitchen speaker" {
		t.Fatalf("members = %+v", got)
	}
	if !got[1].Muted || got[1].Level < 0.79 || got[1].Level > 0.81 {
		t.Errorf("the Kitchen speaker's own volume was not reported: %+v", got[1])
	}
	if got[1].ID() != "de377bef485af47a41418d94175b3160" {
		t.Errorf("ID() = %q, want the id without dashes", got[1].ID())
	}
}

func TestAPlainDeviceHasNoGroupMembers(t *testing.T) {
	d := newGroup(t, "Nest display")
	host, port := d.Addr()
	got, err := GroupMembers(host, port)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("members = %+v, want none", got)
	}
}

func TestMembersAreMatchedToDiscoveredDevicesByIDThenByName(t *testing.T) {
	devs := []Device{
		{UUID: "e5651925f19b385f80bbb46e2ca681ed", Name: "Nest display", Host: "10.0.0.2", Port: 8009},
		{UUID: "static-kitchen-speaker", Name: "kitchen SPEAKER", Host: "10.0.0.3", Port: 8009},
		{UUID: "other", Name: "Other", Host: "10.0.0.4", Port: 8009},
	}
	members := []GroupMember{
		{DeviceID: "e5651925-f19b-385f-80bb-b46e2ca681ed", Name: "renamed since"},
		{DeviceID: "de377bef-485a-f47a-4141-8d94175b3160", Name: "Kitchen speaker"},
		{DeviceID: "11111111-2222-3333-4444-555555555555", Name: "Not on the network"},
	}
	found, missing := MatchMembers(members, devs)
	if len(found) != 2 || found[0].Host != "10.0.0.2" || found[1].Host != "10.0.0.3" {
		t.Errorf("found = %+v", found)
	}
	if len(missing) != 1 || missing[0].Name != "Not on the network" {
		t.Errorf("missing = %+v", missing)
	}
}

func TestAMutedMemberOfTheGroupIsFlaggedEvenWhenTheGroupIsAudible(t *testing.T) {
	// The group reports volume 0.5, not muted, but one of its members is muted:
	// the Athan starts and that speaker stays silent.
	m, sp := newRig(t)
	sp.SetGroupMembers(
		fakecast.GroupMember{DeviceID: "aaaaaaaa-0000-0000-0000-000000000001", Name: "Nest display", Level: 0.5},
		fakecast.GroupMember{DeviceID: "aaaaaaaa-0000-0000-0000-000000000002", Name: "Kitchen speaker", Level: 0.8, Muted: true},
	)
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	m.CheckGroupMembers()
	pb := m.LastPlayback()
	if !pb.Started {
		t.Fatalf("not started: %+v", pb)
	}
	if !strings.Contains(pb.Warning, "Kitchen speaker") || !strings.Contains(strings.ToLower(pb.Warning), "muted") {
		t.Errorf("warning = %q, want it to name the muted Kitchen speaker", pb.Warning)
	}
	if strings.Contains(pb.Warning, "Nest display") {
		t.Errorf("warning %q blames the audible Nest display", pb.Warning)
	}
	if len(pb.Members) != 2 {
		t.Errorf("members in the report = %+v, want both", pb.Members)
	}
}

func TestAnAudibleGroupRaisesNoMemberWarning(t *testing.T) {
	m, sp := newRig(t)
	sp.SetGroupMembers(
		fakecast.GroupMember{DeviceID: "aaaaaaaa-0000-0000-0000-000000000001", Name: "Nest display", Level: 0.5},
		fakecast.GroupMember{DeviceID: "aaaaaaaa-0000-0000-0000-000000000002", Name: "Kitchen speaker", Level: 0.8},
	)
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	m.CheckGroupMembers()
	if w := m.LastPlayback().Warning; w != "" {
		t.Errorf("warned about an audible group: %q", w)
	}
}

func TestPlayingTheAthanOpensNoExtraConnectionToTheGroup(t *testing.T) {
	// Asking for the members is a separate, later step: the Athan's own path
	// must keep using its one warm connection.
	m, sp := newRig(t)
	sp.SetGroupMembers(fakecast.GroupMember{DeviceID: "aaaaaaaa-0000-0000-0000-000000000001", Name: "Nest display", Level: 0.5})
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	if n := sp.Connections(); n != 1 {
		t.Errorf("%d connections while playing, want 1", n)
	}
}

// Real replies from the status pages (2026-11-02), names and ids unchanged in
// shape: a leader has cast_port and elected_leader "self"; a member has neither
// port nor "self"; a TV has no groups.
const (
	eurekaLeader = `{"name":"Nest display","multizone":{"audio_output_delay":0.0,"dynamic_groups":[],"groups":[
	 {"cast_port":32003,"elected_leader":"self","leader":"","name":"Upstairs","uuid":"022a67d1-b1ed-4335-9e11-80172b59bf24","created_by_cloud":false,"multichannel_group":false,"channel_selection":""},
	 {"cast_port":32067,"elected_leader":"self","leader":"","name":"athan","uuid":"7738a185-2ecf-4e7a-af11-b330a1bfcec5"}],
	 "max_static_groups":100,"multichannel_status":0},"device_info":{"capabilities":{"multizone_supported":true}}}`
	eurekaMember = `{"name":"Kitchen speaker","multizone":{"groups":[
	 {"elected_leader":"192.168.86.187:10001","leader":"","name":"athan","uuid":"7738a185-2ecf-4e7a-af11-b330a1bfcec5"},
	 {"cast_port":32124,"elected_leader":"self","leader":"","name":"All Nest minis","uuid":"bcda95f4-d624-4307-a575-a9ec700448e9"}]}}`
	eurekaTV = `{"name":"Living Room TV","multizone":{"groups":[],"dynamic_groups":[]}}`
)

func eurekaServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL + "/setup/eureka_info?params=multizone,name"
}

func TestAStatusPageSaysWhichGroupsADeviceIsIn(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantName string
		want                 []ConfiguredGroup
	}{
		{"leader", eurekaLeader, "Nest display", []ConfiguredGroup{
			{Name: "Upstairs", UUID: "022a67d1-b1ed-4335-9e11-80172b59bf24", Leader: "self", CastPort: 32003},
			{Name: "athan", UUID: "7738a185-2ecf-4e7a-af11-b330a1bfcec5", Leader: "self", CastPort: 32067},
		}},
		{"member", eurekaMember, "Kitchen speaker", []ConfiguredGroup{
			{Name: "athan", UUID: "7738a185-2ecf-4e7a-af11-b330a1bfcec5", Leader: "192.168.86.187:10001"},
			{Name: "All Nest minis", UUID: "bcda95f4-d624-4307-a575-a9ec700448e9", Leader: "self", CastPort: 32124},
		}},
		{"tv", eurekaTV, "Living Room TV", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := eurekaServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method %s: the status page must only be read", r.Method)
				}
				fmt.Fprint(w, tc.body)
			})
			name, groups, err := DeviceGroups(context.Background(), page)
			if err != nil {
				t.Fatal(err)
			}
			if name != tc.wantName || len(groups) != len(tc.want) {
				t.Fatalf("name=%q groups=%+v", name, groups)
			}
			for i := range groups {
				if groups[i] != tc.want[i] {
					t.Errorf("group %d = %+v, want %+v", i, groups[i], tc.want[i])
				}
			}
		})
	}
}

func TestAStatusPageThatCannotBeReadIsAnError(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"forbidden": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) },
		"not json":  func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>") },
		// Port 8008 and old firmware answer without the section: unknown, not "no groups".
		"no multizone section": func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"name":"x"}`) },
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := DeviceGroups(context.Background(), eurekaServer(t, h)); err == nil {
				t.Error("no error")
			}
		})
	}
}

func TestDeviceGroupsGivesUpWhenTheContextEnds(t *testing.T) {
	release := make(chan struct{})
	page := eurekaServer(t, func(w http.ResponseWriter, r *http.Request) { <-release })
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := DeviceGroups(ctx, page); err == nil {
		t.Error("no error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
}

// memberRig is a group and its speakers, all fake: every status page is served by
// one test server, by device name.
type memberRig struct {
	m      *Manager
	group  *fakecast.Speaker
	sp     map[string]*fakecast.Speaker
	groups map[string][]string // device name → the groups its status page lists
	broken map[string]http.HandlerFunc
	dir    string
}

func newMemberRig(t *testing.T, speakers ...string) *memberRig {
	t.Helper()
	t.Setenv("AZAN_DISABLE_MDNS", "1")
	r := &memberRig{group: fakecast.New("athan"), sp: map[string]*fakecast.Speaker{}, groups: map[string][]string{}, broken: map[string]http.HandlerFunc{}}
	if err := r.group.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.group.Close)
	host, port := r.group.Addr()
	spec := []string{fmt.Sprintf("athan=%s:%d", host, port)}
	for _, n := range speakers {
		s := fakecast.New(n)
		if err := s.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		r.sp[n] = s
		h, p := s.Addr()
		spec = append(spec, fmt.Sprintf("%s=%s:%d", n, h, p))
	}
	t.Setenv("AZAN_STATIC_DEVICES", strings.Join(spec, ";"))

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		name := q.URL.Query().Get("dev")
		if h := r.broken[name]; h != nil {
			h(w, q)
			return
		}
		var gs []string
		for _, g := range r.groups[name] {
			gs = append(gs, fmt.Sprintf(`{"name":%q,"uuid":"static-%s","elected_leader":"self"}`, g, g))
		}
		fmt.Fprintf(w, `{"name":%q,"multizone":{"groups":[%s]}}`, name, strings.Join(gs, ","))
	}))
	t.Cleanup(srv.Close)
	old := eurekaURL
	eurekaURL = func(d Device) string { return srv.URL + "/setup/eureka_info?dev=" + url.QueryEscape(d.Name) }
	t.Cleanup(func() { eurekaURL = old })

	r.m = NewManager("athan")
	r.dir = t.TempDir()
	r.m.SetCacheDir(r.dir)
	return r
}

// restart is the agent starting again, with whatever is on the network now.
func (r *memberRig) restart() *Manager {
	m := NewManager("athan")
	m.SetCacheDir(r.dir)
	return m
}

func deviceNames(ds []Device) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return strings.Join(out, ",")
}

func TestAMemberConfiguredIntoTheGroupButNotJoinedIsFound(t *testing.T) {
	// The real case of 2026-11-02: three speakers configured, the group's session
	// has two, and the Athan silently did not reach the Playroom Speaker.
	r := newMemberRig(t, "Nest display", "Kitchen speaker", "Playroom Speaker", "Living area speaker")
	r.groups["Nest display"] = []string{"athan", "Upstairs"}
	r.groups["Kitchen speaker"] = []string{"athan"}
	r.groups["Playroom Speaker"] = []string{"Upstairs", "athan"}
	r.groups["Living area speaker"] = []string{"Downstairs"}
	r.sp["Nest display"].JoinGroup(r.group)
	r.sp["Kitchen speaker"].JoinGroup(r.group)

	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := deviceNames(ms.Configured); got != "Kitchen speaker,Nest display,Playroom Speaker" {
		t.Errorf("configured = %s", got)
	}
	if got := deviceNames(ms.Joined); got != "Kitchen speaker,Nest display" {
		t.Errorf("joined = %s", got)
	}
	if got := deviceNames(ms.NotJoined); got != "Playroom Speaker" {
		t.Errorf("not joined = %s", got)
	}
	if !ms.LiveKnown || len(ms.Live) != 2 || ms.CheckedAt.IsZero() {
		t.Errorf("live = %+v", ms)
	}
	if last, ok := r.m.LastMembership(); !ok || deviceNames(last.NotJoined) != "Playroom Speaker" {
		t.Errorf("LastMembership = %+v, %v", last, ok)
	}
}

func TestAllConfiguredMembersJoinedRaisesNothing(t *testing.T) {
	r := newMemberRig(t, "Nest display", "Kitchen speaker")
	r.groups["Nest display"] = []string{"athan"}
	r.groups["Kitchen speaker"] = []string{"athan"}
	r.sp["Nest display"].JoinGroup(r.group)
	r.sp["Kitchen speaker"].JoinGroup(r.group)
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.NotJoined) != 0 || len(ms.Unknown) != 0 || len(ms.Joined) != 2 {
		t.Errorf("%+v", ms)
	}
}

func TestAMemberThatCannotBeAskedDoesNotBlockTheOthers(t *testing.T) {
	r := newMemberRig(t, "Nest display", "Kitchen speaker", "Playroom Speaker")
	r.groups["Nest display"] = []string{"athan"}
	r.groups["Playroom Speaker"] = []string{"athan"}
	r.broken["Kitchen speaker"] = func(w http.ResponseWriter, q *http.Request) { w.WriteHeader(http.StatusForbidden) }
	r.sp["Nest display"].JoinGroup(r.group)
	r.sp["Kitchen speaker"].JoinGroup(r.group) // joined, though its page is broken

	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ms.Errors["Kitchen speaker"] == "" {
		t.Errorf("errors = %v", ms.Errors)
	}
	if got := deviceNames(ms.NotJoined); got != "Playroom Speaker" {
		t.Errorf("not joined = %s", got)
	}
	if got := deviceNames(ms.Joined); got != "Kitchen speaker,Nest display" {
		t.Errorf("joined = %s", got)
	}
}

func TestAHangingStatusPageIsCutShortSoTheAthanIsNotDelayed(t *testing.T) {
	old := membershipTimeout
	membershipTimeout = 300 * time.Millisecond
	t.Cleanup(func() { membershipTimeout = old })
	r := newMemberRig(t, "Nest display", "Kitchen speaker")
	r.groups["Nest display"] = []string{"athan"}
	release := make(chan struct{})
	defer close(release)
	r.broken["Kitchen speaker"] = func(w http.ResponseWriter, q *http.Request) { <-release }
	r.sp["Nest display"].JoinGroup(r.group)

	start := time.Now()
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
	if ms.Errors["Kitchen speaker"] == "" || deviceNames(ms.Configured) != "Nest display" {
		t.Errorf("%+v", ms)
	}
}

func TestWithoutTheGroupsOwnAnswerNobodyIsCalledNotJoined(t *testing.T) {
	// A group that does not answer says nothing about who joined; claiming "will
	// not play" for everyone would be false.
	r := newMemberRig(t, "Nest display")
	r.groups["Nest display"] = []string{"athan"}
	r.group.Close()
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ms.LiveKnown || len(ms.NotJoined) != 0 {
		t.Errorf("%+v", ms)
	}
	if deviceNames(ms.Configured) != "Nest display" || ms.Errors["athan"] == "" {
		t.Errorf("%+v", ms)
	}
}

func TestAPlainSpeakerIsItsOwnOnlyMember(t *testing.T) {
	// A plain device lists itself in its multizone status and no page lists it as a group.
	r := newMemberRig(t, "Kitchen speaker")
	r.groups["Kitchen speaker"] = []string{"Downstairs"}
	r.group.SetGroupMembers(fakecast.GroupMember{DeviceID: "aaaaaaaa-0000-0000-0000-000000000001", Name: "athan", Level: 0.5})
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if deviceNames(ms.Configured) != "athan" || deviceNames(ms.Joined) != "athan" || len(ms.NotJoined) != 0 || ms.IsGroup() {
		t.Errorf("%+v", ms)
	}
}

func TestALiveMemberThatDiscoveryDoesNotKnowIsReported(t *testing.T) {
	r := newMemberRig(t, "Nest display")
	r.sp["Nest display"].JoinGroup(r.group)
	r.group.SetGroupMembers(fakecast.GroupMember{DeviceID: "aaaaaaaa-0000-0000-0000-000000000009", Name: "Stranger", Level: 0.5})
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.Unknown) != 1 || ms.Unknown[0].Name != "Stranger" {
		t.Errorf("unknown = %+v", ms.Unknown)
	}
}

func TestEveryDeviceIsAskedOverTheEncryptedPort(t *testing.T) {
	// Port 8008 ignores params=multizone (measured); only 8443 answers it.
	got := defaultEurekaURL(Device{Host: "192.168.86.61", Port: 32124})
	if got != "https://192.168.86.61:8443/setup/eureka_info?params=multizone,name" {
		t.Errorf("url = %s", got)
	}
}

// The agent learns the group's members from discovery alone; nobody lists them.
// An unplugged speaker answers nothing, so only that memory can tell it is missing.

func learned(t *testing.T) *memberRig {
	t.Helper()
	r := newMemberRig(t, "Nest display", "Kitchen speaker", "Playroom Speaker")
	r.groups["Nest display"] = []string{"athan"}
	r.groups["Kitchen speaker"] = []string{"athan"}
	r.groups["Playroom Speaker"] = []string{"athan"}
	r.sp["Nest display"].JoinGroup(r.group)
	r.sp["Kitchen speaker"].JoinGroup(r.group)
	r.sp["Playroom Speaker"].JoinGroup(r.group)
	if _, err := r.m.Membership(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNothingIsOfflineWhileEveryKnownMemberAnswers(t *testing.T) {
	r := learned(t)
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.Offline) != 0 {
		t.Errorf("offline = %s", deviceNames(ms.Offline))
	}
}

func TestAnUnpluggedMemberIsNoticedFromWhatWasLearned(t *testing.T) {
	r := learned(t)
	r.sp["Playroom Speaker"].Close()
	r.broken["Playroom Speaker"] = func(w http.ResponseWriter, q *http.Request) { w.WriteHeader(http.StatusBadGateway) }
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := deviceNames(ms.Offline); got != "Playroom Speaker" {
		t.Errorf("offline = %q, configured = %s", got, deviceNames(ms.Configured))
	}
	if !strings.Contains(ms.Summary(), "cannot be reached: Playroom Speaker") {
		t.Errorf("summary = %s", ms.Summary())
	}
}

func TestWhatWasLearnedSurvivesARestartEvenWhenTheMemberIsGoneFromDiscovery(t *testing.T) {
	r := learned(t)
	r.sp["Playroom Speaker"].Close()
	host, port := r.group.Addr()
	nh, np := r.sp["Nest display"].Addr()
	kh, kp := r.sp["Kitchen speaker"].Addr()
	t.Setenv("AZAN_STATIC_DEVICES", fmt.Sprintf("athan=%s:%d;Nest display=%s:%d;Kitchen speaker=%s:%d", host, port, nh, np, kh, kp))
	ms, err := r.restart().Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := deviceNames(ms.Offline); got != "Playroom Speaker" {
		t.Errorf("offline = %q", got)
	}
}

func TestAMemberThatComesBackIsNoLongerOffline(t *testing.T) {
	r := learned(t)
	playroom := r.sp["Playroom Speaker"]
	host, port := playroom.Addr()
	playroom.Close()
	r.broken["Playroom Speaker"] = func(w http.ResponseWriter, q *http.Request) { w.WriteHeader(http.StatusBadGateway) }
	if ms, _ := r.m.Membership(context.Background()); len(ms.Offline) != 1 {
		t.Fatalf("offline = %s", deviceNames(ms.Offline))
	}
	back := fakecast.New("Playroom Speaker")
	if err := back.StartOnPort(port); err != nil {
		t.Skipf("port %s:%d not free again: %v", host, port, err)
	}
	t.Cleanup(back.Close)
	delete(r.broken, "Playroom Speaker")
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.Offline) != 0 {
		t.Errorf("offline = %s", deviceNames(ms.Offline))
	}
}

func TestAMemberTakenOutOfTheGroupIsForgotten(t *testing.T) {
	// Its own page answers and no longer lists the group: that is a decision, not a fault.
	r := learned(t)
	r.groups["Playroom Speaker"] = []string{"Upstairs"}
	r.sp["Playroom Speaker"].LeaveGroup(r.group)
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.Offline) != 0 {
		t.Errorf("offline = %s", deviceNames(ms.Offline))
	}
	// Unplugging it later must not raise anything.
	r.sp["Playroom Speaker"].Close()
	r.broken["Playroom Speaker"] = func(w http.ResponseWriter, q *http.Request) { w.WriteHeader(http.StatusBadGateway) }
	if ms, _ := r.m.Membership(context.Background()); len(ms.Offline) != 0 {
		t.Errorf("a member taken out of the group was reported offline: %s", deviceNames(ms.Offline))
	}
}

func TestAMemberWhosePageFailsButThatAnswersIsNotOffline(t *testing.T) {
	r := learned(t)
	r.broken["Playroom Speaker"] = func(w http.ResponseWriter, q *http.Request) { w.WriteHeader(http.StatusForbidden) }
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.Offline) != 0 {
		t.Errorf("offline = %s", deviceNames(ms.Offline))
	}
}

func TestAMemberGoneForAMonthIsForgotten(t *testing.T) {
	// A speaker given away can never say it left; it must not warn at every prayer forever.
	r := learned(t)
	r.sp["Playroom Speaker"].Close()
	r.broken["Playroom Speaker"] = func(w http.ResponseWriter, q *http.Request) { w.WriteHeader(http.StatusBadGateway) }
	old := memberForgetAfter
	memberForgetAfter = 0
	t.Cleanup(func() { memberForgetAfter = old })
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.Offline) != 0 {
		t.Errorf("offline = %s", deviceNames(ms.Offline))
	}
}

func TestADamagedMemoryFileIsIgnored(t *testing.T) {
	r := newMemberRig(t, "Nest display")
	r.groups["Nest display"] = []string{"athan"}
	r.sp["Nest display"].JoinGroup(r.group)
	if err := os.WriteFile(filepath.Join(r.dir, knownMembersFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := r.m.Membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if deviceNames(ms.Joined) != "Nest display" || len(ms.Offline) != 0 {
		t.Errorf("%+v", ms)
	}
	if _, err := os.Stat(filepath.Join(r.dir, knownMembersFile)); err != nil {
		t.Error(err)
	}
}
