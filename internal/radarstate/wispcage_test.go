package radarstate

import "testing"

func TestWispCageState_AddAndOpen(t *testing.T) {
	s := NewWispCageState()
	s.HandleNewCagedObject(Params{0: int32(1), 2: []float32{5, 6}, 4: "Wisp Cage"})

	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}
	cage := s.Snapshot()[0]
	if cage.ID != 1 || cage.PosX != 5 || cage.PosY != 6 || cage.Name != "Wisp Cage" {
		t.Errorf("cage = %+v", cage)
	}

	s.HandleCagedObjectStateUpdated(Params{0: int32(1)})
	if s.Len() != 0 {
		t.Error("an opened cage should be removed from the map")
	}
}

func TestWispCageState_MissingIDOrPositionIgnored(t *testing.T) {
	s := NewWispCageState()
	s.HandleNewCagedObject(Params{2: []float32{5, 6}})
	s.HandleNewCagedObject(Params{0: int32(1)})

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (both events missing required fields)", s.Len())
	}
}
