//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"
)

const (
	testUser = "admin"
	testPass = "correct-horse-battery"
)

// client is a minimal browser stand-in: it keeps the session cookie and does
// not follow redirects, so tests can see them.
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, port int) *client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &client{
		t:    t,
		base: fmt.Sprintf("http://127.0.0.1:%d", port),
		http: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// waitUntilUp blocks until the web server answers.
func (c *client) waitUntilUp() {
	c.t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for {
		resp, err := c.http.Get(c.base + "/login")
		if err == nil {
			resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			c.t.Fatalf("web server never came up: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// login posts the login form and reports the status and whether a session
// cookie was issued.
func (c *client) login(user, pass string) (status int, gotCookie bool) {
	c.t.Helper()
	resp, err := c.http.PostForm(c.base+"/login", url.Values{"username": {user}, "password": {pass}})
	if err != nil {
		c.t.Fatal(err)
	}
	resp.Body.Close()
	u, _ := url.Parse(c.base)
	return resp.StatusCode, len(c.http.Jar.Cookies(u)) > 0
}

func (c *client) mustLogin() {
	c.t.Helper()
	c.waitUntilUp()
	if status, ok := c.login(testUser, testPass); status != http.StatusSeeOther || !ok {
		c.t.Fatalf("login failed: status %d, cookie %v", status, ok)
	}
}

// do sends a request and returns status, headers and body.
func (c *client) do(method, path string, body io.Reader, contentType string, hdr map[string]string) (int, http.Header, []byte) {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func (c *client) get(path string) (int, http.Header, []byte) {
	c.t.Helper()
	return c.do("GET", path, nil, "", nil)
}

// getJSON GETs path and decodes the JSON body into v.
func (c *client) getJSON(path string, v any) int {
	c.t.Helper()
	status, _, b := c.get(path)
	if err := json.Unmarshal(b, v); err != nil {
		c.t.Fatalf("GET %s: not JSON (%d): %.200s", path, status, b)
	}
	return status
}

// postJSON POSTs body as JSON and decodes the reply into v (if non-nil).
func (c *client) postJSON(path string, body, v any) int {
	c.t.Helper()
	b, _ := json.Marshal(body)
	status, _, rb := c.do("POST", path, bytes.NewReader(b), "application/json", nil)
	if v != nil {
		if err := json.Unmarshal(rb, v); err != nil {
			c.t.Fatalf("POST %s: not JSON (%d): %.200s", path, status, rb)
		}
	}
	return status
}
