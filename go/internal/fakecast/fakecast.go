// Package fakecast is an in-process fake Google Cast receiver (a "speaker")
// used to test the agent end to end without real hardware.
//
// It speaks the real CASTV2 wire protocol — TLS, 4-byte length framing,
// protobuf CastMessage with JSON payloads — and is faithful enough that the
// agent's real client library (github.com/vishen/go-chromecast) connects to it,
// launches the Default Media Receiver and loads media exactly as it does against
// a real device. Like a real speaker it then downloads the media URL it was
// given, so a test can prove the URL is reachable and serves audio.
//
// Everything the speaker receives is recorded and can be inspected by tests.
// This package is test support only; the agent binary does not import it.
package fakecast

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	pb "github.com/vishen/go-chromecast/cast/proto"
)

const (
	nsConnection = "urn:x-cast:com.google.cast.tp.connection"
	nsHeartbeat  = "urn:x-cast:com.google.cast.tp.heartbeat"
	nsReceiver   = "urn:x-cast:com.google.cast.receiver"
	nsMedia      = "urn:x-cast:com.google.cast.media"

	receiverID     = "receiver-0"
	defaultMediaID = "CC1AD845" // Default Media Receiver app id
)

// FetchResult describes the speaker's download of a loaded media URL.
type FetchResult struct {
	Done           bool
	Status         int    // HTTP status, 0 if the request failed
	ContentType    string // Content-Type response header
	Bytes          int64  // body size
	LooksLikeAudio bool   // body starts with an ID3 tag or an MPEG frame sync
	Err            string
}

// Load is one LOAD command received on the media channel.
type Load struct {
	At          time.Time
	URL         string
	ContentType string
	StreamType  string
	Autoplay    bool
	Fetch       FetchResult
}

// Speaker is a fake Cast receiver.
type Speaker struct {
	Name string

	mu          sync.Mutex
	ln          net.Listener
	conns       map[net.Conn]struct{}
	connections int // total accepted
	closed      bool

	trace        []string
	loads        []Load
	appLaunched  bool
	sessionID    string
	mediaSession int
	playerState  string
	currentURL   string
	currentType  string
	volume       float64
	muted        bool

	wg sync.WaitGroup
}

// New returns an unstarted speaker with the given friendly name.
func New(name string) *Speaker {
	return &Speaker{
		Name:        name,
		conns:       make(map[net.Conn]struct{}),
		volume:      0.5,
		playerState: "IDLE",
	}
}

// Start listens on a free loopback port.
func (s *Speaker) Start() error { return s.StartOnPort(0) }

// StartOnPort listens on the given loopback port (0 = any free port). Tests use
// it to bring a speaker back on the same address after it "went offline".
func (s *Speaker) StartOnPort(port int) error {
	cert, err := selfSignedCert()
	if err != nil {
		return err
	}
	ln, err := tls.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	s.wg.Add(1)
	go s.acceptLoop(ln)
	return nil
}

// Addr returns the host and port the speaker listens on.
func (s *Speaker) Addr() (string, int) {
	a := s.ln.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port
}

// Close stops the speaker and drops all connections.
func (s *Speaker) Close() {
	s.mu.Lock()
	s.closed = true
	ln := s.ln
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	if ln != nil {
		ln.Close()
	}
	s.wg.Wait()
}

// DropConnections closes every open client connection (the listener stays up),
// to simulate a speaker that rebooted or lost Wi-Fi.
func (s *Speaker) DropConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		c.Close()
	}
}

// Loads returns a copy of the LOAD commands received so far.
func (s *Speaker) Loads() []Load {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Load(nil), s.loads...)
}

// Trace returns the protocol messages seen, as "namespace TYPE" strings.
func (s *Speaker) Trace() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.trace...)
}

// Connections returns how many client connections have been accepted.
func (s *Speaker) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

// Volume returns the current receiver volume and mute state.
func (s *Speaker) Volume() (level float64, muted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.volume, s.muted
}

// PlayerState returns the media player state (IDLE, PLAYING, PAUSED).
func (s *Speaker) PlayerState() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.playerState
}

// WaitForLoads blocks until at least n LOAD commands have arrived and their
// media downloads have finished, or the timeout expires.
func (s *Speaker) WaitForLoads(n int, timeout time.Duration) ([]Load, error) {
	deadline := time.Now().Add(timeout)
	for {
		loads := s.Loads()
		if len(loads) >= n {
			done := true
			for _, l := range loads[:n] {
				if !l.Fetch.Done {
					done = false
				}
			}
			if done {
				return loads, nil
			}
		}
		if time.Now().After(deadline) {
			return loads, fmt.Errorf("timed out after %v waiting for %d load(s); got %d (trace: %v)",
				timeout, n, len(loads), s.Trace())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Speaker) acceptLoop(ln net.Listener) {
	defer s.wg.Done()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			return
		}
		s.conns[c] = struct{}{}
		s.connections++
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serve(c)
			s.mu.Lock()
			delete(s.conns, c)
			s.mu.Unlock()
		}()
	}
}

type request struct {
	Type      string `json:"type"`
	RequestID int    `json:"requestId"`
	AppID     string `json:"appId"`
	Volume    *struct {
		Level *float64 `json:"level"`
		Muted *bool    `json:"muted"`
	} `json:"volume"`
	Media struct {
		ContentID   string `json:"contentId"`
		ContentType string `json:"contentType"`
		StreamType  string `json:"streamType"`
	} `json:"media"`
	Autoplay bool `json:"autoplay"`
}

