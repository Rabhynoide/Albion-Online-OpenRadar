//go:build windows

package overlay

import "syscall"

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

// ebitenKey is a Win32 virtual-key code, deliberately not ebiten.Key: once
// SetWindowMousePassthrough(true) is active this window can never regain focus through a
// click, so a focus-based key listener (including Ebiten's own inpututil) goes deaf exactly
// when a toggle key is needed most. Every toggle in this package (click-through, the settings
// panel) is read this way instead - see the validated spike in
// docs/technical/NATIVE_OVERLAY_CLIENT.md.
type ebitenKey int

const (
	keyF2 ebitenKey = 0x71
	keyF3 ebitenKey = 0x72
	keyF4 ebitenKey = 0x73
	keyF5 ebitenKey = 0x74
	keyF6 ebitenKey = 0x75
	keyF9 ebitenKey = 0x78
)

// isKeyDownGlobally polls a key's state system-wide via GetAsyncKeyState, independent of
// window focus.
func isKeyDownGlobally(key ebitenKey) bool {
	ret, _, _ := procGetAsyncKeyState.Call(uintptr(key))
	return ret&0x8000 != 0
}
