package prayer

import (
	"testing"
	"time"
)

func TestTheJournalSessionStartsAheadOfThePrayerButNeverInThePast(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delay time.Duration
		want  time.Duration
	}{
		{"hours ahead", 3 * time.Hour, 3*time.Hour - journalLeadTime},
		{"just over the lead", journalLeadTime + time.Second, time.Second},
		{"inside the lead (agent started late)", 20 * time.Second, 0},
		{"exactly the lead", journalLeadTime, 0},
	} {
		if got := sessionStartDelay(tc.delay); got != tc.want {
			t.Errorf("%s: delay %v → %v, want %v", tc.name, tc.delay, got, tc.want)
		}
	}
}
