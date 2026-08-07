package radarstate

import "testing"

func TestMistsDungeonState_AddRemove(t *testing.T) {
	s := NewMistsDungeonState()
	s.AddPortal(1, 10, 20, "Knightfall Abbey")

	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}
	p := s.Snapshot()[0]
	if p.ID != 1 || p.PosX != 10 || p.PosY != 20 || p.Name != "Knightfall Abbey" {
		t.Errorf("portal = %+v", p)
	}

	s.RemovePortal(1)
	if s.Len() != 0 {
		t.Error("Len() should be 0 after RemovePortal")
	}
}

func TestMistsDungeonState_AddDoesNotOverwrite(t *testing.T) {
	s := NewMistsDungeonState()
	s.AddPortal(1, 10, 20, "First")
	s.AddPortal(1, 99, 99, "Second")

	if got := s.Snapshot()[0].Name; got != "First" {
		t.Errorf("Name = %q, want First", got)
	}
}
