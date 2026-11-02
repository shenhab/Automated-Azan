package journal

import (
	"time"

	"azan-agent/internal/trace"
)

// Which agent actions can explain which device changes. A change that no action
// explains, while the agent is playing, is something outside: a phone, a voice
// command, another app taking the speaker over.
var explains = map[string][]string{
	"app-launched": {"load-sent", "launch"},
	"app-closed":   {"stop-sent"},
	"volume":       {"volume-set", "mute-sent", "unmute-sent"},
}

// mediaReceiverApp is the app the agent's Load launches on a speaker.
const mediaReceiverApp = "Default Media Receiver"

const (
	actionLeadSlack = time.Second      // a device's clock-free report can only follow the action; allow for event ordering
	actionWindow    = 20 * time.Second // how long after an action its effects may show
)

type agentAction struct {
	at   time.Time
	kind string
	host string
}

// attribution remembers what the agent recently asked of which device.
type attribution struct{ actions []agentAction }

func newAttribution() *attribution { return &attribution{} }

// Add records an agent event if it is something that changes a device.
func (a *attribution) Add(e trace.Event) {
	if e.Source != trace.SourceAgent && e.Source != trace.SourceTV {
		return
	}
	switch e.Kind {
	case "load-sent", "launch", "stop-sent", "volume-set", "mute-sent", "unmute-sent", "pause-sent", "play-sent":
	default:
		return
	}
	host, _ := e.Fields["host"].(string)
	a.actions = append(a.actions, agentAction{at: e.At, kind: e.Kind, host: host})
	// forget what can no longer explain anything
	keep := a.actions[:0]
	for _, x := range a.actions {
		if e.At.Sub(x.at) <= actionWindow+time.Minute {
			keep = append(keep, x)
		}
	}
	a.actions = keep
}

// Caused reports whether the change is explained by the agent (or is not a
// change someone could cause: the first look, a file reaching its end, a fault).
func (a *attribution) Caused(host string, c change) bool {
	kinds := explains[c.Kind]
	switch c.Kind {
	case "initial-state", "app-session":
		return true
	case "app-closed":
		// Our Load launches the media receiver, which closes whatever was open
		// (the idle screen, Backdrop): that is ours. The media receiver itself
		// closing is not, unless we stopped it.
		if app, _ := c.Fields["app"].(string); app != mediaReceiverApp {
			kinds = []string{"stop-sent", "load-sent", "launch"}
		}
	case "player-state":
		state, _ := c.Fields["state"].(string)
		reason, _ := c.Fields["idle_reason"].(string)
		switch {
		case state == "IDLE" && (reason == "FINISHED" || reason == "ERROR"):
			return true
		case state == "IDLE" && reason == "":
			kinds = []string{"load-sent"} // the first report of a new media session
		case state == "IDLE": // CANCELLED, INTERRUPTED
			kinds = []string{"stop-sent", "load-sent"}
		case state == "PAUSED":
			kinds = []string{"pause-sent"}
		default: // PLAYING, BUFFERING
			kinds = []string{"load-sent", "play-sent"}
		}
	}
	for _, x := range a.actions {
		if x.host != "" && host != "" && x.host != host {
			continue
		}
		if !contains(kinds, x.kind) {
			continue
		}
		if d := c.At.Sub(x.at); d >= -actionLeadSlack && d <= actionWindow {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
