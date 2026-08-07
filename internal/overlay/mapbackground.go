package overlay

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/webp"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

// mapRotationRad is numerically the same value as screenRotation, but used here as a genuine
// trigonometric rotation angle (matching web/scripts/drawings/MapsDrawing.js's ctx.rotate(...)
// call) - unlike TransformPoint, which (faithfully reproducing a quirk already present in the
// shipped JS) uses that same number as a plain scalar multiplier, not an actual rotation. Both
// behaviors are kept exactly as they ship today rather than "fixed" to be consistent with each
// other, since that's what the web radar's own map-vs-entity alignment already looks like.
const mapRotationRad = -0.785398

// mapImageCache lazily loads and decodes zone map tiles (web/images/Maps/*.webp) from disk,
// caching both hits and misses (a nil entry means "tried, unavailable" - matches
// DrawImageMap's own "no image, skip silently" behavior rather than retrying every frame).
type mapImageCache struct {
	dir   string
	cache map[string]*ebiten.Image
}

func newMapImageCache(appDir string) *mapImageCache {
	return &mapImageCache{
		dir:   filepath.Join(appDir, "web", "images", "Maps"),
		cache: make(map[string]*ebiten.Image),
	}
}

func (c *mapImageCache) get(name string) *ebiten.Image {
	if img, ok := c.cache[name]; ok {
		return img
	}
	img := c.load(name)
	c.cache[name] = img
	return img
}

func (c *mapImageCache) load(name string) *ebiten.Image {
	f, err := os.Open(filepath.Join(c.dir, name+".webp"))
	if err != nil {
		return nil
	}
	defer f.Close()

	src, err := webp.Decode(f)
	if err != nil {
		return nil
	}
	return ebiten.NewImageFromImage(src)
}

// mapImageName mirrors MapDrawing.js's draw(): Roads of Avalon passage/tunnel instances have a
// per-instance zone id (e.g. "PSG-0039#2") but only one downloaded tile per base zone
// ("PSG-0039.webp"), named from zone.file, not zone.id - truncate at the first "_" the same
// way the downloader does (issue #15).
func mapImageName(zoneFile string) string {
	if zoneFile == "" {
		return ""
	}
	if idx := strings.IndexByte(zoneFile, '_'); idx >= 0 {
		return zoneFile[:idx]
	}
	return zoneFile
}

// mapAssetExtentCenter mirrors ZonesDatabase.js's getMapAssetExtent/getMapAssetCenter: falls
// back to a based-on-nothing-else default extent (825, matching the JS fallback) when a zone's
// bounds are missing entirely (zero-value, since gamedata.Zone leaves Bounds unset rather than
// erroring for a zone with no bounds field).
func mapAssetExtentCenter(zone gamedata.Zone) (extent, centerX, centerY float64) {
	b := zone.Bounds
	if b.Min == [2]float64{} && b.Max == [2]float64{} {
		return 825, 0, 0
	}
	extent = math.Max(b.Max[0]-b.Min[0], b.Max[1]-b.Min[1])
	centerX = (b.Min[0] + b.Max[0]) / 2
	centerY = (b.Min[1] + b.Max[1]) / 2
	return extent, centerX, centerY
}

// MapInterpolate mirrors MapDrawing.js's interpolate: unlike every entity (which tracks its
// position relative to the local player, see InterpolateEntity), the map background tracks the
// local player's own position directly, Y negated - lerped with the same ~100ms smoothing.
func MapInterpolate(hX, hY *float64, lpX, lpY, t float64) {
	*hX = Lerp(*hX, lpX, t)
	*hY = Lerp(*hY, -lpY, t)
}

// MapScreenTransform mirrors MapDrawing.js's draw() + DrawImageMap()'s canvas transform stack
// (scale(1,-1) -> translate(canvasCenter,-canvasCenter) -> rotate(mapRotationRad) ->
// translate(-adjX,adjY) -> drawImage centered at local origin), solved analytically for where
// the image's own center lands on screen and at what pixel size - see
// docs/technical/NATIVE_OVERLAY_CLIENT.md for the full derivation (this used a real rotation
// matrix, unlike TransformPoint's scalar approximation, so it could not simply reuse that
// function).
func MapScreenTransform(mapHX, mapHY, zoneCenterX, zoneCenterY, extent, zoomLevel, canvasCenter float64) (screenX, screenY, size float64) {
	scaleFactor := baseZoom * zoomLevel
	adjX := (mapHX - zoneCenterX) * scaleFactor
	adjY := (mapHY + zoneCenterY) * scaleFactor
	size = extent * scaleFactor

	x, y := -adjX, adjY
	sin, cos := math.Sincos(mapRotationRad)
	rx := x*cos - y*sin
	ry := x*sin + y*cos
	screenX = rx + canvasCenter
	screenY = -(ry - canvasCenter)
	return screenX, screenY, size
}
