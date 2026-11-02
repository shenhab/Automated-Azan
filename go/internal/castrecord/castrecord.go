// Package castrecord records the raw Cast protocol exchange with a device so it can
// be replayed as test fixtures. It is READ-ONLY by construction: the connection it
// hands to the client library refuses to send anything that could change a device
// (see readOnly), so recording against a family's real speakers cannot start, stop or
// mute anything.
package castrecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/cast"
	pb "github.com/vishen/go-chromecast/cast/proto"
)

// ErrNotReadOnly is returned for any message that is not on the read-only list.
var ErrNotReadOnly = errors.New("castrecord is read-only: refusing to send a message that could change the device")

// readOnly is the complete list of message types the recorder will send:
// session housekeeping and status requests. Everything else (LAUNCH, LOAD, STOP,
// PLAY, PAUSE, SEEK, SET_VOLUME, queue commands, ...) is refused.
var readOnly = map[string]bool{
	"CONNECT":    true,
	"CLOSE":      true,
	"GET_STATUS": true,
	"PONG":       true,
}

// Direction of a frame relative to the recorder.
type Direction string

const (
	Out Direction = "out" // sent by us
	In  Direction = "in"  // received from the device
)

// Frame is one Cast message as it went over the wire.
type Frame struct {
	At        time.Time       `json:"at"`
	Dir       Direction       `json:"dir"`
	Src       string          `json:"src"`
	Dst       string          `json:"dst"`
	Namespace string          `json:"ns"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// Recorder wraps a connection, records every frame and enforces read-only.
type Recorder struct {
	inner cast.Conn
	out   chan *pb.CastMessage

	mu     sync.Mutex
	frames []Frame
	once   sync.Once
}

// New wraps inner. Pass the result to application.WithConnection.
func New(inner cast.Conn) *Recorder {
	return &Recorder{inner: inner, out: make(chan *pb.CastMessage, 64)}
}

// Frames returns a copy of everything recorded so far.
func (r *Recorder) Frames() []Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Frame(nil), r.frames...)
}

func (r *Recorder) add(f Frame) {
	r.mu.Lock()
	r.frames = append(r.frames, f)
	r.mu.Unlock()
}

func typeOf(payload []byte) string {
	var h struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(payload, &h)
	return h.Type
}

// Send records and forwards read-only messages and refuses all others.
func (r *Recorder) Send(requestID int, payload cast.Payload, src, dst, ns string) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	typ := typeOf(b)
	if !readOnly[typ] {
		return fmt.Errorf("%w (%q)", ErrNotReadOnly, typ)
	}
	r.add(Frame{At: time.Now(), Dir: Out, Src: src, Dst: dst, Namespace: ns, Type: typ, Payload: b})
	return r.inner.Send(requestID, payload, src, dst, ns)
}

// MsgChan returns the stream of device messages, recording each on the way through.
func (r *Recorder) MsgChan() chan *pb.CastMessage {
	r.once.Do(func() {
		go func() {
			defer close(r.out)
			for m := range r.inner.MsgChan() {
				body := m.GetPayloadUtf8()
				r.add(Frame{At: time.Now(), Dir: In, Src: m.GetSourceId(), Dst: m.GetDestinationId(),
					Namespace: m.GetNamespace(), Type: typeOf([]byte(body)), Payload: json.RawMessage(body)})
				r.out <- m
			}
		}()
	})
	return r.out
}

func (r *Recorder) Start(addr string, port int) error { return r.inner.Start(addr, port) }
func (r *Recorder) Close() error                      { return r.inner.Close() }
func (r *Recorder) SetDebug(d bool)                   { r.inner.SetDebug(d) }
func (r *Recorder) LocalAddr() (string, error)        { return r.inner.LocalAddr() }
func (r *Recorder) RemoteAddr() (string, error)       { return r.inner.RemoteAddr() }
func (r *Recorder) RemotePort() (string, error)       { return r.inner.RemotePort() }

var (
	ipRe   = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	uuidRe = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	hexRe  = regexp.MustCompile(`"(deviceId|device_id|id)"\s*:\s*"[0-9a-fA-F]{12,}"`)
)

// Scrub removes what identifies a household from a payload before it is committed
// as a fixture: IP addresses, UUIDs (each distinct one gets a stable token, so
// sessions stay linkable), hardware ids, and the given names (old → new).
func Scrub(s string, names map[string]string) string {
	s = ipRe.ReplaceAllString(s, "192.0.2.1")
	seen := map[string]string{}
	s = uuidRe.ReplaceAllStringFunc(s, func(u string) string {
		if t, ok := seen[u]; ok {
			return t
		}
		t := fmt.Sprintf("session-%d", len(seen)+1)
		seen[u] = t
		return t
	})
	s = hexRe.ReplaceAllString(s, `"$1":"device-id"`)
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		s = strings.ReplaceAll(s, k, names[k])
	}
	return s
}
