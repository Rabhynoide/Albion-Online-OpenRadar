// Package radarapp is the capture+HTTP/WebSocket bootstrap shared by every OpenRadar binary
// (cmd/radar's TUI/-overlay modes, cmd/radar-settings' native settings window). It deliberately
// has NO dependency on any GUI toolkit (no bubbletea, no internal/overlay, no internal/settingsui)
// - see docs/technical/NATIVE_SETTINGS_CLIENT.md for why that matters: Ebiten (used by
// internal/overlay) and Fyne (used by internal/settingsui) cannot both be linked into the same
// Windows executable, even if only one is ever actually run - the mere presence of both in one
// binary makes Fyne's window-class registration fail. Keeping this package UI-agnostic and having
// each binary layer its own UI-specific wiring on top (via the OnEvent/OnRequest/OnResponse hooks
// below) is what keeps cmd/radar and cmd/radar-settings safely separable.
package radarapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	assets "github.com/nospy/albion-openradar"
	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/logger"
	"github.com/nospy/albion-openradar/internal/photon"
	"github.com/nospy/albion-openradar/internal/server"
	"github.com/nospy/albion-openradar/internal/updatecheck"
)

const (
	shutdownTimeout = 10 * time.Second
	// updateCheckInterval throttles how often StartUpdateCheck actually calls the GitHub API,
	// mirroring internal/hub's own marketStaleAfter-style cache TTL precedent.
	updateCheckInterval = time.Hour
)

// Config is everything a binary's flags decide before bootstrapping.
type Config struct {
	AppDir         string
	DevMode        bool
	IPAddr         string // network adapter IP override, skips interface auto-pick
	ServerPort     int
	PcapCaptureDir string
	Version        string
	BuildTime      string
}

// App owns capture, the HTTP/WebSocket server, and Photon dispatch - the same responsibilities
// cmd/radar/main.go's App held before this package existed. OnEvent/OnRequest/OnResponse let a
// caller observe already-decoded Photon data as a second subscriber alongside the WebSocket
// broadcast (e.g. cmd/radar's -overlay mode feeding internal/radarstate) without this package
// needing to import whatever GUI-specific package owns that subscriber.
type App struct {
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	version    string
	serverPort int

	Logger         *logger.Logger
	HTTPServer     *server.HTTPServer
	WSHandler      *server.WebSocketHandler
	CaptureManager *capture.Manager

	photonParser *photon.PhotonParser

	OnEvent          func(*photon.EventData)
	OnRequest        func(*photon.OperationRequest)
	OnResponse       func(*photon.OperationResponse)
	OnUpdateAvailable func(version string)

	packetsProcessed uint64
	packetsErrors    uint64
	packetsEncrypted uint64
	httpRunning      int32
}

// New enumerates network interfaces, resolves/persists the capture target, and builds the HTTP
// server and capture manager - the same sequence cmd/radar/main.go's runApp+newApp used to
// perform inline. It deliberately does NOT start capturing (capture.Manager.Reconfigure) or the
// HTTP server goroutine itself: callers that need to observe Photon events as a second
// subscriber (setting OnEvent/OnRequest/OnResponse) must do so before capture starts, so this
// returns the resolved target interfaces for the caller to pass to
// app.CaptureManager.Reconfigure(target) once hooks are wired - see StartServers for the server
// goroutine.
func New(ctx context.Context, cancel context.CancelFunc, cfg Config) (*App, []capture.NetworkInterface, error) {
	allIfaces, err := capture.EnumerateInterfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("enumerate interfaces: %w", err)
	}

	if _, mErr := capture.MigrateIPTxt(cfg.AppDir, capture.ResolveByIP); mErr != nil {
		logger.PrintWarn("NET", "ip.txt migration failed: %v", mErr)
	}

	cfgPersisted, _ := capture.ReadConfig(cfg.AppDir)
	target := resolvePersisted(cfgPersisted, allIfaces, cfg.IPAddr)
	if len(target) == 0 {
		target = autoPickDefaults(allIfaces)
		if len(target) > 0 {
			toPersist := make([]capture.PersistedInterface, 0, len(target))
			for _, i := range target {
				toPersist = append(toPersist, capture.PersistedInterface{Name: i.Name, Description: i.Description})
			}
			_ = capture.WriteConfig(cfg.AppDir, capture.Config{CaptureInterfaces: toPersist})
			logger.PrintInfo("NET", "Auto-selected %d interface(s). Change in settings if needed.", len(target))
		}
	}

	manager := capture.NewManager(ctx)
	log := logger.New("./logs", cfgPersisted.Logging.ServerLogsEnabled)
	wsHandler := server.NewWebSocketHandler(log)

	httpServer, err := createHTTPServer(cfg, wsHandler, log, manager, allIfaces)
	if err != nil {
		manager.Close(context.Background())
		return nil, nil, fmt.Errorf("create HTTP server: %w", err)
	}

	app := &App{
		ctx:            ctx,
		cancel:         cancel,
		version:        cfg.Version,
		serverPort:     cfg.ServerPort,
		Logger:         log,
		HTTPServer:     httpServer,
		WSHandler:      wsHandler,
		CaptureManager: manager,
	}
	app.photonParser = photon.NewPhotonParser(app.dispatchEvent, app.dispatchRequest, app.dispatchResponse)
	app.photonParser.OnEncrypted = app.onPhotonEncrypted
	app.photonParser.OnParseError = app.onPhotonParseError
	app.CaptureManager.OnPacket(app.handlePacket)

	if cfgPersisted.Logging.PcapRecording {
		if err := manager.StartRecording(cfg.PcapCaptureDir); err != nil {
			logger.PrintWarn("PKT", "pcap recording could not start: %v", err)
			_ = capture.MutateConfig(cfg.AppDir, func(c *capture.Config) {
				c.Logging.PcapRecording = false
			})
		}
	}

	return app, target, nil
}

