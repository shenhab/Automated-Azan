package chromecast

import (
	"strings"
	"testing"

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
