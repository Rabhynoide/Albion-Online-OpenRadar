package overlay

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/nospy/albion-openradar/internal/radarstate"
)

// This file is a first functional pass standing in for web/scripts/drawings/*.js's per-type
// icon set: one flat color/shape per entity type rather than loaded sprite images - see
// docs/technical/NATIVE_OVERLAY_CLIENT.md.

func drawChest(screen *ebiten.Image, x, y float32) {
	const size = float32(9)
	vector.FillRect(screen, x-size/2, y-size/2, size, size, color.RGBA{R: 0xFF, G: 0xD7, B: 0x00, A: 255}, true)
}

// chestRaritySubstrings mirrors ChestsDrawing.js's own inline rarity detection - each rarity is
// matched by substring against the chest's (lowercased) name, checked in this exact order (green
// first, then blue/purple/yellow), gated by its own settingChest{Color} checkbox.
var chestRaritySubstrings = []struct {
	setting string
	subs    []string
}{
	{"settingChestGreen", []string{"standard", "green"}},
	{"settingChestBlue", []string{"uncommon", "blue"}},
	{"settingChestPurple", []string{"rare", "purple"}},
	{"settingChestYellow", []string{"legendary", "yellow"}},
}

// shouldRenderChest is a draw-time port of ChestsDrawing.js's invalidate() rarity gate.
func shouldRenderChest(c radarstate.Chest, isOn func(string) bool) bool {
	name := strings.ToLower(c.Name)
	for _, rarity := range chestRaritySubstrings {
		if !isOn(rarity.setting) {
			continue
		}
		for _, sub := range rarity.subs {
			if strings.Contains(name, sub) {
				return true
			}
		}
	}
	return false
}

// dungeonColors mirrors DungeonsHandler.js's setDrawNameByType intent (a distinct marker per
// dungeon family) as flat colors rather than the JS's per-enchant image set.
var dungeonColors = map[radarstate.DungeonType]color.RGBA{
	radarstate.DungeonSolo:      {R: 80, G: 140, B: 255, A: 255},
	radarstate.DungeonGroup:     {R: 170, G: 90, B: 220, A: 255},
	radarstate.DungeonCorrupted: {R: 200, G: 30, B: 30, A: 255},
	radarstate.DungeonHellgate:  {R: 120, G: 0, B: 0, A: 255},
	radarstate.DungeonMistSolo:  {R: 60, G: 200, B: 190, A: 255},
	radarstate.DungeonMistGroup: {R: 30, G: 130, B: 125, A: 255},
}

func drawDungeon(screen *ebiten.Image, x, y float32, d radarstate.Dungeon) {
	c, ok := dungeonColors[d.Type]
	if !ok {
		c = color.RGBA{R: 150, G: 150, B: 150, A: 255}
	}
	vector.StrokeCircle(screen, x, y, 8, 2, c, true)
}

// shouldRenderDungeon is a draw-time port of DungeonsHandler.js's addDungeon type/enchant gate -
// the JS applies this at ingestion time (an entry matching a disabled filter is never even
// added); the Go port always tracks every dungeon and filters here instead, so toggling a filter
// takes immediate effect on already-tracked entries too (see radarstate.Dungeon's own doc
// comment for why that's a deliberate improvement, not just a port). Mist portal dungeon types
// route through the Mists settings (settingMistSolo/Duo + settingMistE0-4), not the Dungeon
// settings, matching the JS's own routing.
func shouldRenderDungeon(d radarstate.Dungeon, isOn func(string) bool) bool {
	enchantKey := func(prefix string) string { return fmt.Sprintf("%sE%d", prefix, d.Enchant) }
	switch d.Type {
	case radarstate.DungeonSolo:
		return isOn("settingDungeonSolo") && isOn(enchantKey("settingDungeon"))
	case radarstate.DungeonGroup:
		return isOn("settingDungeonDuo") && isOn(enchantKey("settingDungeon"))
	case radarstate.DungeonCorrupted:
		return isOn("settingDungeonCorrupted")
	case radarstate.DungeonHellgate:
		return isOn("settingDungeonHellgate")
	case radarstate.DungeonMistSolo:
		return isOn("settingMistSolo") && isOn(enchantKey("settingMist"))
	case radarstate.DungeonMistGroup:
		return isOn("settingMistDuo") && isOn(enchantKey("settingMist"))
	default:
		return false
	}
}

func drawFish(screen *ebiten.Image, x, y float32) {
	vector.FillCircle(screen, x, y, 5, color.RGBA{R: 90, G: 190, B: 230, A: 255}, true)
}

// drawLocalTreasure mirrors LocalTreasuresHandler.js's "single shared icon in v1" (buried
// chests, temporary rich resources, smuggler piles, timed events are all visually identical).
func drawLocalTreasure(screen *ebiten.Image, x, y float32) {
	vector.FillCircle(screen, x, y, 7, color.RGBA{R: 218, G: 165, B: 32, A: 255}, true)
}

func drawMistsDungeonPortal(screen *ebiten.Image, x, y float32) {
	vector.FillCircle(screen, x, y, 8, color.RGBA{R: 150, G: 60, B: 220, A: 255}, true)
}

func drawWispCage(screen *ebiten.Image, x, y float32) {
	vector.FillCircle(screen, x, y, 6, color.RGBA{R: 210, G: 210, B: 60, A: 255}, true)
}

// drawMistWisp is the pre-portal "wisp sign" marker (MistsWispDrawing.js) - distinct from
// drawWispCage (a spawned, already-opened wisp cage): a smaller, dimmer marker since it's just an
// early warning that a portal may open nearby.
func drawMistWisp(screen *ebiten.Image, x, y float32) {
	vector.FillCircle(screen, x, y, 4, color.RGBA{R: 170, G: 140, B: 230, A: 220}, true)
}

// shouldRenderMist is a draw-time port of MistsWispDrawing.js's invalidate() gate: a master
// settingWispSpawn toggle, then the same enchant + solo/duo-type gate the Dungeons Mist types
// share (settingMistE0-4, settingMistSolo/Duo) - m.IsSolo is MobsHandler.js's `type == 0`.
func shouldRenderMist(m radarstate.Mist, isOn func(string) bool) bool {
	if !isOn("settingWispSpawn") {
		return false
	}
	if !isOn(fmt.Sprintf("settingMistE%d", m.Enchant)) {
		return false
	}
	if m.IsSolo {
		return isOn("settingMistSolo")
	}
	return isOn("settingMistDuo")
}
