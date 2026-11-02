package castrecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/cast"
	pb "github.com/vishen/go-chromecast/cast/proto"

	"azan-agent/internal/fakecast"
)

// stubConn is a connection that records what reached the "device".
type stubConn struct {
	sent []string
	ch   chan *pb.CastMessage
}

func newStub() *stubConn                          { return &stubConn{ch: make(chan *pb.CastMessage, 8)} }
func (s *stubConn) Start(string, int) error       { return nil }
func (s *stubConn) MsgChan() chan *pb.CastMessage { return s.ch }
func (s *stubConn) Close() error                  { close(s.ch); return nil }
func (s *stubConn) SetDebug(bool)                 {}
func (s *stubConn) LocalAddr() (string, error)    { return "127.0.0.1", nil }
func (s *stubConn) RemoteAddr() (string, error)   { return "127.0.0.1", nil }
func (s *stubConn) RemotePort() (string, error)   { return "8009", nil }
func (s *stubConn) Send(_ int, p cast.Payload, _, _, _ string) error {
	b, _ := json.Marshal(p)
	s.sent = append(s.sent, string(b))
	return nil
}

// The recorder is used against people's real devices, so it must be read-only by
// construction: anything that could start, stop, mute or change a device is refused
// before it reaches the wire. This is the safety property, so it is tested for every
// message the client library can send.
func TestRecorderRefusesEverythingThatCouldChangeADevice(t *testing.T) {
	headers := map[string]cast.Payload{
		"LAUNCH":       &cast.LaunchHeader,
		"STOP":         &cast.StopHeader,
		"PLAY":         &cast.PlayHeader,
		"PAUSE":        &cast.PauseHeader,
		"SEEK":         &cast.SeekHeader,
		"SET_VOLUME":   &cast.VolumeHeader,
		"LOAD":         &cast.LoadHeader,
		"QUEUE_LOAD":   &cast.QueueLoadHeader,
		"QUEUE_UPDATE": &cast.QueueUpdateHeader,
		"SKIP_AD":      &cast.SkipHeader,
		"MADE_UP":      &cast.PayloadHeader{Type: "MADE_UP"},
	}
	for name, p := range headers {
		inner := newStub()
		r := New(inner)
		err := r.Send(1, p, "sender-0", "receiver-0", "urn:x-cast:com.google.cast.media")
		if !errors.Is(err, ErrNotReadOnly) {
			t.Errorf("%s: Send returned %v, want ErrNotReadOnly", name, err)
		}
		if len(inner.sent) != 0 {
			t.Errorf("%s reached the device: %v", name, inner.sent)
		}
	}
	// Reading is allowed.
	for _, p := range []cast.Payload{&cast.ConnectHeader, &cast.GetStatusHeader, &cast.PongHeader, &cast.CloseHeader} {
		inner := newStub()
		if err := New(inner).Send(1, p, "sender-0", "receiver-0", "urn:x-cast:com.google.cast.tp.connection"); err != nil {
			t.Errorf("a read-only message was refused: %v", err)
		}
		if len(inner.sent) != 1 {
			t.Errorf("a read-only message did not reach the device")
		}
	}
}

func TestRecorderCapturesFramesInBothDirectionsFromARealExchange(t *testing.T) {
	t.Setenv("AZAN_DISABLE_MDNS", "1")
	sp := fakecast.New("Rec Speaker")
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)

	host, port := sp.Addr()
	rec := New(cast.NewConnection())
	app := application.NewApplication(application.WithConnection(rec), application.WithCacheDisabled(true))
	if err := app.Start(host, port); err != nil {
		t.Fatalf("a read-only client could not even connect (the allowlist is too tight): %v", err)
	}
	t.Cleanup(func() { app.Close(false) })
	if err := app.Update(); err != nil {
		t.Fatal(err)
	}

	frames := rec.Frames()
	var sentStatus, gotReceiverStatus bool
	for _, f := range frames {
		if f.Dir == Out && f.Type == "GET_STATUS" {
			sentStatus = true
		}
		if f.Dir == In && f.Type == "RECEIVER_STATUS" {
			gotReceiverStatus = true
		}
		if f.At.IsZero() || len(f.Payload) == 0 || !json.Valid(f.Payload) {
			t.Errorf("incomplete frame: %+v", f)
		}
	}
	if !sentStatus || !gotReceiverStatus {
		t.Errorf("did not capture both directions (sent GET_STATUS=%v, got RECEIVER_STATUS=%v): %d frames", sentStatus, gotReceiverStatus, len(frames))
	}
	for _, f := range frames {
		if f.Dir == Out && !readOnly[f.Type] {
			t.Errorf("recorded an outgoing %s", f.Type)
		}
	}
}

func TestScrubRemovesWhatIdentifiesAHousehold(t *testing.T) {
	in := `{"type":"RECEIVER_STATUS","status":{"applications":[{"displayName":"Default Media Receiver",` +
		`"sessionId":"7c0d6a64-3f5e-4f0c-9c88-0d7d3b0a1111","transportId":"7c0d6a64-3f5e-4f0c-9c88-0d7d3b0a1111",` +
		`"statusText":"Casting to Kitchen speaker"}]},"media":{"contentId":"http://192.168.86.5:8080/media/a.mp3"},"deviceId":"44cb6c7f1a2b3c4d"}`
	out := Scrub(in, map[string]string{"Kitchen speaker": "Speaker A", "athan": "Group A"})
	for _, leak := range []string{"192.168.86.5", "Kitchen", "7c0d6a64-3f5e-4f0c-9c88-0d7d3b0a1111"} {
		if strings.Contains(out, leak) {
			t.Errorf("scrubbed output still contains %q: %s", leak, out)
		}
	}
	if !json.Valid([]byte(out)) {
		t.Errorf("scrubbing broke the JSON: %s", out)
	}
	// What the contract tests depend on must survive.
	for _, keep := range []string{`"type":"RECEIVER_STATUS"`, "Default Media Receiver", "/media/a.mp3"} {
		if !strings.Contains(out, keep) {
			t.Errorf("scrubbing removed %q: %s", keep, out)
		}
	}
	// The same id is replaced by the same token so sessions stay linkable.
	if strings.Count(out, "session-1") != 2 {
		t.Errorf("one session id should map to one token: %s", out)
	}
}

func TestFramesSinceReturnsOnlyTheNewFrames(t *testing.T) {
	r := New(nil)
	for i := 0; i < 5; i++ {
		r.add(Frame{Type: fmt.Sprint(i)})
	}
	if got := r.FramesSince(0); len(got) != 5 {
		t.Errorf("since 0 = %d frames", len(got))
	}
	got := r.FramesSince(3)
	if len(got) != 2 || got[0].Type != "3" || got[1].Type != "4" {
		t.Errorf("since 3 = %+v", got)
	}
	if r.FramesSince(5) != nil || r.FramesSince(99) != nil {
		t.Error("nothing new should return nil")
	}
}
