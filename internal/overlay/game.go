package overlay

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/nospy/albion-openradar/internal/radarstate"
)

const (
	targetTPS     = 30 // matches RadarRenderer.js's TARGET_FPS
	windowTitle   = "OpenRadar Overlay"
	defaultWindow = 500
	minWindowSize = 200

	// resizeHandleSize is the clickable bottom-right corner square, in logical pixels, that
	// starts a resize drag - since the window is undecorated (Run), there's no OS-drawn border
	// to grab, so this is drawn and hit-tested by hand (see Draw/beginDrag).
	resizeHandleSize = 18
)

// dragMode tracks what a held left-click currently does to the window - see Update.
type dragMode int

const (
	dragNone dragMode = iota
	dragMove
	dragResize
)

// Game implements ebiten.Game - the native radar window. Renders the local player (fixed
// screen-center dot, matching the web radar's own "world moves, player doesn't" convention),
// the zone map background, and every tracked entity type with a distinct color/shape per
// category - a functional pass, not a pixel-perfect port of DrawingUtils.js's canvas
// gradients/badges/pulsing rings (see docs/technical/NATIVE_OVERLAY_CLIENT.md). Resource
// clustering and health bars are deliberately out of scope for this client (not needed).
type Game struct {
	state     *State
	settings  *settingsPanel
	mapImages *mapImageCache
	audio     *alertPlayer
	appDir    string

	harvestablePos   *posTracker
	mobPos           *posTracker
	mistPos          *posTracker
	chestPos         *posTracker
	dungeonPos       *posTracker
	fishPos          *posTracker
	localTreasurePos *posTracker
	mistsDungeonPos  *posTracker
	wispCagePos      *posTracker
	mapHX, mapHY     float64

	width, height int
	lastFrame     time.Time

	clickThrough bool
	f9WasDown    bool

	drag                               dragMode
	dragStartCursorX, dragStartCursorY int
	dragStartWinX, dragStartWinY       int
	dragStartWinW, dragStartWinH       int

	flashUntil time.Time // one-shot screen flash (settingFlash), active while now < flashUntil
}

func NewGame(state *State, appDir string) *Game {
	return &Game{
		state:            state,
		settings:         newSettingsPanel(appDir),
		mapImages:        newMapImageCache(appDir),
		audio:            newAlertPlayer(appDir),
		appDir:           appDir,
		harvestablePos:   newPosTracker(),
		mobPos:           newPosTracker(),
		mistPos:          newPosTracker(),
		chestPos:         newPosTracker(),
		dungeonPos:       newPosTracker(),
		fishPos:          newPosTracker(),
		localTreasurePos: newPosTracker(),
		mistsDungeonPos:  newPosTracker(),
		wispCagePos:      newPosTracker(),
		width:            defaultWindow,
		height:           defaultWindow,
		lastFrame:        time.Now(),
	}
}

// Run configures the window (undecorated, transparent, always on top) and starts the Ebiten
// main loop (blocking, same "owns the main thread" role app.program.Run() plays for the
// bubbletea TUI - see cmd/radar/main.go). Click-through itself is toggled at runtime via F9
// (see Update), not here - see the click-through spike findings in
// docs/technical/NATIVE_OVERLAY_CLIENT.md for why a global key poll is required instead of a
// normal Ebiten key listener. appDir is where settings-sync.json lives (see settings.go) and
// overlay-window.json (see windowstate.go), from which the last saved position/size is restored.
func Run(state *State, appDir string) error {
	width, height := defaultWindow, defaultWindow
	ws, hasState := loadWindowState(appDir)
	if hasState {
		width, height = ws.Width, ws.Height
	}

	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowSize(width, height)
	ebiten.SetWindowSizeLimits(minWindowSize, minWindowSize, -1, -1)
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetTPS(targetTPS)
	if hasState {
		ebiten.SetWindowPosition(ws.X, ws.Y)
	}

	return ebiten.RunGameWithOptions(NewGame(state, appDir), &ebiten.RunGameOptions{ScreenTransparent: true})
}