// StartCapture opens the resolved target interfaces (returned by New) - call after wiring
// OnEvent/OnRequest/OnResponse so no Photon events are dispatched before those hooks are set.
func (a *App) StartCapture(target []capture.NetworkInterface) error {
	return a.CaptureManager.Reconfigure(target)
}

func createHTTPServer(
	cfg Config,
	wsHandler *server.WebSocketHandler,
	log *logger.Logger,
	mgr *capture.Manager,
	allIfaces []capture.NetworkInterface,
) (*server.HTTPServer, error) {
	if cfg.DevMode {
		logger.PrintInfo("MODE", "Development mode: reading files from disk")
		return server.NewHTTPServerDev(cfg.ServerPort, cfg.AppDir, wsHandler, log, cfg.Version, cfg.BuildTime, mgr, allIfaces, mgr, cfg.PcapCaptureDir)
	}
	logger.PrintInfo("MODE", "Production mode: using embedded assets")
	return server.NewHTTPServer(
		cfg.ServerPort,
		assets.Images,
		assets.Scripts,
		assets.Data,
		assets.Sounds,
		assets.Styles,
		assets.Templates,
		wsHandler,
		log,
		cfg.Version,
		cfg.BuildTime,
		mgr,
		allIfaces,
		cfg.AppDir,
		mgr,
		cfg.PcapCaptureDir,
	)
}

func (a *App) Context() context.Context { return a.ctx }

// Go tracks a background goroutine against this App's shutdown WaitGroup, so Shutdown waits for
// it to exit (bounded by shutdownTimeout) - the same pattern startServers/startUpdateCheck used
// inline before this package existed, exposed for callers that add their own goroutines (e.g.
// cmd/radar's TUI capture-state poller).
func (a *App) Go(fn func()) { a.wg.Go(fn) }

func (a *App) HTTPRunning() bool { return atomic.LoadInt32(&a.httpRunning) == 1 }
func (a *App) PacketsProcessed() uint64 { return atomic.LoadUint64(&a.packetsProcessed) }
func (a *App) PacketsErrors() uint64    { return atomic.LoadUint64(&a.packetsErrors) }
func (a *App) PacketsEncrypted() uint64 { return atomic.LoadUint64(&a.packetsEncrypted) }

// StartServers starts the HTTP/WebSocket server in a tracked goroutine and prints the session
// banner. Blocking-safe to call via `go app.StartServers()`.
func (a *App) StartServers() {
	a.Logger.PrintSessionInfo()
	logger.PrintInfo("APP", "Starting servers...")

	a.wg.Go(func() {
		atomic.StoreInt32(&a.httpRunning, 1)
		if err := a.HTTPServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) &&
			a.ctx.Err() == nil {
			logger.PrintError("HTTP", "Error: %v", err)
		}
		atomic.StoreInt32(&a.httpRunning, 0)
	})

	time.Sleep(100 * time.Millisecond)

	logger.PrintSuccess("HTTP", "Server: http://localhost:%d", a.serverPort)
	for _, ip := range capture.LANAddresses() {
		logger.PrintSuccess("HTTP", "Server: http://%s:%d  (LAN)", ip, a.serverPort)
	}
	logger.PrintSuccess("WS", "WebSocket: ws://localhost:%d/ws", a.serverPort)
	for _, ip := range capture.LANAddresses() {
		logger.PrintSuccess("WS", "WebSocket: ws://%s:%d/ws  (LAN)", ip, a.serverPort)
	}
	logger.PrintInfo("PKT", "Listening for Albion packets on UDP port 5056...")
	for _, s := range a.CaptureManager.State().Active {
		logger.PrintInfo("NET", "Capturing on %s [%s]", s.Description, s.Address)
	}
}

func (a *App) handlePacket(payload []byte) {
	if a.photonParser.ReceivePacket(payload) {
		atomic.AddUint64(&a.packetsProcessed, 1)
	}
}

func (a *App) onPhotonParseError(reason string, payloadLen int) {
	n := atomic.AddUint64(&a.packetsErrors, 1)
	if n%100 == 1 {
		logger.PrintWarn("PKT", "Parsing errors: %d (last reason: %s, payload len: %d)",
			n, reason, payloadLen)
	}
}

