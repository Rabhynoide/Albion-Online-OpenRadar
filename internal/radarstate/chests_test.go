package radarstate

import "testing"

func TestChestsState_HandleNewLootChest(t *testing.T) {
	s := NewChestsState()
	s.HandleNewLootChest(Params{0: int32(1), 1: []float32{10, 20}, 3: "Green Chest", 5: int32(1)})

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	c := snap[0]
	if c.ID != 1 || c.PosX != 10 || c.PosY != 20 || c.Name != "Green Chest" || c.Rarity == nil || *c.Rarity != 1 {
		t.Errorf("chest = %+v", c)
	}
}

// @verified: mirrors ChestsHandler.js's Mist-chest name-shift - Parameters[3] mentioning "mist"
// means the real name is one slot later, at Parameters[4].
func TestChestsState_MistChestNameShiftsToParam4(t *testing.T) {
	s := NewChestsState()
	s.HandleNewLootChest(Params{0: int32(1), 1: []float32{0, 0}, 3: "Mist Chest Marker", 4: "Real Mist Chest Name"})

	snap := s.Snapshot()
	if snap[0].Name != "Real Mist Chest Name" {
		t.Errorf("Name = %q, want the Parameters[4] fallback name", snap[0].Name)
	}
}

func TestChestsState_NoRarityIsNil(t *testing.T) {
	s := NewChestsState()
	s.HandleNewLootChest(Params{0: int32(1), 1: []float32{0, 0}, 3: "Chest"})

	if s.Snapshot()[0].Rarity != nil {
		t.Error("Rarity should be nil when Parameters[5] is absent")
	}
}

func TestChestsState_DuplicateIDDoesNotOverwrite(t *testing.T) {
	s := NewChestsState()
	s.HandleNewLootChest(Params{0: int32(1), 1: []float32{10, 20}, 3: "First"})
	s.HandleNewLootChest(Params{0: int32(1), 1: []float32{99, 99}, 3: "Second"})

	if got := s.Snapshot()[0].Name; got != "First" {
		t.Errorf("Name = %q, want First (re-observation should only touch, not overwrite)", got)
	}
}

func TestChestsState_RemoveAndClear(t *testing.T) {
	s := NewChestsState()
	s.HandleNewLootChest(Params{0: int32(1), 1: []float32{0, 0}, 3: "A"})
	s.HandleNewLootChest(Params{0: int32(2), 1: []float32{0, 0}, 3: "B"})

	s.Remove(1)
	if s.Len() != 1 {
		t.Errorf("Len() = %d after Remove, want 1", s.Len())
	}

	s.Clear()
	if s.Len() != 0 {
		t.Errorf("Len() = %d after Clear, want 0", s.Len())
	}
}
