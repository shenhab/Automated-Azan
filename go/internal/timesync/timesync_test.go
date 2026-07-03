package timesync

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestParseNTPTransmitTimestamp(t *testing.T) {
	// Unix time 2023-01-01T00:00:00Z, expressed as NTP seconds since 1900.
	want := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
	ntpSecs := uint32(want.Unix() + ntpEpochDelta)

	resp := make([]byte, 48)
	binary.BigEndian.PutUint32(resp[40:44], ntpSecs)

	got := parseNTPTransmitTimestamp(resp)

	if !got.UTC().Equal(want) {
		t.Fatalf("parseNTPTransmitTimestamp() = %v, want %v", got.UTC(), want)
	}
}

func TestParseNTPTransmitTimestampEpoch(t *testing.T) {
	// An NTP timestamp equal to the epoch delta itself corresponds to the
	// Unix epoch (1970-01-01T00:00:00Z).
	resp := make([]byte, 48)
	binary.BigEndian.PutUint32(resp[40:44], ntpEpochDelta)

	got := parseNTPTransmitTimestamp(resp)

	want := time.Unix(0, 0).UTC()
	if !got.UTC().Equal(want) {
		t.Fatalf("parseNTPTransmitTimestamp() = %v, want %v", got.UTC(), want)
	}
}

func TestParseWorldTimeResp(t *testing.T) {
	// Realistic worldtimeapi.org response body, including a UTC offset.
	body := strings.NewReader(`{"datetime":"2024-03-15T08:15:30.500000+00:00"}`)

	got, err := parseWorldTimeResp(body)
	if err != nil {
		t.Fatalf("parseWorldTimeResp() error = %v", err)
	}

	want := time.Date(2024, time.March, 15, 8, 15, 30, 500000000, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseWorldTimeResp() = %v, want %v", got, want)
	}
}

func TestParseWorldTimeDatetime(t *testing.T) {
	tests := []struct {
		name     string
		datetime string
		want     time.Time
	}{
		{
			name:     "zero UTC offset",
			datetime: "2023-01-01T12:30:45.123456+00:00",
			want:     time.Date(2023, time.January, 1, 12, 30, 45, 123456000, time.UTC),
		},
		{
			name:     "non-zero UTC offset",
			datetime: "2023-06-15T08:00:00.123456+01:00",
			want:     time.Date(2023, time.June, 15, 8, 0, 0, 123456000, time.FixedZone("", 3600)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWorldTimeDatetime(tt.datetime)
			if err != nil {
				t.Fatalf("parseWorldTimeDatetime(%q) error = %v", tt.datetime, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("parseWorldTimeDatetime(%q) = %v, want %v", tt.datetime, got, tt.want)
			}
		})
	}
}

func TestParseWorldTimeRespMissingField(t *testing.T) {
	body := strings.NewReader(`{}`)

	if _, err := parseWorldTimeResp(body); err == nil {
		t.Fatal("parseWorldTimeResp() error = nil, want error for missing datetime")
	}
}

func TestParseTimeAPIResp(t *testing.T) {
	body := strings.NewReader(`{"dateTime":"2024-03-15T08:15:30.5000000"}`)

	got, err := parseTimeAPIResp(body)
	if err != nil {
		t.Fatalf("parseTimeAPIResp() error = %v", err)
	}

	want := time.Date(2024, time.March, 15, 8, 15, 30, 500000000, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseTimeAPIResp() = %v, want %v", got, want)
	}
}

func TestParseTimeAPIRespMissingField(t *testing.T) {
	body := strings.NewReader(`{}`)

	if _, err := parseTimeAPIResp(body); err == nil {
		t.Fatal("parseTimeAPIResp() error = nil, want error for missing dateTime")
	}
}