// saveWindowGeometry persists the window's current position/size - called once a move or resize
// drag ends (Update), not every frame, so it's cheap enough to hit disk synchronously.
func (g *Game) saveWindowGeometry() {
	x, y := ebiten.WindowPosition()
	w, h := ebiten.WindowSize()
	saveWindowState(g.appDir, windowState{X: x, Y: y, Width: w, Height: h})
}

func (g *Game) Update() error {
	now := time.Now()
	dt := now.Sub(g.lastFrame)
	g.lastFrame = now

	// Global poll, not a focus-dependent Ebiten key listener - once click-through is on, this
	// window can never regain focus via a click, so a normal key listener would go deaf right
	// when the user needs the toggle most. See the validated spike.
	f9Down := isKeyDownGlobally(keyF9)
	if f9Down && !g.f9WasDown {
		g.clickThrough = !g.clickThrough
		ebiten.SetWindowMousePassthrough(g.clickThrough)
	}
	g.f9WasDown = f9Down

	g.updateDrag()

	g.settings.refresh()
	g.updateInterpolation(dt)
	g.updateAlerts(now)
	return nil
}

// updateAlerts drains PlayersState/HarvestablesState's PendingAlerts each tick and turns them
// into an actual sound/flash - see internal/radarstate's own doc comments on those methods,
// which describe exactly this integration; nothing previously called them at all; the overlay
// had zero hostile-player or resource sound/flash alerts before this. Resource sound is
// pre-gated by settingResourceSound inside HarvestablesState itself (only sound-enabled matches
// are ever queued there), so no re-check is needed here; player sound/flash are NOT pre-gated
// (see PlayersState's own doc comment) and are checked here instead.
func (g *Game) updateAlerts(now time.Time) {
	if g.state == nil {
		return
	}
	if g.state.Players != nil {
		if alerts := g.state.Players.PendingAlerts(); len(alerts) > 0 {
			if g.settings.isOn("settingSound") {
				g.audio.playHostilePlayer()
			}
			if g.settings.isOn("settingFlash") {
				g.flashUntil = now.Add(400 * time.Millisecond)
			}
		}
	}
	if g.state.Harvestables != nil {
		if alerts := g.state.Harvestables.PendingAlerts(); len(alerts) > 0 {
			g.audio.playResourceFound()
		}
	}
}

// updateDrag implements move/resize for this undecorated window by hand: a left-click starting
// in the bottom-right resizeHandleSize corner resizes, anywhere else on the (non-click-through)
// window body moves it. While click-through is on, the OS delivers no mouse events to this
// window at all, so no extra guard is needed for that case.
func (g *Game) updateDrag() {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		winX, winY := ebiten.WindowPosition()
		cx, cy := ebiten.CursorPosition()
		// The anchor is stored in ABSOLUTE screen coordinates (window position + window-relative
		// cursor position), not just the window-relative cursor position: CursorPosition() is
		// relative to the window's CURRENT top-left, which itself moves every frame during a
		// move-drag, so a window-relative-only delta would oscillate as soon as the window moved
		// out from under an otherwise-stationary mouse cursor.
		g.dragStartCursorX, g.dragStartCursorY = winX+cx, winY+cy
		g.dragStartWinX, g.dragStartWinY = winX, winY
		g.dragStartWinW, g.dragStartWinH = ebiten.WindowSize()
		if cx >= g.width-resizeHandleSize && cy >= g.height-resizeHandleSize {
			g.drag = dragResize
		} else {
			g.drag = dragMove
		}
	}

	if g.drag == dragNone {
		return
	}

	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.drag = dragNone
		g.saveWindowGeometry()
		return
	}

	winX, winY := ebiten.WindowPosition()
	cx, cy := ebiten.CursorPosition()
	dx, dy := (winX+cx)-g.dragStartCursorX, (winY+cy)-g.dragStartCursorY

	switch g.drag {
	case dragMove:
		ebiten.SetWindowPosition(g.dragStartWinX+dx, g.dragStartWinY+dy)
	case dragResize:
		// Square only: the radar view is calibrated around a square canvas (canvasCenter is a
		// single value shared by both axes - see Draw), so width and height are always kept
		// equal, driven by the average of the two drag deltas (dragStartWinW/H already start
		// equal, by the same invariant).
		delta := (dx + dy) / 2
		size := g.dragStartWinW + delta
		if size < minWindowSize {
			size = minWindowSize
		}
		ebiten.SetWindowSize(size, size)
	}
}

