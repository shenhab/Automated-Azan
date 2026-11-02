package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"azan-agent/internal/journal"
	"azan-agent/internal/trace"
)

// SetJournal gives the web app the Journal to show.
func (s *Server) SetJournal(j *journal.Journal) { s.journal = j }

// beginManual starts a Journal session for something a person asked the speaker to
// play from the web app, and notes who asked.
func (s *Server) beginManual(r *http.Request, label string, max time.Duration) {
	if s.journal == nil || !s.cfg.Journal.Enabled {
		return
	}
	s.journal.Begin(journal.Spec{Kind: "manual", Label: label, ExpectPlayback: true, MaxDuration: max})
	trace.From(trace.SourceSystem, "manual-action", "", label+" requested from the web app by "+realIP(r))
}

// noteManual records a manual action that is not a play (stop, release TVs).
func (s *Server) noteManual(r *http.Request, what string) {
	if s.journal == nil || !s.cfg.Journal.Enabled {
		return
	}
	trace.From(trace.SourceSystem, "manual-action", "", what+" requested from the web app by "+realIP(r))
}

const journalOff = "the Journal is switched off (Settings → Journal)"

func (s *Server) journalReady(w http.ResponseWriter) bool {
	if s.journal == nil || !s.cfg.Journal.Enabled {
		writeJSON(w, map[string]interface{}{"success": false, "error": journalOff})
		return false
	}
	return true
}

// handleJournal renders the Journal dashboard; its data comes from the API below.
func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "journal.html", map[string]interface{}{
		"page":    "journal",
		"config":  s.cfg.AsWebDict(),
		"enabled": s.journal != nil && s.cfg.Journal.Enabled,
	})
}

// JournalStats counts sessions by outcome.
type JournalStats struct {
	Total      int `json:"total"`
	OK         int `json:"ok"`
	Warning    int `json:"warning"`
	Failed     int `json:"failed"`
	Running    int `json:"running"`
	HeardYes   int `json:"heard_yes"`
	HeardNo    int `json:"heard_no"`
	HeardPart  int `json:"heard_partly"`
	Mismatches int `json:"mismatches"` // the Journal said OK but a person did not hear it (or the reverse)
}

func (s *Server) handleAPIJournalSessions(w http.ResponseWriter, r *http.Request) {
	if !s.journalReady(w) {
		return
	}
	days := 14
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 {
		days = v
	}
	if days > 365 {
		days = 365
	}
	kind, verdict, prayer := r.URL.Query().Get("kind"), r.URL.Query().Get("verdict"), r.URL.Query().Get("prayer")
	limit := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}

	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var out []journal.Summary
	var st JournalStats
	for _, sm := range s.journal.Sessions(since) {
		if kind != "" && sm.Spec.Kind != kind {
			continue
		}
		if verdict != "" && sm.Verdict != verdict {
			continue
		}
		if prayer != "" && !strings.EqualFold(sm.Spec.Prayer, prayer) && !strings.EqualFold(sm.Spec.Label, prayer) {
			continue
		}
		st.Total++
		switch sm.Verdict {
		case journal.VerdictOK:
			st.OK++
		case journal.VerdictWarning:
			st.Warning++
		case journal.VerdictFailed:
			st.Failed++
		case journal.VerdictRunning:
			st.Running++
		}
		switch sm.Heard {
		case "yes":
			st.HeardYes++
		case "no":
			st.HeardNo++
		case "partly":
			st.HeardPart++
		}
		if (sm.Heard == "no" && sm.Verdict == journal.VerdictOK) || (sm.Heard == "yes" && sm.Verdict == journal.VerdictFailed) {
			st.Mismatches++
		}
		if limit == 0 || len(out) < limit {
			out = append(out, sm)
		}
	}
	writeJSON(w, map[string]interface{}{"success": true, "sessions": out, "stats": st, "days": days})
}

func (s *Server) handleAPIJournalSession(w http.ResponseWriter, r *http.Request) {
	if !s.journalReady(w) {
		return
	}
	d, err := s.journal.Detail(r.URL.Query().Get("id"))
	if err != nil {
		writeJSON(w, errResp(err))
		return
	}
	writeJSON(w, map[string]interface{}{"success": true, "detail": d})
}

func (s *Server) handleAPIJournalLive(w http.ResponseWriter, r *http.Request) {
	if !s.journalReady(w) {
		return
	}
	live := s.journal.Live()
	if live == nil {
		live = []journal.LiveRun{}
	}
	writeJSON(w, map[string]interface{}{"success": true, "live": live})
}

// handleAPIJournalRaw downloads a device's raw Cast recording. With scrub=1 the
// addresses, ids and device names are removed so it can be shared.
func (s *Server) handleAPIJournalRaw(w http.ResponseWriter, r *http.Request) {
	if !s.journalReady(w) {
		return
	}
	q := r.URL.Query()
	scrub := q.Get("scrub") == "1"
	rc, err := s.journal.Raw(q.Get("id"), q.Get("device"), scrub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer rc.Close()
	name := q.Get("id")
	if scrub {
		name += "-scrubbed"
	} else {
		name += "-" + safeFileWord(q.Get("device"))
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.jsonl"`, name))
	io.Copy(w, rc)
}

func safeFileWord(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *Server) handleAPIJournalNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.journalReady(w) {
		return
	}
	var body struct {
		ID    string `json:"id"`
		Heard string `json:"heard"`
		Text  string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, errResp(err))
		return
	}
	if err := s.journal.AddNote(body.ID, body.Heard, strings.TrimSpace(body.Text)); err != nil {
		writeJSON(w, errResp(err))
		return
	}
	d, _ := s.journal.Detail(body.ID)
	writeJSON(w, map[string]interface{}{"success": true, "detail": d})
}

// applyJournalConfig reads the journal_* settings from an update.
func (s *Server) applyJournalConfig(body map[string]interface{}) {
	j := &s.cfg.Journal
	if v, ok := body["journal_enabled"].(bool); ok {
		j.Enabled = v
	}
	if v, ok := body["journal_retention_days"].(float64); ok {
		j.RetentionDays = int(v)
	}
	if v, ok := body["journal_max_mb"].(float64); ok {
		j.MaxMB = int(v)
	}
	if v, ok := body["journal_watch_tvs"].(bool); ok {
		j.WatchTVs = v
	}
	if v, ok := body["journal_notify_on"].(string); ok {
		j.NotifyOn = v
	}
	if v, ok := body["journal_watch_devices"].([]interface{}); ok {
		j.WatchDevices = nil
		for _, x := range v {
			if name, ok := x.(string); ok && strings.TrimSpace(name) != "" {
				j.WatchDevices = append(j.WatchDevices, strings.TrimSpace(name))
			}
		}
	}
}

func validateJournalConfig(body map[string]interface{}) error {
	if v, ok := body["journal_retention_days"].(float64); ok && (v < 1 || v > 365 || v != float64(int(v))) {
		return fmt.Errorf("the Journal keeps recordings for a whole number of days between 1 and 365")
	}
	if v, ok := body["journal_max_mb"].(float64); ok && (v < 10 || v > 5000 || v != float64(int(v))) {
		return fmt.Errorf("the Journal size limit must be a whole number of megabytes between 10 and 5000")
	}
	if v, ok := body["journal_notify_on"].(string); ok && v != "problems" && v != "failures" && v != "off" {
		return fmt.Errorf("notify on must be problems, failures or off")
	}
	return nil
}
