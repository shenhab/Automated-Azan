package prayer

import "testing"

func TestParseHHMM(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantHour   int
		wantMinute int
		wantErr    bool
	}{
		// valid
		{"valid mid morning", "05:30", 5, 30, false},
		{"valid end of day", "23:59", 23, 59, false},
		{"valid midnight", "00:00", 0, 0, false},

		// malformed
		{"non-numeric", "abc", 0, 0, true},
		{"empty string", "", 0, 0, true},
		{"wrong separator", "5-30", 0, 0, true},

		// out of range
		{"hour out of range", "24:00", 0, 0, true},
		{"minute out of range", "12:60", 0, 0, true},
		{"negative hour", "-1:30", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHour, gotMinute, err := parseHHMM(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseHHMM(%q) = (%d, %d, nil); want error", tt.input, gotHour, gotMinute)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHHMM(%q) returned unexpected error: %v", tt.input, err)
			}
			if gotHour != tt.wantHour || gotMinute != tt.wantMinute {
				t.Errorf("parseHHMM(%q) = (%d, %d); want (%d, %d)", tt.input, gotHour, gotMinute, tt.wantHour, tt.wantMinute)
			}
		})
	}
}