// t mirrors RadarRenderer.js's per-frame lerp factor: min(1, deltaTime_ms / 100), i.e. ~100ms
// smoothing between position updates regardless of actual frame rate.
func lerpFactor(dt time.Duration) float64 {
	t := float64(dt.Milliseconds()) / 100
	if t > 1 {
		t = 1
	}
	return t
}

func (g *Game) updateInterpolation(dt time.Duration) {
	if g.state == nil {
		return
	}
	t := lerpFactor(dt)
	lpX, lpY := float64(g.state.Session.LocalX), float64(g.state.Session.LocalY)

	MapInterpolate(&g.mapHX, &g.mapHY, lpX, lpY, t)

	if g.state.Harvestables != nil {
		snap := g.state.Harvestables.Snapshot()
		entries := make([]idPos, len(snap))
		for i, h := range snap {
			entries[i] = idPos{id: h.ID, x: h.PosX, y: h.PosY}
		}
		g.harvestablePos.sync(entries, lpX, lpY, t)
	}

	if g.state.Mobs != nil {
		snap := g.state.Mobs.MobSnapshot()
		entries := make([]idPos, len(snap))
		for i, m := range snap {
			entries[i] = idPos{id: m.ID, x: m.PosX, y: m.PosY}
		}
		g.mobPos.sync(entries, lpX, lpY, t)

		mists := g.state.Mobs.MistSnapshot()
		mistEntries := make([]idPos, len(mists))
		for i, m := range mists {
			mistEntries[i] = idPos{id: m.ID, x: m.PosX, y: m.PosY}
		}
		g.mistPos.sync(mistEntries, lpX, lpY, t)
	}

	if g.state.Chests != nil {
		snap := g.state.Chests.Snapshot()
		entries := make([]idPos, len(snap))
		for i, c := range snap {
			entries[i] = idPos{id: c.ID, x: c.PosX, y: c.PosY}
		}
		g.chestPos.sync(entries, lpX, lpY, t)
	}

	if g.state.Dungeons != nil {
		snap := g.state.Dungeons.Snapshot()
		entries := make([]idPos, len(snap))
		for i, d := range snap {
			entries[i] = idPos{id: d.ID, x: d.PosX, y: d.PosY}
		}
		g.dungeonPos.sync(entries, lpX, lpY, t)
	}

	if g.state.Fishing != nil {
		snap := g.state.Fishing.Snapshot()
		entries := make([]idPos, len(snap))
		for i, f := range snap {
			entries[i] = idPos{id: f.ID, x: f.PosX, y: f.PosY}
		}
		g.fishPos.sync(entries, lpX, lpY, t)
	}

	if g.state.LocalTreasures != nil {
		snap := g.state.LocalTreasures.Snapshot()
		entries := make([]idPos, len(snap))
		for i, lt := range snap {
			entries[i] = idPos{id: lt.ID, x: lt.PosX, y: lt.PosY}
		}
		g.localTreasurePos.sync(entries, lpX, lpY, t)
	}

	if g.state.MistsDungeon != nil {
		snap := g.state.MistsDungeon.Snapshot()
		entries := make([]idPos, len(snap))
		for i, m := range snap {
			entries[i] = idPos{id: m.ID, x: m.PosX, y: m.PosY}
		}
		g.mistsDungeonPos.sync(entries, lpX, lpY, t)
	}

	if g.state.WispCage != nil {
		snap := g.state.WispCage.Snapshot()
		entries := make([]idPos, len(snap))
		for i, w := range snap {
			entries[i] = idPos{id: w.ID, x: w.PosX, y: w.PosY}
		}
		g.wispCagePos.sync(entries, lpX, lpY, t)
	}
}

