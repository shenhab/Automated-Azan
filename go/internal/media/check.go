package media

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// requiredAthans are the files the Athan falls back to when none is configured
// for a prayer; if one is missing everywhere, that prayer cannot play.
var requiredAthans = []string{"media_Athan.mp3", "media_adhan_al_fajr.mp3"}

// Status describes where the agent's audio comes from.
type Status struct {
	Dir       string
	DirExists bool
	Local     []string // audio files in Dir (the user's own)
	Bundled   []string // audio files built into the program
	Missing   []string // required Athan files found in neither place
}

func isAudio(name string) bool {
	return strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".m4a")
}

// Check looks at the optional media directory and the bundled audio (the
// "embedded" directory of embedded). The program serves a requested file from the
// directory if it is there, and from the bundle otherwise, so a missing
// directory is normal and only a file missing from both matters.
func Check(dir string, embedded fs.FS) Status {
	st := Status{Dir: dir}

	have := map[string]bool{}
	if entries, err := fs.ReadDir(embedded, "embedded"); err == nil {
		for _, e := range entries {
			if !e.IsDir() && isAudio(e.Name()) {
				st.Bundled = append(st.Bundled, e.Name())
				have[e.Name()] = true
			}
		}
	}
	if entries, err := os.ReadDir(dir); err == nil {
		st.DirExists = true
		for _, e := range entries {
			if !e.IsDir() && isAudio(e.Name()) {
				st.Local = append(st.Local, e.Name())
				have[e.Name()] = true
			}
		}
	}
	sort.Strings(st.Bundled)
	sort.Strings(st.Local)

	for _, n := range requiredAthans {
		if !have[n] {
			st.Missing = append(st.Missing, n)
		}
	}
	return st
}

// Problem returns a sentence describing why Athan playback would fail, or "" if
// nothing is wrong.
func (s Status) Problem() string {
	if len(s.Missing) == 0 {
		return ""
	}
	return fmt.Sprintf("these Athan audio files are in neither %s nor the program itself: %s — those prayers cannot play",
		s.Dir, strings.Join(s.Missing, ", "))
}
