package settingsui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/nospy/albion-openradar/internal/logger"
)

// overlayControl launches/stops the native click-through map overlay (internal/overlay, run via
// the sibling cmd/radar binary's -overlay flag) as a separate child process. This has to be a
// genuinely different EXECUTABLE, not just a different process running the same binary: Ebiten
// (used by internal/overlay) and Fyne (used by this package) cannot both be linked into the same
// Windows binary at all - even an unused import of one breaks the other's window creation (see
// docs/technical/NATIVE_SETTINGS_CLIENT.md) - so cmd/radar-settings (this package's host binary)
// never imports internal/overlay, and instead shells out to cmd/radar, which does. -no-server is
// passed because this settings process already runs the HTTP/WebSocket server (internal/server)
// the child would otherwise try to bind a second time on the same port; capture.Manager running
// twice (once per process) is accepted as a cheap, well-understood duplication of a passive local
// pcap listener, not a real cost.
type overlayControl struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	button *widget.Button
	status *widget.Label
	appDir string
}

// newOverlayControl takes appDir as a fallback lookup location for the sibling cmd/radar
// binary (see radarExecutablePath) - needed for `go run ./cmd/radar-settings`, where
// os.Executable() points at a Go build-cache temp binary with no radar.exe next to it, unlike a
// real shipped build (see Makefile) where both binaries sit side by side.
func newOverlayControl(appDir string) *overlayControl {
	oc := &overlayControl{
		status: widget.NewLabel("Overlay carte : arrêté"),
		appDir: appDir,
	}
	oc.button = widget.NewButton("Lancer l'overlay", oc.toggle)
	return oc
}

func (oc *overlayControl) widget() fyne.CanvasObject {
	return container.NewHBox(oc.button, oc.status)
}

func (oc *overlayControl) toggle() {
	oc.mu.Lock()
	running := oc.cmd != nil
	oc.mu.Unlock()
	if running {
		oc.stop()
	} else {
		oc.start()
	}
}

func (oc *overlayControl) start() {
	exe, err := radarExecutablePath(oc.appDir)
	if err != nil {
		logger.PrintWarn("SETTINGS", "resolve radar executable failed: %v", err)
		return
	}
	cmd := exec.Command(exe, "-overlay", "-no-server")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		logger.PrintWarn("SETTINGS", "launch overlay failed: %v", err)
		return
	}

	oc.mu.Lock()
	oc.cmd = cmd
	oc.mu.Unlock()
	oc.button.SetText("Arrêter l'overlay")
	oc.status.SetText("Overlay carte : en cours")

	go func() {
		_ = cmd.Wait()
		oc.mu.Lock()
		oc.cmd = nil
		oc.mu.Unlock()
		fyne.Do(func() {
			oc.button.SetText("Lancer l'overlay")
			oc.status.SetText("Overlay carte : arrêté")
		})
	}()
}

// stop kills the child process; its cmd.Wait() goroutine (started in start()) resets the
// button/status once the process actually exits, so this doesn't touch UI state directly.
func (oc *overlayControl) stop() {
	oc.mu.Lock()
	cmd := oc.cmd
	oc.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := cmd.Process.Kill(); err != nil {
		logger.PrintWarn("SETTINGS", "stop overlay failed: %v", err)
	}
}

// radarExecutablePath resolves the sibling cmd/radar binary - "radar.exe" (or "radar" outside
// Windows). It tries two locations: next to this process's own executable first (a real shipped
// build, see Makefile, where both binaries sit side by side), then appDir (the working
// directory) - needed for `go run ./cmd/radar-settings`, whose os.Executable() points at a
// throwaway Go build-cache temp binary with nothing next to it, but whose working directory is
// still the project root a manually-built radar.exe would sit in.
func radarExecutablePath(appDir string) (string, error) {
	name := "radar"
	if runtime.GOOS == "windows" {
		name = "radar.exe"
	}

	var tried []string
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		tried = append(tried, candidate)
	}

	candidate := filepath.Join(appDir, name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	tried = append(tried, candidate)

	return "", fmt.Errorf("%s not found (tried %v)", name, tried)
}