// screenPos resolves an already-interpolated entity to its current screen position, or
// ok=false if it isn't tracked yet (e.g. the very first frame after it spawned, before Update
// has run once).
func (g *Game) screenPos(tracker *posTracker, id int, zoom, center float64) (sx, sy float32, ok bool) { //nolint:unparam // zoom is currently always called with 1.0 (no zoom control wired up yet, see Draw's TODO) but is a real, already-varying parameter of TransformPoint - keeping it threaded through avoids re-plumbing 8 call sites once zoom exists
	e := tracker.get(id)
	if e == nil {
		return 0, 0, false
	}
	x, y := TransformPoint(e.HX, e.HY, zoom, center)
	return float32(x), float32(y), true
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Clear() // fully transparent - SetScreenTransparent handles per-pixel alpha compositing

	center := float32(g.width) / 2
	centerF := float64(center)
	// zoom scales with window size (not fixed) so resizing rescales the view instead of cropping
	// it: the full detection range that was visible at defaultWindow stays visible at any window
	// size, just rendered smaller/larger. settingOverlayZoom (edited from the native settings
	// client's Settings page) multiplies on top of that, defaulting to 1.0 (no extra zoom).
	zoom := float64(g.width) / defaultWindow * g.settings.getNumber("settingOverlayZoom", 1.0)

	g.drawMapBackground(screen, zoom, centerF)

	if g.state == nil {
		g.drawHUD(screen)
		return
	}

	if g.state.Harvestables != nil {
		for _, h := range g.state.Harvestables.Snapshot() {
			if h.Size <= 0 {
				continue // depleted/already-empty - mirrors HarvestablesDrawing.js's own
				// `if (harvestableOne.size <= 0) continue`, a defensive draw-time guard kept even
				// though the state layer is meant to remove a depleted node (UpdateHarvested/
				// HandleHarvestableChangeState) - this client never had the same guard
			}
			if !g.state.Harvestables.ShouldRender(h) {
				continue // tier/enchant not checked on the Resources page - see ShouldRender's doc
			}
			if sx, sy, ok := g.screenPos(g.harvestablePos, h.ID, zoom, centerF); ok {
				drawHarvestable(screen, sx, sy, h)
			}
		}
	}

	if g.state.Chests != nil {
		for _, c := range g.state.Chests.Snapshot() {
			if !shouldRenderChest(c, g.settings.isOn) {
				continue
			}
			if sx, sy, ok := g.screenPos(g.chestPos, c.ID, zoom, centerF); ok {
				drawChest(screen, sx, sy)
			}
		}
	}

	if g.state.Dungeons != nil {
		for _, d := range g.state.Dungeons.Snapshot() {
			if !shouldRenderDungeon(d, g.settings.isOn) {
				continue
			}
			if sx, sy, ok := g.screenPos(g.dungeonPos, d.ID, zoom, centerF); ok {
				drawDungeon(screen, sx, sy, d)
			}
		}
	}

	if g.settings.isOn("settingFishing") && g.state.Fishing != nil {
		for _, f := range g.state.Fishing.Snapshot() {
			if sx, sy, ok := g.screenPos(g.fishPos, f.ID, zoom, centerF); ok {
				drawFish(screen, sx, sy)
			}
		}
	}

	if g.settings.isOn("settingLocalTreasures") && g.state.LocalTreasures != nil {
		for _, lt := range g.state.LocalTreasures.Snapshot() {
			if sx, sy, ok := g.screenPos(g.localTreasurePos, lt.ID, zoom, centerF); ok {
				drawLocalTreasure(screen, sx, sy)
			}
		}
	}

	if g.settings.isOnDefault("settingShowKnightfallAbbey", true) && g.state.MistsDungeon != nil {
		for _, m := range g.state.MistsDungeon.Snapshot() {
			if sx, sy, ok := g.screenPos(g.mistsDungeonPos, m.ID, zoom, centerF); ok {
				drawMistsDungeonPortal(screen, sx, sy)
			}
		}
	}

	if g.settings.isOn("settingCage") && g.state.WispCage != nil {
		for _, w := range g.state.WispCage.Snapshot() {
			if sx, sy, ok := g.screenPos(g.wispCagePos, w.ID, zoom, centerF); ok {
				drawWispCage(screen, sx, sy)
			}
		}
	}

	if g.state.Mobs != nil {
		for _, mist := range g.state.Mobs.MistSnapshot() {
			if !shouldRenderMist(mist, g.settings.isOn) {
				continue
			}
			if sx, sy, ok := g.screenPos(g.mistPos, mist.ID, zoom, centerF); ok {
				drawMistWisp(screen, sx, sy)
			}
		}

		for _, m := range g.state.Mobs.MobSnapshot() {
			if !shouldRenderMob(m, g.settings.isOn, g.settings.getNumber) {
				continue // a hostile/drone/event mob type hidden or below-threshold - see
				// shouldRenderMob's doc
			}
			if !g.state.Mobs.ShouldRender(m) {
				continue // a living-resource (skinnable/harvestable) mob whose tier/enchant isn't
				// checked on the Resources page - see MobsState.ShouldRender's doc
			}
			if sx, sy, ok := g.screenPos(g.mobPos, m.ID, zoom, centerF); ok {
				drawMob(screen, sx, sy, m)
			}
		}
	}

	// Local player: fixed at screen center - the world moves around them, matching
	// CanvasManager.js's setupOurPlayerCanvas convention.
	vector.FillCircle(screen, center, center, 6, color.RGBA{R: 60, G: 140, B: 255, A: 255}, true)

	g.drawAlerts(screen)
	g.drawResizeHandle(screen)
	g.drawHUD(screen)
}

