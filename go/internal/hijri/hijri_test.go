package hijri

import (
	"testing"
	"time"
)

// TestValidateGregorian covers the Hijri-epoch boundary: dates before
// 622-07-16 have no meaningful Hijri conversion and must be rejected before
// any network call is attempted.
func TestValidateGregorian(t *testing.T) {
	cases := []struct {
		name    string
		date    time.Time
		wantErr bool
	}{
		{"long before epoch", time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC), true},
		{"one day before epoch", time.Date(622, time.July, 15, 0, 0, 0, 0, time.UTC), true},
		{"exactly at epoch", time.Date(622, time.July, 16, 0, 0, 0, 0, time.UTC), false},
		{"one day after epoch", time.Date(622, time.July, 17, 0, 0, 0, 0, time.UTC), false},
		{"well after epoch", time.Date(2026, time.July, 2, 0, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGregorian(tc.date)
			if tc.wantErr && err == nil {
				t.Errorf("validateGregorian(%s) = nil, want error", tc.date.Format("2006-01-02"))
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateGregorian(%s) = %v, want nil", tc.date.Format("2006-01-02"), err)
			}
		})
	}
}

// TestFetchRejectsPreEpochDate confirms fetch short-circuits on validation
// failure without attempting the Aladhan network call.
func TestFetchRejectsPreEpochDate(t *testing.T) {
	_, err := fetch(time.Date(500, time.January, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("fetch() with a pre-epoch date = nil error, want error")
	}
}
