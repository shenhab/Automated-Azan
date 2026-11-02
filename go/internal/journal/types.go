// Package journal records, for every Athan (and Quran / Kahf / manual play), what
// the agent did and what the speakers and TVs reported, on one timeline, and
// turns it into a verdict a person can read. It watches devices read-only (see
// internal/castrecord) and can never disturb a playback.
package journal

import "time"

// Verdicts.
const (
	VerdictOK      = "ok"
	VerdictWarning = "warning"
	VerdictFailed  = "failed"
	VerdictRunning = "running"
)

// Spec says what a session is about.
type Spec struct {
	Kind           string        `json:"kind"`   // athan | quran | kahf | manual
	Prayer         string        `json:"prayer"` // Fajr, Dhuhr, ... (athan)
	Label          string        `json:"label"`  // "Fajr Athan"
	ScheduledFor   time.Time     `json:"scheduled_for,omitempty"`
	MaxDuration    time.Duration `json:"max_duration_ns,omitempty"` // hard cap; 0 = default for the kind
	ExpectPlayback bool          `json:"expect_playback"`           // failing to start is a failure
}

// Target is a device to watch.
type Target struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Role string `json:"role"` // speaker | member | tv | extra
	// NotJoined marks a member configured into the group but absent from the
	// group's session when the recording began: the Athan will not reach it.
	NotJoined bool `json:"not_joined,omitempty"`
}

// Facts are what the analysis found out.
type Facts struct {
	Started        bool      `json:"started"`
	PlayedFrom     time.Time `json:"played_from,omitempty"`
	PlayedTo       time.Time `json:"played_to,omitempty"`
	DurationSec    float64   `json:"duration_sec,omitempty"`
	EndReason      string    `json:"end_reason,omitempty"`
	StoppedByAgent bool      `json:"stopped_by_agent,omitempty"`
	Volume         float64   `json:"volume,omitempty"`
	VolumeKnown    bool      `json:"volume_known,omitempty"`
	Muted          bool      `json:"muted,omitempty"`
	Retries        int       `json:"retries,omitempty"`
	Paused         bool      `json:"paused,omitempty"`
	AgentError     string    `json:"agent_error,omitempty"`
	External       []string  `json:"external,omitempty"` // changes the agent did not cause
	Health         []string  `json:"health,omitempty"`

	// The speaker's group. Empty for a plain speaker.
	Members            []string `json:"members,omitempty"`             // configured into the group
	MembersNotJoined   []string `json:"members_not_joined,omitempty"`  // configured, not in the group's session
	MembersPlayed      []string `json:"members_played,omitempty"`      // seen showing the group's session
	MembersUnconfirmed []string `json:"members_unconfirmed,omitempty"` // joined, never seen showing it
	MemberWarnings     []string `json:"member_warnings,omitempty"`     // e.g. a member that is muted
}

// Result is the outcome of analysing a session.
type Result struct {
	Verdict  string
	Headline string
	Facts    Facts
}

// Summary is what is stored in session.json and listed on the dashboard.
type Summary struct {
	ID       string    `json:"id"`
	Spec     Spec      `json:"spec"`
	Started  time.Time `json:"started"`
	Ended    time.Time `json:"ended,omitempty"`
	EndedBy  string    `json:"ended_by,omitempty"`
	Verdict  string    `json:"verdict"`
	Headline string    `json:"headline"`
	Facts    Facts     `json:"facts"`
	Targets  []Target  `json:"targets"`
	Events   int       `json:"events"`
	Bytes    int64     `json:"bytes,omitempty"`
	Heard    string    `json:"heard,omitempty"` // what a person said: yes | no | partly
}

// Note is what a person added to a session.
type Note struct {
	At    time.Time `json:"at"`
	Heard string    `json:"heard,omitempty"` // yes | no | partly | ""
	Text  string    `json:"text,omitempty"`
}

// DeviceLive is what a watched device is doing right now.
type DeviceLive struct {
	Name      string  `json:"name"`
	Role      string  `json:"role"`
	Reachable bool    `json:"reachable"`
	App       string  `json:"app"`
	State     string  `json:"state"` // PLAYING, BUFFERING, PAUSED, IDLE, or "" when there is no media
	Level     float64 `json:"level"`
	Muted     bool    `json:"muted"`
}

// LiveRun is a session in progress.
type LiveRun struct {
	ID       string       `json:"id"`
	Label    string       `json:"label"`
	Kind     string       `json:"kind"`
	Started  time.Time    `json:"started"`
	Headline string       `json:"headline"`
	Devices  []DeviceLive `json:"devices"`
}