// drawAlerts renders settingFlash's one-shot red screen flash and settingFlashDangerousPlayer's
// pulsing border while any hostile player is currently a tracked threat (PlayersState.
// ThreatPlayers) - both drawn last/on top so they're never occluded by map/entities.
func (g *Game) drawAlerts(screen *ebiten.Image) {
	if g.state == nil {
		return
	}
	if time.Now().Before(g.flashUntil) {
		vector.FillRect(screen, 0, 0, float32(g.width), float32(g.height), color.RGBA{R: 255, A: 70}, false)
	}
	if g.state.Players != nil && g.settings.isOn("settingFlashDangerousPlayer") {
		if threats := g.state.Players.ThreatPlayers(); len(threats) > 0 {
			g.drawThreatBorder(screen)
		}
	}
}

// drawThreatBorder mirrors the web app's pulsating red border: alpha oscillates via a sine wave
// so it visibly pulses rather than sitting at a flat intensity.
func (g *Game) drawThreatBorder(screen *ebiten.Image) {
	pulse := (math.Sin(float64(time.Now().UnixMilli())/200) + 1) / 2 // 0..1
	alpha := uint8(80 + pulse*150)
	c := color.RGBA{R: 220, A: alpha}
	const thickness = 6
	w, h := float32(g.width), float32(g.height)
	vector.StrokeRect(screen, thickness/2, thickness/2, w-thickness, h-thickness, thickness, c, true)
}

// drawResizeHandle draws a small diagonal grip in the bottom-right corner (see resizeHandleSize)
// so there's a visible affordance for the click-drag-to-resize zone on this otherwise
// undecorated window. Hidden during click-through since nothing here is clickable then anyway.
func (g *Game) drawResizeHandle(screen *ebiten.Image) {
	if g.clickThrough {
		return
	}
	x, y := float32(g.width), float32(g.height)
	grip := color.RGBA{R: 255, G: 255, B: 255, A: 160}
	for i := float32(4); i < resizeHandleSize; i += 5 {
		vector.StrokeLine(screen, x-i, y-2, x-2, y-i, 1.5, grip, true)
	}
}

