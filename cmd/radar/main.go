package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/logger"
	"github.com/nospy/albion-openradar/internal/overlay"
	"github.com/nospy/albion-openradar/internal/photon"
	"github.com/nospy/albion-openradar/internal/radarapp"
	"github.com/nospy/albion-openradar/internal/radarstate"
	"github.com/nospy/albion-openradar/internal/ui"
)

// Version info (injected at build time via ldflags)
// Default values are used when running with 'go run' without ldflags
var (
	Version   = "dev"
	BuildTime = "unknown"
)

const (
	serverPort     = 5001
	pcapCaptureDir = "./logs/captures"
)

// App layers this binary's UI-specific state (TUI, native map overlay) on top of the shared
// radarapp.App - see internal/radarapp's package doc for why that split exists: this binary
// links internal/overlay (Ebiten), which cannot coexist in one executable with Fyne
// (cmd/radar-settings), so anything overlay/TUI-specific stays here rather than in the shared
// package.
type App struct {
	*radarapp.App
	program      *tea.Program
	radarRouter  *radarstate.Router
	overlayState *overlay.State
}

func main() {
	cfg := parseFlags()
	if cfg.showVersion {
		fmt.Printf("OpenRadar v%s (built: %s)\n", Version, BuildTime)
		return
	}

	printBanner()

	for {
		shouldRestart := runApp(cfg)
		if !shouldRestart {
			break
		}
		fmt.Println("Restarting...")
	}
}

