package timesync

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

var ntpServers = []string{
	"pool.ntp.org",
	"0.pool.ntp.org",
	"1.pool.ntp.org",
	"time.google.com",
	"time.cloudflare.com",
}

// ntpEpochDelta is the number of seconds between the NTP epoch (Jan 1, 1900)
// and the Unix epoch (Jan 1, 1970).
const ntpEpochDelta = 2208988800

// parseNTPTransmitTimestamp extracts the transmit timestamp (bytes 40-43 of
// a 48-byte NTP response) and converts it from the NTP epoch to a Unix time.
func parseNTPTransmitTimestamp(resp []byte) time.Time {
	secs := binary.BigEndian.Uint32(resp[40:44])
	return time.Unix(int64(secs)-ntpEpochDelta, 0)
}

type worldTimeResp struct {
	Datetime string `json:"datetime"`
}

type timeAPIResp struct {
	DateTime string `json:"dateTime"`
}

// parseWorldTimeDatetime parses the "datetime" field of a worldtimeapi.org
// response, which is a full RFC3339 timestamp including a UTC offset (e.g.
// "2023-01-01T12:30:45.123456+00:00").
func parseWorldTimeDatetime(datetime string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, datetime)
}

// parseWorldTimeResp decodes a worldtimeapi.org response body and returns
// the parsed time.
func parseWorldTimeResp(body io.Reader) (time.Time, error) {
	var r worldTimeResp
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		return time.Time{}, err
	}
	if r.Datetime == "" {
		return time.Time{}, fmt.Errorf("empty datetime")
	}
	return parseWorldTimeDatetime(r.Datetime)
}

// parseTimeAPIResp decodes a timeapi.io response body and returns the
// parsed time.
func parseTimeAPIResp(body io.Reader) (time.Time, error) {
	var r timeAPIResp
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		return time.Time{}, err
	}
	if r.DateTime == "" {
		return time.Time{}, fmt.Errorf("empty dateTime")
	}
	return time.Parse("2006-01-02T15:04:05.9999999", r.DateTime)
}

// GetNTPTime queries a single NTP server and returns the current time.
func GetNTPTime(server string, timeout time.Duration) (time.Time, error) {
	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:123", server), timeout)
	if err != nil {
		return time.Time{}, fmt.Errorf("dial %s: %w", server, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	// NTP request: LI=0, VN=3, Mode=3 (client)
	req := make([]byte, 48)
	req[0] = 0x1b

	if _, err := conn.Write(req); err != nil {
		return time.Time{}, fmt.Errorf("write: %w", err)
	}

	resp := make([]byte, 48)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return time.Time{}, fmt.Errorf("read: %w", err)
	}

	return parseNTPTransmitTimestamp(resp), nil
}

// GetHTTPTime tries world time APIs as fallback.
func GetHTTPTime() (time.Time, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	// Try worldtimeapi.org
	if resp, err := client.Get("http://worldtimeapi.org/api/timezone/Europe/Dublin"); err == nil {
		defer resp.Body.Close()
		if t, err := parseWorldTimeResp(resp.Body); err == nil {
			return t, nil
		}
	}

	// Try timeapi.io
	if resp, err := client.Get("https://timeapi.io/api/Time/current/zone?timeZone=Europe/Dublin"); err == nil {
		defer resp.Body.Close()
		if t, err := parseTimeAPIResp(resp.Body); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("all HTTP time APIs failed")
}

// Sync tries all NTP servers then HTTP fallback, logs the result.
func Sync() (time.Time, error) {
	for _, srv := range ntpServers {
		t, err := GetNTPTime(srv, 10*time.Second)
		if err == nil {
			log.Printf("[timesync] synced via NTP server %s: %s", srv, t.Format(time.RFC3339))
			return t, nil
		}
		log.Printf("[timesync] NTP %s failed: %v", srv, err)
	}

	t, err := GetHTTPTime()
	if err == nil {
		log.Printf("[timesync] synced via HTTP fallback: %s", t.Format(time.RFC3339))
		return t, nil
	}

	return time.Time{}, fmt.Errorf("all time sync methods failed")
}
