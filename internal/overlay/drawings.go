package overlay

import (
	"image/color"

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
