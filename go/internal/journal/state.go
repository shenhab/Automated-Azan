package journal

import (
	"encoding/json"
	"fmt"
	"time"

	"azan-agent/internal/castrecord"
	"azan-agent/internal/trace"
)

// change is one thing a device's own reports say has changed.
type change struct {
	At     time.Time
	Kind   string // initial-state | app-launched | app-closed | app-session | player-state | volume
	Text   string
	Fields map[string]any
}

func (c change) event(device, role string) trace.Event {
	f := map[string]any{"role": role}
	for k, v := range c.Fields {
		f[k] = v
	}
	return trace.Event{At: c.At, Source: trace.SourceDevice, Kind: c.Kind, Device: device, Text: c.Text, Fields: f}
}

// tracker follows a device by reading the raw Cast frames it sent, not the Cast
// library's cache (which keeps the old app and media after the app has closed).
type tracker struct {
	seeded   bool
	appID    string
	appName  string
	appSess  string // the app's sessionId: a group's members show the group's
	hasApp   bool
	hasMedia bool
	session  int
	state    string
	reason   string
	level    float64
	muted    bool
	control  string
}

func newTracker() *tracker { return &tracker{} }

type receiverStatus struct {
	Status struct {
		Applications []struct {
			AppID        string `json:"appId"`
			DisplayName  string `json:"displayName"`
			IsIdleScreen bool   `json:"isIdleScreen"`
			SessionID    string `json:"sessionId"`
		} `json:"applications"`
		Volume struct {
			Level       float64 `json:"level"`
			Muted       bool    `json:"muted"`
			ControlType string  `json:"controlType"`
		} `json:"volume"`
	} `json:"status"`
}

type mediaStatus struct {
	Status []struct {
		MediaSessionID int     `json:"mediaSessionId"`
		PlayerState    string  `json:"playerState"`
		IdleReason     string  `json:"idleReason"`
		CurrentTime    float64 `json:"currentTime"`
	} `json:"status"`
}

// Feed consumes one frame and returns what changed because of it.
func (t *tracker) Feed(f castrecord.Frame) []change {
	if f.Dir != castrecord.In {
		return nil
	}
	switch f.Type {
	case "RECEIVER_STATUS":
		var rs receiverStatus
		if json.Unmarshal(f.Payload, &rs) != nil {
			return nil
		}
		return t.receiver(f.At, rs)
	case "MEDIA_STATUS":
		var ms mediaStatus
		if json.Unmarshal(f.Payload, &ms) != nil || len(ms.Status) == 0 {
			return nil
		}
		m := ms.Status[len(ms.Status)-1]
		if t.hasMedia && t.session == m.MediaSessionID && t.state == m.PlayerState && t.reason == m.IdleReason {
			return nil
		}
		t.hasMedia, t.session, t.state, t.reason = true, m.MediaSessionID, m.PlayerState, m.IdleReason
		text := m.PlayerState
		if m.IdleReason != "" {
			text += " (" + m.IdleReason + ")"
		}
		return []change{{At: f.At, Kind: "player-state", Text: text, Fields: map[string]any{
			"state": m.PlayerState, "idle_reason": m.IdleReason, "session": m.MediaSessionID, "position": m.CurrentTime,
		}}}
	}
	return nil
}

func (t *tracker) receiver(at time.Time, rs receiverStatus) []change {
	var out []change
	var appID, appName, appSess string
	hasApp := len(rs.Status.Applications) > 0
	if hasApp {
		a := rs.Status.Applications[len(rs.Status.Applications)-1]
		appID, appName, appSess = a.AppID, a.DisplayName, a.SessionID
	}
	v := rs.Status.Volume

	if !t.seeded {
		t.seeded = true
		t.appID, t.appName, t.appSess, t.hasApp = appID, appName, appSess, hasApp
		t.level, t.muted, t.control = v.Level, v.Muted, v.ControlType
		what := "no app running"
		if hasApp {
			what = "app " + appName
		}
		return []change{{At: at, Kind: "initial-state", Text: fmt.Sprintf("%s, volume %.2f%s", what, v.Level, mutedSuffix(v.Muted)),
			Fields: map[string]any{"app": appName, "app_session": appSess, "level": v.Level, "muted": v.Muted, "control_type": v.ControlType}}}
	}

	if hasApp != t.hasApp || appID != t.appID {
		if t.hasApp {
			out = append(out, change{At: at, Kind: "app-closed", Text: "app closed: " + t.appName, Fields: map[string]any{"app": t.appName}})
		}
		if hasApp {
			out = append(out, change{At: at, Kind: "app-launched", Text: "app launched: " + appName, Fields: map[string]any{"app": appName, "app_session": appSess}})
		}
		t.hasMedia, t.state, t.reason, t.session = false, "", "", 0 // a new app instance starts from nothing
		t.appID, t.appName, t.appSess, t.hasApp = appID, appName, appSess, hasApp
	} else if hasApp && appSess != t.appSess {
		out = append(out, change{At: at, Kind: "app-session", Text: "new session of " + appName, Fields: map[string]any{"app": appName, "app_session": appSess}})
		t.appSess = appSess
	}

	if v.Level != t.level || v.Muted != t.muted {
		var text string
		switch {
		case v.Level != t.level:
			text = fmt.Sprintf("volume %.2f → %.2f%s", t.level, v.Level, mutedSuffix(v.Muted))
		case v.Muted:
			text = "muted"
		default:
			text = "unmuted"
		}
		out = append(out, change{At: at, Kind: "volume", Text: text, Fields: map[string]any{
			"level": v.Level, "muted": v.Muted, "prev_level": t.level, "prev_muted": t.muted, "control_type": v.ControlType,
		}})
		t.level, t.muted = v.Level, v.Muted
	}
	return out
}

func mutedSuffix(m bool) string {
	if m {
		return " (muted)"
	}
	return ""
}
