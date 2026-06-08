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

	// Transmit timestamp is at bytes 40-47
	secs := binary.BigEndian.Uint32(resp[40:44])
	// NTP epoch is Jan 1, 1900; Unix epoch is Jan 1, 1970
	const ntpDelta = 2208988800
	t := time.Unix(int64(secs)-ntpDelta, 0)
	return t, nil
}

// GetHTTPTime tries world time APIs as fallback.
func GetHTTPTime() (time.Time, error) {
	type worldTimeResp struct {
		Datetime string `json:"datetime"`
	}
	type timeAPIResp struct {
		DateTime string `json:"dateTime"`
	}

	client := &http.Client{Timeout: 5 * time.Second}

	// Try worldtimeapi.org
	if resp, err := client.Get("http://worldtimeapi.org/api/timezone/Europe/Dublin"); err == nil {
		defer resp.Body.Close()
		var r worldTimeResp
		if json.NewDecoder(resp.Body).Decode(&r) == nil && r.Datetime != "" {
			if t, err := time.Parse(time.RFC3339Nano, r.Datetime[:len(r.Datetime)-3]+"Z"); err == nil {
				return t, nil
			}
		}
	}

	// Try timeapi.io
	if resp, err := client.Get("https://timeapi.io/api/Time/current/zone?timeZone=Europe/Dublin"); err == nil {
		defer resp.Body.Close()
		var r timeAPIResp
		if json.NewDecoder(resp.Body).Decode(&r) == nil && r.DateTime != "" {
			if t, err := time.Parse("2006-01-02T15:04:05.9999999", r.DateTime); err == nil {
				return t, nil
			}
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