func runApp(cfg Config) bool {
	appDir, err := os.Getwd()
	if err != nil {
		exitWithError("Failed to get working directory", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	app, err := newApp(appDir, cfg, ctx, cancel)
	if err != nil {
		cancel()
		exitWithError("Failed to create app", err)
	}

	restartRequested := false

	if cfg.overlay {
		// No TUI in this mode - app.program stays nil. The native window is a separate OS
		// window entirely, and -no-server skips the HTTP/WS server when this process was
		// launched as a child of cmd/radar-settings (which already runs one).
		app.StartUpdateCheck(appDir)
		if !cfg.noServer {
			go app.StartServers()
		}
		go app.updateStats()

		if err := overlay.Run(app.overlayState, appDir); err != nil {
			fmt.Printf("Overlay error: %v\n", err)
		}

		app.Shutdown()
		return restartRequested
	}

	dashboard := ui.NewDashboard(Version, serverPort, cfg.devMode, capture.LANAddresses(), nil)
	app.program = tea.NewProgram(dashboard, tea.WithAltScreen())
	app.OnUpdateAvailable = func(version string) {
		app.program.Send(ui.UpdateAvailableMsg{Version: version})
	}

	app.startCaptureStatePoll()
	app.StartUpdateCheck(appDir)

	// Set up log callback to send logs to dashboard
	logger.SetLogCallback(func(level, tag, message string) {
		app.program.Send(ui.LogMsg{
			Level:   level,
			Tag:     tag,
			Message: message,
		})
	})

	// Start servers in background (will also print session info)
	go app.StartServers()

	// Start stats updater
	go app.updateStats()

	// Run dashboard (blocking)
	model, err := app.program.Run()
	if err != nil {
		logger.ClearLogCallback()
		fmt.Printf("Dashboard error: %v\n", err)
	}

	// Check if restart was requested
	if d, ok := model.(ui.Dashboard); ok {
		restartRequested = d.RestartRequested()
	}

	// Cleanup
	logger.ClearLogCallback()
	app.Shutdown()

	return restartRequested
}

// Config holds command-line configuration
type Config struct {
	devMode     bool
	showVersion bool
	ipAddr      string
	overlay     bool
	noServer    bool
}

func parseFlags() Config {
	cfg := Config{}
	flag.BoolVar(&cfg.devMode, "dev", false, "Run in development mode (read files from disk)")
	flag.BoolVar(&cfg.showVersion, "version", false, "Show version information")
	flag.StringVar(&cfg.ipAddr, "ip", "", "Network adapter IP address (skip interactive prompt)")
	flag.BoolVar(&cfg.overlay, "overlay", false, "Run the native click-through radar overlay instead of the TUI dashboard")
	flag.BoolVar(&cfg.noServer, "no-server", false, "Skip starting the HTTP/WebSocket server (used when launched as a child process by cmd/radar-settings, whose own server already covers it)")
	flag.Parse()
	return cfg
}

func printBanner() {
	fmt.Printf("OpenRadar v%s\n", Version)
	fmt.Println("====================")
}

func exitWithError(msg string, err error) {
	fmt.Printf("%s: %v\n", msg, err)
	os.Exit(1)
}

func newApp(appDir string, cfg Config, ctx context.Context, cancel context.CancelFunc) (*App, error) {
	base, target, err := radarapp.New(ctx, cancel, radarapp.Config{
		AppDir:         appDir,
		DevMode:        cfg.devMode,
		IPAddr:         cfg.ipAddr,
		ServerPort:     serverPort,
		PcapCaptureDir: pcapCaptureDir,
		Version:        Version,
		BuildTime:      BuildTime,
	})
	if err != nil {
		return nil, err
	}

	app := &App{App: base}

	if cfg.overlay {
		state, router, err := buildOverlayState(appDir)
		if err != nil {
			return nil, fmt.Errorf("failed to build overlay state: %w", err)
		}
		app.overlayState = state
		app.radarRouter = router
		app.OnEvent = router.HandleEvent
		app.OnRequest = router.HandleRequest
		app.OnResponse = func(resp *photon.OperationResponse) { router.HandleResponse(resp, router.ClearAll) }
	}

	if err := app.StartCapture(target); err != nil {
		logger.PrintWarn("NET", "Some interfaces failed to open: %v", err)
	}

	return app, nil
}

func (app *App) updateStats() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-app.Context().Done():
			return
		case <-ticker.C:
			if app.program != nil {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)

				wsStats := app.WSHandler.Stats()
				logStats := app.Logger.GetStats()
				app.program.Send(ui.StatsMsg{
					Packets:       app.PacketsProcessed(),
					Errors:        app.PacketsErrors(),
					WsClients:     app.WSHandler.ClientCount(),
					MemoryMB:      float64(m.Alloc) / 1024 / 1024,
					MemorySysMB:   float64(m.Sys) / 1024 / 1024,
					Goroutines:    runtime.NumGoroutine(),
					WsBatches:     wsStats.BatchesSent,
					WsMessages:    wsStats.MessagesSent,
					WsQueueSize:   wsStats.MessagesQueue,
					BytesReceived: app.CaptureManager.BytesReceived(),
					BytesSent:     wsStats.BytesSent,
					LogEntries:    logStats.TotalEntries,
					LogBatches:    logStats.TotalBatches,
					LogBufferSize: logStats.BufferSize,
				})

				captureActive := len(app.CaptureManager.State().Active) > 0
				app.program.Send(ui.StatusMsg{
					HTTPRunning:    app.HTTPRunning(),
					WSRunning:      app.WSHandler.ClientCount() >= 0,
					CaptureRunning: captureActive,
				})
			}
		}
	}
}

// startCaptureStatePoll pushes a CaptureStateMsg to the TUI every 2s so
// header and Config tab reflect live Manager state without coupling ui to capture.
func (app *App) startCaptureStatePoll() {
	app.Go(func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-app.Context().Done():
				return
			case <-t.C:
				if app.program == nil {
					continue
				}
				s := app.CaptureManager.State()
				summaries := make([]ui.CaptureSummary, 0, len(s.Active))
				for _, a := range s.Active {
					summaries = append(summaries, ui.CaptureSummary{
						Description: a.Description,
						Address:     a.Address,
						Category:    string(a.Category),
					})
				}
				app.program.Send(ui.CaptureStateMsg{
					Active:       summaries,
					LanAddresses: capture.LANAddresses(),
					Status:       string(s.Status),
				})
			}
		}
	})
}
