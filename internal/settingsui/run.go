package settingsui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/logger"
)

// CaptureController is the subset of *capture.Manager the Network/Debug sections of the
// Settings page need - live interface state/reconfiguration and pcap recording control, the
// same split internal/server/network_api.go's NetworkManager and settings_api.go's Recorder
// interfaces already use for exactly the same reason (minimal coupling, easy to fake in tests).
type CaptureController interface {
	State() capture.State
	Reconfigure([]capture.NetworkInterface) error
	StartRecording(dir string) error
	StopRecording() error
	IsRecording() bool
}

// Deps is everything the settings window needs from cmd/radar's App - mirrors
// cmd/radar/overlay_setup.go's buildOverlayState pattern of the caller assembling already-running
// infrastructure and handing over just enough, since this package can't import cmd/radar's App
// type (main imports settingsui, never the other way round).
type Deps struct {
	AppDir     string
	ServerPort int
	Capture    CaptureController
	Logger     *logger.Logger
	CaptureDir string
	AllIfaces  []capture.NetworkInterface
}

// Run builds and shows the native settings window, blocking until the user closes it (same
// blocking contract as overlay.Run/tea.Program.Run). It runs on the OS main thread - Fyne
// requires that, same as Ebiten requires it for the map overlay - which is exactly why the
// overlay is launched as a separate child process instead of embedded here (see
// overlay_control.go's doc comment).
func Run(deps Deps) error {
	s := NewStore(deps.AppDir)
	oc := newOverlayControl(deps.AppDir)

	a := app.NewWithID("com.openradar.settings")
	a.Settings().SetTheme(&radarTheme{})
	w := a.NewWindow("OpenRadar")
	w.Resize(fyne.NewSize(1020, 720))
	w.SetOnClosed(oc.stop)

	items := []navItem{
		{"Joueurs", newPlayersPage(s)},
		{"Ressources", newResourcesPage(s)},
		{"Ennemis", newEnemiesPage(s)},
		{"Coffres", newChestsPage(s)},
		{"Liste ignorée", newIgnoreListPage(s)},
		{"Réglages", newSettingsPage(deps, s)},
	}
	sidebar, pages := buildSidebarNav(items)

	w.SetContent(container.NewBorder(newTopBar(deps, oc), nil, sidebar, nil, pages))
	w.ShowAndRun()
	return nil
}

// newTopBar mirrors the web app's header: brand on the left, live capture status + the overlay
// launcher + a reminder that Market stays browser-only on the right (see
// docs/technical/NATIVE_SETTINGS_CLIENT.md).
func newTopBar(deps Deps, oc *overlayControl) fyne.CanvasObject {
	brand := widget.NewLabelWithStyle("OpenRadar", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	brand.Importance = widget.HighImportance

	dot := canvas.NewCircle(radarMuted)
	statusLabel := widget.NewLabel("Capture : ...")
	refreshStatus := func() {
		if deps.Capture == nil {
			return
		}
		if len(deps.Capture.State().Active) > 0 {
			dot.FillColor = radarSuccess
			statusLabel.SetText("Capture active")
		} else {
			dot.FillColor = radarMuted
			statusLabel.SetText("Capture arrêtée")
		}
		dot.Refresh()
	}
	refreshStatus()
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for range t.C {
			fyne.Do(refreshStatus)
		}
	}()
	dotBox := container.New(layout.NewGridWrapLayout(fyne.NewSize(10, 10)), dot)
	status := container.NewHBox(dotBox, statusLabel)

	webInfo := widget.NewLabel(fmt.Sprintf("Web (Market) : http://localhost:%d/market", deps.ServerPort))

	right := container.NewHBox(status, widget.NewSeparator(), oc.widget(), widget.NewSeparator(), webInfo)
	return container.NewBorder(nil, widget.NewSeparator(), brand, right)
}
