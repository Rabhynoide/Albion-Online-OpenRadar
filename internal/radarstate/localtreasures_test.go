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
	}, nil)

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
}

// @verified: mirrors LocalTreasuresHandler.js's EXCLUDED_LABEL_PREFIXES - a SPECIAL_EVENT_*
// entry sharing its id with a real live mob (pcap-confirmed 2026-07-30) must be dropped here to
// avoid a duplicate encounter marker.
func TestLocalTreasuresState_SpecialEventLabelExcludedWhenMobTracked(t *testing.T) {
	s := NewLocalTreasuresState()
	hasMob := func(id int) bool { return id == 1 }
	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{1, 2},
		5: []float32{10, 20, 30, 40},
		6: []int32{0, 0},
		7: []int32{0, 0},
		8: []string{"SPECIAL_EVENT_1", "ANNIVERSARY_EVENT"},
	}, hasMob)

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1 (SPECIAL_EVENT_1 excluded)", len(snap))
	}
	if snap[0].ID != 2 {
		t.Errorf("remaining entry ID = %d, want 2", snap[0].ID)
	}
}

// @verified 2026-08-08 (issue #164/#163): a real pcap capture of a buried-treasure decor
// ("destroy to spawn loot") showed a SPECIAL_EVENT_1 entry (id 102115) that never appeared
// anywhere else in the capture - no matching NewMob, not even a Leave for that id - yet the old
// blanket label exclusion hid it from the radar regardless. Only a live mob-id cross-check tells
// this case apart from the lead-up-mob case above.
func TestLocalTreasuresState_SpecialEventLabelDrawnWhenNoMobTracked(t *testing.T) {
	s := NewLocalTreasuresState()
	hasMob := func(id int) bool { return false }
	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{102115},
		5: []float32{-298, 299},
		6: []int32{0},
		7: []int32{0},
		8: []string{"SPECIAL_EVENT_1"},
	}, hasMob)

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1 (no matching mob, must stay drawable)", len(snap))
	}
	if snap[0].ID != 102115 {
		t.Errorf("entry ID = %d, want 102115", snap[0].ID)
	}
}

// A nil hasMob (no mob list available at all) must fail open - stay drawable - rather than hide
// the entity, which is the whole point of this fix.
func TestLocalTreasuresState_SpecialEventLabelDrawnWhenHasMobNil(t *testing.T) {
	s := NewLocalTreasuresState()
	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{102115}, 5: []float32{-298, 299}, 6: []int32{0}, 7: []int32{0},
		8: []string{"SPECIAL_EVENT_1"},
	}, nil)

	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}
}

func TestLocalTreasuresState_RemoveAndClear(t *testing.T) {
	s := NewLocalTreasuresState()
	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{1}, 5: []float32{0, 0}, 6: []int32{0}, 7: []int32{0}, 8: []string{"X"},
	}, nil)
	s.Remove(1)
	if s.Len() != 0 {
		t.Error("Len() should be 0 after Remove")
	}

	s.HandleLocalTreasuresUpdate(Params{
		4: []int32{2}, 5: []float32{0, 0}, 6: []int32{0}, 7: []int32{0}, 8: []string{"X"},
	}, nil)
	s.Clear()
	if s.Len() != 0 {
		t.Error("Len() should be 0 after Clear")
	}
}