// drawMapBackground mirrors MapDrawing.js's draw()+DrawImageMap(): renders the current zone's
// pre-rendered map tile, positioned/rotated/scaled via MapScreenTransform so it stays aligned
// under the entities TransformPoint draws on top of it.
func (g *Game) drawMapBackground(screen *ebiten.Image, zoomLevel, canvasCenter float64) {
	if g.state == nil || g.state.Session == nil || g.state.Zones == nil {
		return
	}
	// settingOverlayMapOpacity is a 0-100 percentage (edited from the native settings client's
	// Settings page), defaulting to 100 (fully opaque, today's original behavior).
	opacity := g.settings.getNumber("settingOverlayMapOpacity", 100)
	if opacity <= 0 {
		return
	}
	zoneID := g.state.Session.CurrentZoneID
	if zoneID == "" {
		return
	}
	zone, ok := g.state.Zones.GetZone(zoneID)
	if !ok {
		return
	}
	name := mapImageName(zone.File)
	if name == "" {
		return
	}
	img := g.mapImages.get(name)
	if img == nil {
		return
	}

	extent, centerX, centerY := mapAssetExtentCenter(zone)
	screenX, screenY, size := MapScreenTransform(g.mapHX, g.mapHY, centerX, centerY, extent, zoomLevel, canvasCenter)

	bounds := img.Bounds()
	nativeW, nativeH := float64(bounds.Dx()), float64(bounds.Dy())
	if nativeW == 0 || nativeH == 0 {
		return
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(size/nativeW, size/nativeH)
	op.GeoM.Translate(-size/2, -size/2)
	op.GeoM.Rotate(mapRotationRad)
	op.GeoM.Scale(1, -1)
	op.GeoM.Translate(screenX, screenY)
	if opacity < 100 {
		op.ColorScale.ScaleAlpha(float32(opacity / 100))
	}
	screen.DrawImage(img, op)
}

func (g *Game) drawHUD(screen *ebiten.Image) {
	passthroughState := "OFF"
	if g.clickThrough {
		passthroughState = "ON"
	}
	zone := "?"
	mobCount, harvCount := 0, 0
	if g.state != nil {
		if g.state.Session != nil && g.state.Session.CurrentZoneID != "" {
			zone = g.state.Session.CurrentZoneID
			if g.state.Zones != nil {
				if z, ok := g.state.Zones.GetZone(zone); ok {
					zone = z.Name
				}
			}
		}
		if g.state.Mobs != nil {
			mobCount = g.state.Mobs.MobLen()
		}
		if g.state.Harvestables != nil {
			harvCount = g.state.Harvestables.Len()
		}
	}
	var hud strings.Builder
	fmt.Fprintf(&hud, "OpenRadar overlay\nF9: click-through %s\nDrag to move, corner to resize\nZone: %s\nMobs: %d  Resources: %d",
		passthroughState, zone, mobCount, harvCount)
	ebitenutil.DebugPrint(screen, hud.String())
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	g.width, g.height = outsideWidth, outsideHeight
	return outsideWidth, outsideHeight
}

// harvestableColors mirrors DrawingUtils.js's getResourceCategoryColor.
var harvestableColors = map[string]color.RGBA{
	"Fiber": {R: 0x4C, G: 0xAF, B: 0x50, A: 255},
	"Hide":  {R: 0xA1, G: 0x88, B: 0x7F, A: 255},
	"Log":   {R: 0x8D, G: 0x6E, B: 0x63, A: 255},
	"Ore":   {R: 0x42, G: 0xA5, B: 0xF5, A: 255},
	"Rock":  {R: 0x9C, G: 0x27, B: 0xB0, A: 255},
}

// enchantRingColors mirrors Albion Online's own item-quality/enchantment border color
// convention (green/blue/purple/gold for E1-E4) - not ported from any existing web drawing (the
// web app's badge mode only shows the enchant as a "+N" text label, not a color, and its default
// icon mode encodes it in the sprite image itself, which this client doesn't use), added as a
// native-overlay-only visual so an enchanted node is recognizable at a glance without reading
// text. E0 (unenchanted) gets no ring, matching the game's own convention of a plain border for
// normal-quality items.
var enchantRingColors = map[int]color.RGBA{
	1: {R: 0x1E, G: 0xC8, B: 0x1E, A: 255}, // E1 green
	2: {R: 0x3C, G: 0x8C, B: 0xFF, A: 255}, // E2 blue
	3: {R: 0xAA, G: 0x3C, B: 0xDC, A: 255}, // E3 purple
	4: {R: 0xFF, G: 0xA5, B: 0x00, A: 255}, // E4 gold
}

// drawEnchantRing draws the enchantment-color ring described above around a size×size square
// centered at (x,y) - a no-op for E0/unrecognized levels.
func drawEnchantRing(screen *ebiten.Image, x, y, size float32, enchant int) {
	ringColor, ok := enchantRingColors[enchant]
	if !ok {
		return
	}
	const thickness = float32(2)
	ringSize := size + 4
	vector.StrokeRect(screen, x-ringSize/2, y-ringSize/2, ringSize, ringSize, thickness, ringColor, true)
}

// drawEnchantRingCircle is drawEnchantRing for circle-shaped markers (living resources, drawn as
// circles via drawMob, unlike static harvestables' squares).
func drawEnchantRingCircle(screen *ebiten.Image, x, y, radius float32, enchant int) {
	ringColor, ok := enchantRingColors[enchant]
	if !ok {
		return
	}
	const thickness = float32(2)
	vector.StrokeCircle(screen, x, y, radius+3, thickness, ringColor, true)
}

func drawHarvestable(screen *ebiten.Image, x, y float32, h radarstate.Harvestable) {
	c, ok := harvestableColors[h.StringType]
	if !ok {
		c = color.RGBA{R: 0x41, G: 0x69, B: 0xE1, A: 255} // default blue, matches DrawCustomImage's fallback dot
	}
	const size = float32(8)
	vector.FillRect(screen, x-size/2, y-size/2, size, size, c, true)
	drawEnchantRing(screen, x, y, size, h.Charges)
}

// mobColors gives each EnemyType a distinct color - a simple first pass standing in for
// DrawingUtils.js's per-type icon set.
var mobColors = map[radarstate.EnemyType]color.RGBA{
	radarstate.EnemyBoss:              {R: 220, G: 20, B: 20, A: 255},
	radarstate.EnemyMiniBoss:          {R: 255, G: 140, B: 0, A: 255},
	radarstate.EnemyEnchanted:         {R: 180, G: 60, B: 220, A: 255},
	radarstate.EnemyNormal:            {R: 200, G: 200, B: 200, A: 255},
	radarstate.EnemyMedium:            {R: 220, G: 220, B: 120, A: 255},
	radarstate.EnemyLivingHarvestable: {R: 0x4C, G: 0xAF, B: 0x50, A: 255},
	radarstate.EnemyLivingSkinnable:   {R: 0xA1, G: 0x88, B: 0x7F, A: 255},
}

func drawMob(screen *ebiten.Image, x, y float32, m radarstate.Mob) {
	c, ok := mobColors[m.Type]
	if !ok {
		c = color.RGBA{R: 150, G: 150, B: 150, A: 255}
	}
	const radius = float32(7)
	vector.FillCircle(screen, x, y, radius, c, true)
	if m.Type == radarstate.EnemyLivingHarvestable || m.Type == radarstate.EnemyLivingSkinnable {
		drawEnchantRingCircle(screen, x, y, radius, m.EnchantmentLevel)
	}
}

// shouldRenderMob is a draw-time port of MobsDrawing.js's invalidate() branch selection (the
// non-living-resource branches - living resources are MobsState.ShouldRender's job, called
// separately, see its own doc comment). EnemyLivingHarvestable/EnemyLivingSkinnable always
// report true here since their gate lives there instead.
func shouldRenderMob(m radarstate.Mob, isOn func(string) bool, getNumber func(string, float64) float64) bool {
	switch {
	case m.Type == radarstate.EnemyLivingHarvestable || m.Type == radarstate.EnemyLivingSkinnable:
		return true
	case m.Type >= radarstate.EnemyNormal && m.Type <= radarstate.EnemyBoss:
		if !m.Identified {
			if !isOn("settingShowUnmanagedEnemies") {
				return false
			}
		} else if settingName := radarstate.SettingNameForEnemyType(m.Type); settingName != "" && !isOn(settingName) {
			return false
		}
		if isOn("settingShowMinimumHealthEnemies") {
			threshold := getNumber("settingTextMinimumHealthEnemies", 2100)
			if float64(m.MaxHealth) < threshold {
				return false
			}
		}
		return true
	case m.Type == radarstate.EnemyDrone:
		return isOn("settingAvaloneDrones")
	case m.Type == radarstate.EnemyEvents:
		return isOn("settingShowEventEnemies")
	default: // EnemyMistBoss: no filter in the web app either
		return true
	}
}
