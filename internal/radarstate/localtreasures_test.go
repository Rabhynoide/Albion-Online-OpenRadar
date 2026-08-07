package radarstate

import "testing"

func TestLocalTreasuresState_HandleUpdate(t *testing.T) {
	s := NewLocalTreasuresState()
	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{1, 2},
		5: []float32{10, 20, 30, 40},
		6: []int32{100, 200},
		7: []int32{500, 600},
		8: []string{"BURIED_TREASURE", "ANNIVERSARY_EVENT"},
	})

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
}

// @verified: mirrors LocalTreasuresHandler.js's EXCLUDED_LABEL_PREFIXES - a SPECIAL_EVENT_*
// entry shares its id with a real NewMob (pcap-confirmed 2026-07-30), so it must be dropped
// here to avoid a duplicate encounter marker.
func TestLocalTreasuresState_SpecialEventLabelExcluded(t *testing.T) {
	s := NewLocalTreasuresState()
	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{1, 2},
		5: []float32{10, 20, 30, 40},
		6: []int32{0, 0},
		7: []int32{0, 0},
		8: []string{"SPECIAL_EVENT_1", "ANNIVERSARY_EVENT"},
	})

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1 (SPECIAL_EVENT_1 excluded)", len(snap))
	}
	if snap[0].ID != 2 {
		t.Errorf("remaining entry ID = %d, want 2", snap[0].ID)
	}
}

func TestLocalTreasuresState_RemoveAndClear(t *testing.T) {
	s := NewLocalTreasuresState()
	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{1}, 5: []float32{0, 0}, 6: []int32{0}, 7: []int32{0}, 8: []string{"X"},
	})
	s.Remove(1)
	if s.Len() != 0 {
		t.Error("Len() should be 0 after Remove")
	}

	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{2}, 5: []float32{0, 0}, 6: []int32{0}, 7: []int32{0}, 8: []string{"X"},
	})
	s.Clear()
	if s.Len() != 0 {
		t.Error("Len() should be 0 after Clear")
	}
}