func (s *Speaker) serve(c net.Conn) {
	defer c.Close()
	var wmu sync.Mutex
	send := func(ns, src, dst string, payload any) {
		body, _ := json.Marshal(payload)
		msg := &pb.CastMessage{
			ProtocolVersion: pb.CastMessage_CASTV2_1_0.Enum(),
			SourceId:        &src,
			DestinationId:   &dst,
			Namespace:       &ns,
			PayloadType:     pb.CastMessage_STRING.Enum(),
			PayloadUtf8:     proto.String(string(body)),
		}
		data, err := proto.Marshal(msg)
		if err != nil {
			return
		}
		frame := make([]byte, 4+len(data))
		binary.BigEndian.PutUint32(frame, uint32(len(data)))
		copy(frame[4:], data)
		wmu.Lock()
		defer wmu.Unlock()
		c.Write(frame)
	}

	for {
		var hdr [4]byte
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n == 0 || n > 1<<20 {
			return
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
		var m pb.CastMessage
		if proto.Unmarshal(buf, &m) != nil {
			return
		}
		var req request
		_ = json.Unmarshal([]byte(m.GetPayloadUtf8()), &req)
		s.handle(&m, &req, send)
	}
}

func shortNS(ns string) string {
	if i := strings.LastIndex(ns, "."); i >= 0 {
		return ns[i+1:]
	}
	return ns
}

func (s *Speaker) handle(m *pb.CastMessage, req *request, send func(ns, src, dst string, payload any)) {
	ns, src, dst := m.GetNamespace(), m.GetSourceId(), m.GetDestinationId()

	s.mu.Lock()
	s.trace = append(s.trace, shortNS(ns)+" "+req.Type)
	s.mu.Unlock()

	switch ns {
	case nsConnection:
		// CONNECT / CLOSE need no reply.

	case nsHeartbeat:
		if req.Type == "PING" {
			send(ns, dst, src, map[string]any{"type": "PONG"})
		}

	case nsReceiver:
		switch req.Type {
		case "GET_STATUS":
		case "LAUNCH":
			s.mu.Lock()
			if req.AppID == defaultMediaID {
				s.appLaunched = true
				s.sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
			}
			s.mu.Unlock()
		case "SET_VOLUME":
			s.mu.Lock()
			if req.Volume != nil {
				if req.Volume.Level != nil {
					s.volume = *req.Volume.Level
				}
				if req.Volume.Muted != nil {
					s.muted = *req.Volume.Muted
				}
			}
			s.mu.Unlock()
		case "STOP":
			s.mu.Lock()
			s.appLaunched = false
			s.playerState = "IDLE"
			s.mu.Unlock()
		}
		send(ns, receiverID, src, s.receiverStatus(req.RequestID))

	case nsMedia:
		switch req.Type {
		case "GET_STATUS":
		case "LOAD":
			s.onLoad(req)
		case "PAUSE":
			s.setState("PAUSED")
		case "PLAY":
			s.setState("PLAYING")
		case "STOP":
			s.setState("IDLE")
		}
		send(ns, dst, src, s.mediaStatus(req.RequestID))
	}
}

func (s *Speaker) setState(st string) {
	s.mu.Lock()
	s.playerState = st
	s.mu.Unlock()
}

func (s *Speaker) onLoad(req *request) {
	s.mu.Lock()
	s.mediaSession++
	s.playerState = "PLAYING"
	s.currentURL = req.Media.ContentID
	s.currentType = req.Media.ContentType
	idx := len(s.loads)
	s.loads = append(s.loads, Load{
		At:          time.Now(),
		URL:         req.Media.ContentID,
		ContentType: req.Media.ContentType,
		StreamType:  req.Media.StreamType,
		Autoplay:    req.Autoplay,
	})
	s.mu.Unlock()

	// Like a real speaker, download the media it was told to play.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		res := fetch(req.Media.ContentID)
		s.mu.Lock()
		s.loads[idx].Fetch = res
		s.mu.Unlock()
	}()
}

func fetch(url string) FetchResult {
	res := FetchResult{Done: true}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode
	res.ContentType = resp.Header.Get("Content-Type")
	head := make([]byte, 3)
	n, _ := io.ReadFull(resp.Body, head)
	res.Bytes = int64(n)
	if n == 3 {
		// ID3v2 tag, or MPEG audio frame sync (11 set bits).
		res.LooksLikeAudio = string(head) == "ID3" || (head[0] == 0xFF && head[1]&0xE0 == 0xE0)
	}
	rest, _ := io.Copy(io.Discard, resp.Body)
	res.Bytes += rest
	return res
}

func (s *Speaker) receiverStatus(reqID int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	apps := []any{}
	if s.appLaunched {
		apps = append(apps, map[string]any{
			"appId":        defaultMediaID,
			"displayName":  "Default Media Receiver",
			"isIdleScreen": false,
			"sessionId":    s.sessionID,
			"statusText":   "Ready To Cast",
			"transportId":  s.sessionID,
		})
	}
	return map[string]any{
		"type":      "RECEIVER_STATUS",
		"requestId": reqID,
		"status": map[string]any{
			"applications": apps,
			"volume":       map[string]any{"level": s.volume, "muted": s.muted},
		},
	}
}

func (s *Speaker) mediaStatus(reqID int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := []any{}
	if s.mediaSession > 0 {
		st := map[string]any{
			"mediaSessionId": s.mediaSession,
			"playerState":    s.playerState,
			"currentTime":    0,
			"volume":         map[string]any{"level": s.volume, "muted": s.muted},
			"media": map[string]any{
				"contentId":   s.currentURL,
				"contentType": s.currentType,
				"streamType":  "BUFFERED",
			},
		}
		if s.playerState == "IDLE" {
			st["idleReason"] = "CANCELLED"
		}
		status = append(status, st)
	}
	return map[string]any{"type": "MEDIA_STATUS", "requestId": reqID, "status": status}
}

func selfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return tls.Certificate{}, err
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "fakecast"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, errors.New("create cert: " + err.Error())
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
