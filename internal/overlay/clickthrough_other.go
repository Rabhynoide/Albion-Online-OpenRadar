//go:build !windows

package overlay

// ebitenKey mirrors the Windows build's Win32 virtual-key type so callers compile identically
// on both platforms - see clickthrough_windows.go's doc comment for why a global key poll is
// used instead of a normal Ebiten key listener.
type ebitenKey int

const (
	keyF2 ebitenKey = iota
	keyF3
	keyF4
	keyF5
	keyF6
	keyF9
)

// isKeyDownGlobally has no cross-platform equivalent to Windows' GetAsyncKeyState implemented
// yet - global key toggles are a Windows-only capability for now (see
// docs/technical/NATIVE_OVERLAY_CLIENT.md). Always reports "not pressed" elsewhere, so the
// overlay still builds and runs on Linux, just without the toggles.
func isKeyDownGlobally(key ebitenKey) bool {
	return false
}
