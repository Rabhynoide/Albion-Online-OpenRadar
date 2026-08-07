// Package overlay is the native Ebiten radar window (internal/radarstate feeds it, no browser
// involved) - a Go port of the rendering/interaction logic in web/scripts/utils/RadarRenderer.js
// and DrawingUtils.js. This file ports DrawingUtils.js's pure math (screen transform,
// interpolation, unit conversion) - the parts that are meaningfully unit-testable without a
// graphics API. The actual Ebiten Draw() implementation (internal/overlay/game.go) is a
// functional pass (distinct shapes/colors per entity type) rather than a pixel-perfect port of
// DrawingUtils.js's canvas gradients/rounded-rect badges - that's UI polish, not ported.
// Resource clustering and health bars are deliberately out of scope (not needed for this
// client), unlike the web radar's own DrawingUtils.js/detectClusters.
package overlay

import "math"

const (
	scaleFactor    = 1.0
	baseZoom       = 4.0
	screenRotation = -0.785398
)

// RelativeScreenBearing mirrors DrawingUtils.js's relativeScreenBearing: expresses a
// raw-coordinate delta as a bearing in degrees (0=up/N, 90=right/E, 180=down/S, 270=left/W)
// matching the radar's own screen rotation - not true in-game North, just "which way on your
// radar screen".
func RelativeScreenBearing(dx, dy float64) float64 {
	screenDx := screenRotation * (dx - dy)
	screenDy := screenRotation * (dx + dy)
	deg := math.Atan2(screenDx, -screenDy) * 180 / math.Pi
	return math.Mod(deg+360, 360)
}

// compassLabels mirrors DrawingUtils.js's French-style hyphenated COMPASS_LABELS, matching the
// in-game UI's own compass label format.
var compassLabels = [8]string{"N", "N-E", "E", "S-E", "S", "S-O", "O", "N-O"}

// BearingToCompassLabel mirrors bearingToCompassLabel.
func BearingToCompassLabel(bearingDeg float64) string {
	index := int(math.Round(bearingDeg/45)) % 8
	if index < 0 {
		index += 8
	}
	return compassLabels[index]
}

// Lerp mirrors DrawingUtils.js's lerp.
func Lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// TransformPoint mirrors transformPoint: converts a game-world-relative (x, y) offset into
// screen pixel coordinates, applying the radar's fixed screen rotation and zoom, centered on
// canvasCenter (half the render surface's pixel size).
func TransformPoint(x, y, zoomLevel, canvasCenter float64) (screenX, screenY float64) {
	newX := x*screenRotation - y*screenRotation
	newY := x*screenRotation + y*screenRotation
	zoom := baseZoom * zoomLevel
	newX *= zoom
	newY *= zoom
	return newX + canvasCenter, newY + canvasCenter
}

// EntityPos is the minimal shape InterpolateEntity needs - kept independent of any single
// radarstate entity type since Harvestable/Mob/Chest/etc. all need the exact same
// interpolation applied to their own (PosX, PosY) via their own hX/hY screen-position state.
type EntityPos struct {
	PosX, PosY float64
	HX, HY     float64 // screen-relative interpolated position, mutated in place
}

// InterpolateEntity mirrors DrawingUtils.js's interpolateEntity: smooths an entity's on-screen
// position toward its latest known game position (lpX/lpY = local player's game position).
// On first observation (hX/hY both still zero), it jumps straight to the target instead of
// lerping from the origin - avoids every entity visibly sliding in from the center on spawn.
func InterpolateEntity(e *EntityPos, lpX, lpY, t float64) {
	hX := -1*e.PosX + lpX
	hY := e.PosY - lpY

	if e.HX == 0 && e.HY == 0 {
		e.HX, e.HY = hX, hY
	}
	e.HX = Lerp(e.HX, hX, t)
	e.HY = Lerp(e.HY, hY, t)
}

// CalculateDistance mirrors calculateDistance (Euclidean, in raw game units).
func CalculateDistance(x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	return math.Sqrt(dx*dx + dy*dy)
}

// ConvertGameUnitsToMeters mirrors convertGameUnitsToMeters.
func ConvertGameUnitsToMeters(gameUnits float64) int {
	return int(math.Round(gameUnits / 3 * scaleFactor))
}

// MetersToGameUnits mirrors metersToGameUnits.
func MetersToGameUnits(meters float64) float64 {
	if meters <= 0 {
		return 0
	}
	return math.Ceil(meters / scaleFactor * 3)
}
