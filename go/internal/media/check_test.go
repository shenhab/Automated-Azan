package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// Audio comes from two places: an optional Media directory next to the program
// (for the user's own files), and the files bundled inside it. At startup the
// agent must only complain when playback would really fail — when a file the
// Athan needs exists in neither — and not merely because there is no directory.

func bundled(names ...string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, n := range names {
		fsys["embedded/"+n] = &fstest.MapFile{Data: []byte("ID3")}
	}
	return fsys
}

func TestNoMediaDirectoryIsFineWhenTheAudioIsBundled(t *testing.T) {
	st := Check(filepath.Join(t.TempDir(), "does-not-exist"), bundled("media_Athan.mp3", "media_adhan_al_fajr.mp3", "kahf.mp3"))
	if st.DirExists {
		t.Error("reports a directory that does not exist")
	}
	if len(st.Missing) != 0 {
		t.Errorf("complains that %v is missing although it is bundled", st.Missing)
	}
	if len(st.Bundled) != 3 {
		t.Errorf("bundled files = %v, want 3", st.Bundled)
	}
}

func TestOwnFilesInTheMediaDirectoryAreListed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "my-athan.mp3"), []byte("ID3"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	st := Check(dir, bundled("media_Athan.mp3", "media_adhan_al_fajr.mp3"))
	if !st.DirExists || len(st.Local) != 1 || st.Local[0] != "my-athan.mp3" {
		t.Errorf("local files = %v (dir exists %v), want just my-athan.mp3", st.Local, st.DirExists)
	}
	if len(st.Missing) != 0 {
		t.Errorf("unexpected missing files %v", st.Missing)
	}
}

func TestItComplainsWhenAnAthanFileExistsNowhere(t *testing.T) {
	st := Check(filepath.Join(t.TempDir(), "none"), bundled("media_Athan.mp3")) // no Fajr Athan anywhere
	if len(st.Missing) != 1 || st.Missing[0] != "media_adhan_al_fajr.mp3" {
		t.Errorf("missing = %v, want [media_adhan_al_fajr.mp3]", st.Missing)
	}
	if !strings.Contains(st.Problem(), "media_adhan_al_fajr.mp3") {
		t.Errorf("problem text %q does not name the missing file", st.Problem())
	}
}

func TestAMissingBundledFileCanBeSuppliedLocally(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "media_adhan_al_fajr.mp3"), []byte("ID3"), 0o644)
	st := Check(dir, bundled("media_Athan.mp3"))
	if len(st.Missing) != 0 || st.Problem() != "" {
		t.Errorf("file supplied locally still reported missing: %v / %q", st.Missing, st.Problem())
	}
}
