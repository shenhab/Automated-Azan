package chromecast

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vishen/go-chromecast/cast"

	"azan-agent/internal/castrecord"
)

// GroupMember is one device of a Cast group, as the group itself lists it.
type GroupMember struct {
	DeviceID string  `json:"device_id"`
	Name     string  `json:"name"`
	Level    float64 `json:"level"` // this member's own volume, 0..1
	Muted    bool    `json:"muted"`
}

// ID is the device id without dashes, lower case: the form discovery reports.
func (g GroupMember) ID() string {
	return strings.ToLower(strings.ReplaceAll(g.DeviceID, "-", ""))
}

const (
	nsMultizone    = "urn:x-cast:com.google.cast.multizone"
	nsCastConn     = "urn:x-cast:com.google.cast.tp.connection"
	membersTimeout = 3 * time.Second
)

// GroupMembers asks a Cast group which devices are in it, and what volume each
// has. It answers from the group's own multizone status, over a connection of
// its own, and is read-only (the connection refuses anything but status
// requests). A plain device lists itself; a device that does not answer in time
// is an error. Measured live 2026-11-02: `athan` = Nest display + Kitchen speaker.
func GroupMembers(host string, port int) ([]GroupMember, error) {
	rec := castrecord.New(cast.NewConnection())
	if err := rec.Start(host, port); err != nil {
		return nil, fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}
	defer rec.Close()
	msgs := rec.MsgChan()

	if err := rec.Send(1, &cast.ConnectHeader, "sender-0", "receiver-0", nsCastConn); err != nil {
		return nil, err
	}
	if err := rec.Send(2, &cast.PayloadHeader{Type: "GET_STATUS"}, "sender-0", "receiver-0", nsMultizone); err != nil {
		return nil, err
	}
	timeout := time.After(membersTimeout)
	for {
		select {
		case m, ok := <-msgs:
			if !ok {
				return nil, fmt.Errorf("%s:%d closed the connection", host, port)
			}
			if m.GetNamespace() != nsMultizone {
				continue
			}
			var st struct {
				Type   string `json:"type"`
				Status struct {
					Devices []struct {
						DeviceID string `json:"deviceId"`
						Name     string `json:"name"`
						Volume   struct {
							Level float64 `json:"level"`
							Muted bool    `json:"muted"`
						} `json:"volume"`
					} `json:"devices"`
				} `json:"status"`
			}
			if json.Unmarshal([]byte(m.GetPayloadUtf8()), &st) != nil || st.Type != "MULTIZONE_STATUS" {
				continue
			}
			out := make([]GroupMember, 0, len(st.Status.Devices))
			for _, d := range st.Status.Devices {
				out = append(out, GroupMember{DeviceID: d.DeviceID, Name: d.Name, Level: d.Volume.Level, Muted: d.Volume.Muted})
			}
			return out, nil
		case <-timeout:
			return nil, fmt.Errorf("%s:%d did not list its members within %v", host, port, membersTimeout)
		}
	}
}

// MatchMembers finds the discovered device for each member: by device id, and
// failing that by name (static devices have no real id). Members that are not
// on the network are returned as missing.
func MatchMembers(members []GroupMember, devices []Device) (found []Device, missing []GroupMember) {
	for _, mb := range members {
		var hit *Device
		for i := range devices {
			if normID(devices[i].UUID) == mb.ID() {
				hit = &devices[i]
				break
			}
		}
		if hit == nil {
			for i := range devices {
				if strings.EqualFold(devices[i].Name, mb.Name) {
					hit = &devices[i]
					break
				}
			}
		}
		if hit == nil {
			missing = append(missing, mb)
			continue
		}
		found = append(found, *hit)
	}
	return found, missing
}

func normID(s string) string { return strings.ToLower(strings.ReplaceAll(s, "-", "")) }
