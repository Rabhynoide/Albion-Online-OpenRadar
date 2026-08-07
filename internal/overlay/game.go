package overlay

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/nospy/albion-openradar/internal/radarstate"
)

const (
	targetTPS     = 30 // matches RadarRenderer.js's TARGET_FPS
	windowTitle   = "OpenRadar Overlay"
	defaultWindow = 500
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

	harvestablePos   *posTracker
	mobPos           *posTracker
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
}

func NewGame(state *State, appDir string) *Game {
	return &Game{
		state:            state,
		settings:         newSettingsPanel(appDir),
		mapImages:        newMapImageCache(appDir),
		harvestablePos:   newPosTracker(),
		mobPos:           newPosTracker(),
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
// normal Ebiten key listener. appDir is where settings-sync.json lives (see settings.go).
func Run(state *State, appDir string) error {
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowSize(defaultWindow, defaultWindow)
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetTPS(targetTPS)

	return ebiten.RunGameWithOptions(NewGame(state, appDir), &ebiten.RunGameOptions{ScreenTransparent: true})
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

	g.settings.refresh()
	g.updateInterpolation(dt)
	return nil
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
	zoom := 1.0 // TODO: wire to the settings panel's zoom control once it exists

	g.drawMapBackground(screen, zoom, centerF)

	if g.state == nil {
		g.drawHUD(screen)
		return
	}

	if g.state.Harvestables != nil {
		for _, h := range g.state.Harvestables.Snapshot() {
			if sx, sy, ok := g.screenPos(g.harvestablePos, h.ID, zoom, centerF); ok {
				drawHarvestable(screen, sx, sy, h)
			}
		}
	}

	if g.state.Chests != nil {
		for _, c := range g.state.Chests.Snapshot() {
			if sx, sy, ok := g.screenPos(g.chestPos, c.ID, zoom, centerF); ok {
				drawChest(screen, sx, sy)
			}
		}
	}

	if g.state.Dungeons != nil {
		for _, d := range g.state.Dungeons.Snapshot() {
			if sx, sy, ok := g.screenPos(g.dungeonPos, d.ID, zoom, centerF); ok {
				drawDungeon(screen, sx, sy, d)
			}
		}
	}

	if g.state.Fishing != nil {
		for _, f := range g.state.Fishing.Snapshot() {
			if sx, sy, ok := g.screenPos(g.fishPos, f.ID, zoom, centerF); ok {
				drawFish(screen, sx, sy)
			}
		}
	}

	if g.state.LocalTreasures != nil {
		for _, lt := range g.state.LocalTreasures.Snapshot() {
			if sx, sy, ok := g.screenPos(g.localTreasurePos, lt.ID, zoom, centerF); ok {
				drawLocalTreasure(screen, sx, sy)
			}
		}
	}

	if g.state.MistsDungeon != nil {
		for _, m := range g.state.MistsDungeon.Snapshot() {
			if sx, sy, ok := g.screenPos(g.mistsDungeonPos, m.ID, zoom, centerF); ok {
				drawMistsDungeonPortal(screen, sx, sy)
			}
		}
	}

	if g.state.WispCage != nil {
		for _, w := range g.state.WispCage.Snapshot() {
			if sx, sy, ok := g.screenPos(g.wispCagePos, w.ID, zoom, centerF); ok {
				drawWispCage(screen, sx, sy)
			}
		}
	}

	if g.state.Mobs != nil {
		for _, m := range g.state.Mobs.MobSnapshot() {
			if settingID := radarstate.SettingNameForEnemyType(m.Type); settingID != "" && !g.settings.isOn(settingID) {
				continue // e.g. a mob type hidden via the web Enemies page - living resources have
				// no dedicated setting (SettingNameForEnemyType returns "") and always draw.
			}
			if sx, sy, ok := g.screenPos(g.mobPos, m.ID, zoom, centerF); ok {
				drawMob(screen, sx, sy, m)
			}
		}
	}

	// Local player: fixed at screen center - the world moves around them, matching
	// CanvasManager.js's setupOurPlayerCanvas convention.
	vector.FillCircle(screen, center, center, 6, color.RGBA{R: 60, G: 140, B: 255, A: 255}, true)

	g.drawHUD(screen)
}

// drawMapBackground mirrors MapDrawing.js's draw()+DrawImageMap(): renders the current zone's
// pre-rendered map tile, positioned/rotated/scaled via MapScreenTransform so it stays aligned
// under the entities TransformPoint draws on top of it.
func (g *Game) drawMapBackground(screen *ebiten.Image, zoomLevel, canvasCenter float64) {
	if g.state == nil || g.state.Session == nil || g.state.Zones == nil {
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
	fmt.Fprintf(&hud, "OpenRadar overlay\nF9: click-through %s\nZone: %s\nMobs: %d  Resources: %d",
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

func drawHarvestable(screen *ebiten.Image, x, y float32, h radarstate.Harvestable) {
	c, ok := harvestableColors[h.StringType]
	if !ok {
		c = color.RGBA{R: 0x41, G: 0x69, B: 0xE1, A: 255} // default blue, matches DrawCustomImage's fallback dot
	}
	const size = float32(8)
	vector.FillRect(screen, x-size/2, y-size/2, size, size, c, true)
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
	vector.FillCircle(screen, x, y, 7, c, true)
}
