//go:build cgo

package localplay

import (
	"fmt"
	"log"
	"net/http"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

// StartStream fetches url as an HTTP audio stream and plays it on the local
// audio device using the embedded pure-Go oto + go-mp3 stack.
//
// Falls back to external CLI tools (mpg123, ffplay, cvlc) when no audio
// device is available — e.g. in a headless Docker container.
//
// The returned stop function immediately halts playback and closes the
// HTTP connection; it is safe to call multiple times.
func StartStream(url string) (stop func(), err error) {
	stopFn, err := startStreamOto(url)
	if err != nil {
		log.Printf("[localplay] oto unavailable (%v) — trying external player", err)
		return startStreamExternal(url)
	}
	return stopFn, nil
}

func startStreamOto(url string) (func(), error) {
	// Open the HTTP stream. We use a plain GET so the connection stays open
	// as long as the stream is playing.
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("stream fetch: %w", err)
	}

	// Wrap the response body in an MP3 decoder. NewDecoder reads enough
	// header bytes to determine sample rate and channel count.
	dec, err := mp3.NewDecoder(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("mp3 init: %w", err)
	}

	// Initialise the audio output at the stream's native sample rate.
	// go-mp3 always decodes to stereo int16, so those values are fixed.
	otoCtx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   dec.SampleRate(),
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("audio device: %w", err)
	}
	<-ready // wait until the audio device is ready to accept data

	player := otoCtx.NewPlayer(dec)
	player.Play()

	var stopped bool
	return func() {
		if stopped {
			return
		}
		stopped = true
		player.Pause()
		player.Close()
		resp.Body.Close() // abort the HTTP connection, unblocking the decoder
		_ = otoCtx        // keep reference alive until stop is called
	}, nil
}
