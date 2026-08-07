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

// Game implements ebiten.Game - the native radar window. A first functional pass: renders the
// local player (fixed screen-center dot, matching the web radar's own "world moves, player
// doesn't" convention), harvestables, and mobs with distinct colors per category. Chests/
// Dungeons/Fishing/LocalTreasures/Mists/WispCages/cluster rings/health bars are follow-up work
// once this core loop is visually validated - see docs/technical/NATIVE_OVERLAY_CLIENT.md.
type Game struct {
	state     *State
	settings  *settingsPanel
	mapImages *mapImageCache

	harvestablePos map[int]*EntityPos
	mobPos         map[int]*EntityPos
	mapHX, mapHY   float64

	width, height int
	lastFrame     time.Time

	clickThrough bool
	f9WasDown    bool
}

func NewGame(state *State, appDir string) *Game {
	return &Game{
		state:          state,
		settings:       newSettingsPanel(appDir),
		mapImages:      newMapImageCache(appDir),
		harvestablePos: make(map[int]*EntityPos),
		mobPos:         make(map[int]*EntityPos),
		width:          defaultWindow,
		height:         defaultWindow,
		lastFrame:      time.Now(),
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
		live := make(map[int]struct{})
		for _, h := range g.state.Harvestables.Snapshot() {
			live[h.ID] = struct{}{}
			e := g.harvestablePos[h.ID]
			if e == nil {
				e = &EntityPos{}
				g.harvestablePos[h.ID] = e
			}
			e.PosX, e.PosY = float64(h.PosX), float64(h.PosY)
			InterpolateEntity(e, lpX, lpY, t)
		}
		pruneMissing(g.harvestablePos, live)
	}

	if g.state.Mobs != nil {
		live := make(map[int]struct{})
		for _, m := range g.state.Mobs.MobSnapshot() {
			live[m.ID] = struct{}{}
			e := g.mobPos[m.ID]
			if e == nil {
				e = &EntityPos{}
				g.mobPos[m.ID] = e
			}
			e.PosX, e.PosY = float64(m.PosX), float64(m.PosY)
			InterpolateEntity(e, lpX, lpY, t)
		}
		pruneMissing(g.mobPos, live)
	}
}

func pruneMissing(m map[int]*EntityPos, live map[int]struct{}) {
	for id := range m {
		if _, ok := live[id]; !ok {
			delete(m, id)
		}
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Clear() // fully transparent - SetScreenTransparent handles per-pixel alpha compositing

	center := float32(g.width) / 2
	zoom := 1.0 // TODO: wire to the settings panel's zoom control once it exists

	g.drawMapBackground(screen, zoom, float64(center))

	if g.state != nil && g.state.Harvestables != nil {
		for _, h := range g.state.Harvestables.Snapshot() {
			e := g.harvestablePos[h.ID]
			if e == nil {
				continue
			}
			sx, sy := TransformPoint(e.HX, e.HY, zoom, float64(center))
			drawHarvestable(screen, float32(sx), float32(sy), h)
		}
	}

	if g.state != nil && g.state.Mobs != nil {
		for _, m := range g.state.Mobs.MobSnapshot() {
			if settingID := radarstate.SettingNameForEnemyType(m.Type); settingID != "" && !g.settings.isOn(settingID) {
				continue // e.g. F4-toggled off "settingMiniBossEnemy" - living resources have no
				// dedicated setting (SettingNameForEnemyType returns "" for them) and always draw.
			}
			e := g.mobPos[m.ID]
			if e == nil {
				continue
			}
			sx, sy := TransformPoint(e.HX, e.HY, zoom, float64(center))
			drawMob(screen, float32(sx), float32(sy), m)
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
