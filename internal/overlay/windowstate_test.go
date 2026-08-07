package overlay

import "testing"

func TestSaveLoadWindowStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := windowState{X: 120, Y: 340, Width: 450, Height: 450}
	saveWindowState(dir, want)

	got, ok := loadWindowState(dir)
	if !ok {
		t.Fatal("loadWindowState: ok=false, want true")
	}
	if got != want {
		t.Errorf("loadWindowState = %+v, want %+v", got, want)
	}
}

func TestLoadWindowStateNormalizesNonSquare(t *testing.T) {
	dir := t.TempDir()
	saveWindowState(dir, windowState{X: 0, Y: 0, Width: 600, Height: 450})

	got, ok := loadWindowState(dir)
	if !ok {
		t.Fatal("loadWindowState: ok=false, want true")
	}
	if got.Width != 450 || got.Height != 450 {
		t.Errorf("loadWindowState = %+v, want square 450x450 (smaller of the two)", got)
	}
}

func TestLoadWindowStateMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, ok := loadWindowState(dir)
	if ok {
		t.Error("loadWindowState on empty dir: ok=true, want false")
	}
}

func TestLoadWindowStateRejectsBelowMinimum(t *testing.T) {
	dir := t.TempDir()
	saveWindowState(dir, windowState{X: 0, Y: 0, Width: minWindowSize - 1, Height: minWindowSize})

	_, ok := loadWindowState(dir)
	if ok {
		t.Error("loadWindowState with undersized width: ok=true, want false")
	}
}
