//go:build !cgo

package localplay

// StartStream starts streaming audio from url using an external CLI player
// (mpg123, ffplay, or cvlc — first found wins).
// This build path is used when CGO_ENABLED=0 (e.g. headless Docker builds).
func StartStream(url string) (func(), error) {
	return startStreamExternal(url)
}
