package overlay

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// windowState is the overlay window's own position/size, restored across launches. Unlike
// settings.go's settingsPanel (a read-only view of settings-sync.json, the shared store the web
// app/native settings client write feature toggles to), this is chrome state nobody else needs
// to read or edit - so it lives in its own small file and the overlay is the sole reader/writer,
// no conflict with the "overlay never writes shared settings" rule documented in
// docs/technical/NATIVE_OVERLAY_CLIENT.md.
type windowState struct {
	X, Y, Width, Height int
}

const windowStateFilename = "overlay-window.json"

func loadWindowState(appDir string) (windowState, bool) {
	data, err := os.ReadFile(filepath.Join(appDir, windowStateFilename))
	if err != nil {
		return windowState{}, false
	}
	var s windowState
	if err := json.Unmarshal(data, &s); err != nil {
		return windowState{}, false
	}
	if s.Width < minWindowSize || s.Height < minWindowSize {
		return windowState{}, false
	}
	// The window is always kept square (see Game.updateDrag) - normalize a hand-edited or
	// otherwise stale non-square file rather than rejecting it outright.
	if s.Width != s.Height {
		size := s.Width
		if s.Height < size {
			size = s.Height
		}
		s.Width, s.Height = size, size
	}
	return s, true
}

func saveWindowState(appDir string, s windowState) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(appDir, windowStateFilename)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