func (a *App) dispatchEvent(event *photon.EventData) {
	photon.PostProcessEvent(event)
	if a.Logger.IsEnabled() {
		realCode := event.Parameters[252]
		a.Logger.Debug("EVENT_CAPTURE", fmt.Sprintf("Event_%v", realCode), map[string]interface{}{
			"code":       realCode,
			"paramCount": len(event.Parameters),
		}, nil)
	}
	a.WSHandler.BroadcastEvent(event)
	if a.OnEvent != nil {
		a.OnEvent(event)
	}
}

func (a *App) dispatchRequest(req *photon.OperationRequest) {
	photon.PostProcessRequest(req)
	a.WSHandler.BroadcastRequest(req)
	if a.OnRequest != nil {
		a.OnRequest(req)
	}
}

func (a *App) dispatchResponse(resp *photon.OperationResponse) {
	photon.PostProcessResponse(resp)
	a.WSHandler.BroadcastResponse(resp)
	if a.OnResponse != nil {
		a.OnResponse(resp)
	}
}

func (a *App) onPhotonEncrypted() {
	n := atomic.AddUint64(&a.packetsEncrypted, 1)
	if n%100 == 1 {
		logger.PrintWarn("PKT", "Encrypted traffic seen (%d so far, ignored)", n)
	}
}

// StartUpdateCheck fires a one-shot background check for a newer OpenRadar release. It never
// blocks startup and never surfaces a network failure anywhere - same "never let an optional
// external call disrupt the app" philosophy as the Hub fallback paths. See
// docs/technical/AUTO_UPDATE_CHECK.md.
func (a *App) StartUpdateCheck(appDir string) {
	a.wg.Go(func() {
		if a.version == "" || a.version == "dev" {
			return
		}

		cfg, err := capture.ReadConfig(appDir)
		if err != nil {
			return
		}

		if time.Since(cfg.UpdateCheck.LastChecked) < updateCheckInterval {
			a.notifyIfUpdateAvailable(cfg.UpdateCheck.LatestVersion, cfg.UpdateCheck.DismissedVersion)
			return
		}

		release, err := updatecheck.NewClient(updatecheck.DefaultRepo).FetchLatest()
		if err != nil {
			logger.PrintWarn("UPDATE", "check failed: %v", err)
			return
		}

		var dismissed string
		if err := capture.MutateConfig(appDir, func(c *capture.Config) {
			c.UpdateCheck.LatestVersion = release.TagName
			c.UpdateCheck.ReleaseURL = release.HTMLURL
			c.UpdateCheck.LastChecked = time.Now()
			dismissed = c.UpdateCheck.DismissedVersion
		}); err != nil {
			logger.PrintWarn("UPDATE", "persist check result failed: %v", err)
			return
		}

		a.notifyIfUpdateAvailable(release.TagName, dismissed)
	})
}

func (a *App) notifyIfUpdateAvailable(latest, dismissed string) {
	if a.OnUpdateAvailable == nil {
		return
	}
	if updatecheck.IsNewer(a.version, latest) && latest != dismissed {
		a.OnUpdateAvailable(latest)
	}
}

func (a *App) Shutdown() {
	logger.PrintInfo("APP", "Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	a.cancel()
	a.CaptureManager.Close(ctx)
	a.Logger.Stop()

	if err := a.HTTPServer.Shutdown(ctx); err != nil {
		logger.PrintError("HTTP", "Shutdown error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.PrintSuccess("APP", "Shutdown complete")
	case <-ctx.Done():
		logger.PrintWarn("APP", "Shutdown timed out")
	}
}

// resolvePersisted maps a persisted (or CLI-overridden) selection to currently available
// NetworkInterface entries. Returns nil if the override IP no longer resolves; the caller falls
// back to autoPickDefaults.
func resolvePersisted(cfg capture.Config, all []capture.NetworkInterface, ipOverride string) []capture.NetworkInterface {
	if ipOverride != "" {
		for _, i := range all {
			if i.Address == ipOverride {
				return []capture.NetworkInterface{i}
			}
		}
		return nil
	}
	available := make(map[string]capture.NetworkInterface, len(all))
	for _, i := range all {
		available[i.Name] = i
	}
	out := make([]capture.NetworkInterface, 0, len(cfg.CaptureInterfaces))
	for _, p := range cfg.CaptureInterfaces {
		if i, ok := available[p.Name]; ok {
			out = append(out, i)
		}
	}
	return out
}

func autoPickDefaults(all []capture.NetworkInterface) []capture.NetworkInterface {
	out := make([]capture.NetworkInterface, 0)
	for _, i := range capture.RankCandidates(all) {
		c := capture.Categorize(i.Name, i.Description)
		if (c == capture.CategoryEthernet || c == capture.CategoryWiFi || c == capture.CategoryExitLag) && capture.IsRFC1918(i.Address) {
			out = append(out, i)
		}
	}
	return out
}
