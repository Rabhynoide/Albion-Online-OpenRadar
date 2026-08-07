package overlay

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestBearingToCompassLabel(t *testing.T) {
	tests := []struct {
		bearing float64
		want    string
	}{
		{0, "N"}, {45, "N-E"}, {90, "E"}, {135, "S-E"}, {180, "S"}, {225, "S-O"}, {270, "O"}, {315, "N-O"},
		{360, "N"}, {5, "N"}, {40, "N-E"},
	}
	for _, tt := range tests {
		if got := BearingToCompassLabel(tt.bearing); got != tt.want {
			t.Errorf("BearingToCompassLabel(%v) = %q, want %q", tt.bearing, got, tt.want)
		}
	}
}

func TestLerp(t *testing.T) {
	if got := Lerp(0, 10, 0.5); got != 5 {
		t.Errorf("Lerp(0,10,0.5) = %v, want 5", got)
	}
	if got := Lerp(0, 10, 0); got != 0 {
		t.Errorf("Lerp(0,10,0) = %v, want 0", got)
	}
	if got := Lerp(0, 10, 1); got != 10 {
		t.Errorf("Lerp(0,10,1) = %v, want 10", got)
	}
}

func TestInterpolateEntity_FirstObservationJumpsToTarget(t *testing.T) {
	e := &EntityPos{PosX: 100, PosY: 50}
	InterpolateEntity(e, 0, 0, 0.1) // t doesn't matter on the very first call

	wantHX, wantHY := -1*100.0+0, 50.0-0
	if !almostEqual(e.HX, wantHX) || !almostEqual(e.HY, wantHY) {
		t.Errorf("first InterpolateEntity: HX=%v HY=%v, want HX=%v HY=%v (snap to target, no lerp from origin)", e.HX, e.HY, wantHX, wantHY)
	}
}

func TestInterpolateEntity_SubsequentCallsLerp(t *testing.T) {
	e := &EntityPos{PosX: 10, PosY: 10}
	InterpolateEntity(e, 0, 0, 1)       // first call snaps to the target: (-10, 10)
	InterpolateEntity(e, 100, 100, 0.5) // second call lerps halfway toward the new target

	// new target: hX = -10+100 = 90, hY = 10-100 = -90; halfway from (-10,10) to (90,-90) = (40,-40)
	if !almostEqual(e.HX, 40) || !almostEqual(e.HY, -40) {
		t.Errorf("HX=%v HY=%v, want HX=40 HY=-40", e.HX, e.HY)
	}
}

func TestCalculateDistance(t *testing.T) {
	if got := CalculateDistance(0, 0, 3, 4); got != 5 {
		t.Errorf("CalculateDistance = %v, want 5 (3-4-5 triangle)", got)
	}
}

func TestConvertGameUnitsToMeters(t *testing.T) {
	if got := ConvertGameUnitsToMeters(30); got != 10 {
		t.Errorf("ConvertGameUnitsToMeters(30) = %d, want 10", got)
	}
}

func TestMetersToGameUnits(t *testing.T) {
	if got := MetersToGameUnits(10); got != 30 {
		t.Errorf("MetersToGameUnits(10) = %v, want 30", got)
	}
	if got := MetersToGameUnits(0); got != 0 {
		t.Errorf("MetersToGameUnits(0) = %v, want 0", got)
	}
	if got := MetersToGameUnits(-5); got != 0 {
		t.Errorf("MetersToGameUnits(-5) = %v, want 0", got)
	}
}
