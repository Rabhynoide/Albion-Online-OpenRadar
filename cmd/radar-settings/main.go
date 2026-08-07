// Command radar-settings is OpenRadar's native, browser-free configuration client: capture +
// HTTP/WebSocket server (for the Market page, still web-only) plus a Fyne settings window
// covering everything else the web app used to (Players/Resources/Enemies/Chests/Ignore
// List/Settings). It cannot link internal/overlay (Ebiten) - see internal/radarapp's package doc
// - so launching the click-through map overlay spawns the sibling cmd/radar binary as a child
// process instead (internal/settingsui/overlay_control.go).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/logger"
	"github.com/nospy/albion-openradar/internal/radarapp"
	"github.com/nospy/albion-openradar/internal/settingsui"
)

// Version info (injected at build time via ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

const (
	serverPort     = 5001
	pcapCaptureDir = "./logs/captures"
)

func main() {
	devMode := flag.Bool("dev", false, "Run in development mode (read files from disk)")
	showVersion := flag.Bool("version", false, "Show version information")
	ipAddr := flag.String("ip", "", "Network adapter IP address (skip interactive prompt)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("OpenRadar Settings v%s (built: %s)\n", Version, BuildTime)
		return
	}

	fmt.Printf("OpenRadar Settings v%s\n", Version)
	fmt.Println("====================")

	appDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	allIfaces, err := capture.EnumerateInterfaces()
	if err != nil {
		fmt.Printf("Failed to enumerate interfaces: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	app, target, err := radarapp.New(ctx, cancel, radarapp.Config{
		AppDir:         appDir,
		DevMode:        *devMode,
		IPAddr:         *ipAddr,
		ServerPort:     serverPort,
		PcapCaptureDir: pcapCaptureDir,
		Version:        Version,
		BuildTime:      BuildTime,
	})
	if err != nil {
		cancel()
		fmt.Printf("Failed to create app: %v\n", err)
		os.Exit(1)
	}

	if err := app.StartCapture(target); err != nil {
		logger.PrintWarn("NET", "Some interfaces failed to open: %v", err)
	}

	app.StartUpdateCheck(appDir)
	go app.StartServers()

	deps := settingsui.Deps{
		AppDir:     appDir,
		ServerPort: serverPort,
		Capture:    app.CaptureManager,
		Logger:     app.Logger,
		CaptureDir: pcapCaptureDir,
		AllIfaces:  allIfaces,
	}
	if err := settingsui.Run(deps); err != nil {
		fmt.Printf("Settings window error: %v\n", err)
	}

	app.Shutdown()
}
