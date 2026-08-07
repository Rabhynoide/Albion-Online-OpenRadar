package overlay

import (
	"math"
	"testing"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

// @verified: mirrors MapDrawing.js's zoneFile.split('_')[0] truncation (issue #15) - Roads of
// Avalon passage/tunnel instances share one downloaded tile per base zone.
func TestMapImageName(t *testing.T) {
	tests := []struct {
		zoneFile, want string
	}{
		{"1000_CTY_FR_AUTO_T1_NON", "1000"},
		{"PSG-0039_something", "PSG-0039"},
		{"NoUnderscore", "NoUnderscore"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := mapImageName(tt.zoneFile); got != tt.want {
			t.Errorf("mapImageName(%q) = %q, want %q", tt.zoneFile, got, tt.want)
		}
	}
}

func TestMapAssetExtentCenter_UsesRealBounds(t *testing.T) {
	zone := gamedata.Zone{Bounds: gamedata.Bounds{Min: [2]float64{-305, -305}, Max: [2]float64{295, 295}}}

	extent, cx, cy := mapAssetExtentCenter(zone)

	if extent != 600 {
		t.Errorf("extent = %v, want 600 (295 - -305)", extent)
	}
	if cx != -5 || cy != -5 {
		t.Errorf("center = (%v,%v), want (-5,-5)", cx, cy)
	}
}

func TestMapAssetExtentCenter_FallsBackWhenBoundsMissing(t *testing.T) {
	zone := gamedata.Zone{} // zero-value Bounds

	extent, cx, cy := mapAssetExtentCenter(zone)

	if extent != 825 || cx != 0 || cy != 0 {
		t.Errorf("(extent,cx,cy) = (%v,%v,%v), want (825,0,0)", extent, cx, cy)
	}
}

func TestMapInterpolate_TracksLocalPlayerDirectlyWithYNegated(t *testing.T) {
	var hX, hY float64
	MapInterpolate(&hX, &hY, 100, 50, 1) // t=1 -> snaps straight to target

	if hX != 100 || hY != -50 {
		t.Errorf("hX=%v hY=%v, want hX=100 hY=-50", hX, hY)
	}
}

func TestMapInterpolate_Lerps(t *testing.T) {
	hX, hY := 0.0, 0.0
	MapInterpolate(&hX, &hY, 100, 0, 0.5)

	if hX != 50 {
		t.Errorf("hX = %v, want 50 (halfway lerp)", hX)
	}
}

// TestMapScreenTransform_SameZoneCenterYieldsCanvasCenterOnScreen exercises the core identity:
// when the map-tracking position exactly matches the zone's own asset center, the map's
// on-screen center should land exactly on the canvas center point, since adjX/adjY both
// collapse to zero.
func TestMapScreenTransform_SameZoneCenterYieldsCanvasCenterOnScreen(t *testing.T) {
	// mapHX==centerX and mapHY==-centerY makes adjX=0; adjY = (mapHY+centerY)*scale = 0 too.
	screenX, screenY, size := MapScreenTransform(10, -10, 10, 10, 600, 1, 250)

	if math.Abs(screenX-250) > 1e-9 || math.Abs(screenY-250) > 1e-9 {
		t.Errorf("screen = (%v,%v), want (250,250) (canvas center)", screenX, screenY)
	}
	if size != 600*4 { // extent * baseZoom * zoomLevel
		t.Errorf("size = %v, want %v", size, 600*4.0)
	}
}

func TestMapScreenTransform_ZoomScalesSize(t *testing.T) {
	_, _, size1 := MapScreenTransform(0, 0, 0, 0, 100, 1, 250)
	_, _, size2 := MapScreenTransform(0, 0, 0, 0, 100, 2, 250)

	if size2 != size1*2 {
		t.Errorf("size at zoom=2 (%v) should be double zoom=1 (%v)", size2, size1)
	}
}
