package radarstate

import "testing"

func TestClassifyDungeon(t *testing.T) {
	tests := []struct {
		name string
		want DungeonType
	}{
		{"MISTS_SOLO_YELLOW", DungeonMistSolo},
		{"MISTS_DUO_RED", DungeonMistGroup},
		{"CORRUPTED_SOLO_1", DungeonCorrupted}, // must win over "solo" despite containing it
		{"SOLO_STANDARD_1", DungeonSolo},
		{"HELLGATE_2V2_NON_LETHAL", DungeonHellgate},
		{"GROUP_5_STANDARD", DungeonGroup},
		{"STANDARD_GATED_5", DungeonGroup}, // no keyword match -> default group
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyDungeon(tt.name); got != tt.want {
				t.Errorf("classifyDungeon(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestDungeonsState_HandleNewRandomDungeonExit(t *testing.T) {
	s := NewDungeonsState()
	s.HandleNewRandomDungeonExit(Params{0: int32(1), 1: []float32{10, 20}, 3: "SOLO_STANDARD_1", 8: int32(2)})

	d := s.Snapshot()[0]
	if d.ID != 1 || d.PosX != 10 || d.PosY != 20 || d.Type != DungeonSolo || d.Enchant != 2 {
		t.Errorf("dungeon = %+v", d)
	}
}

// @verified: mirrors DungeonsHandler.js's comment "Post-Knightfall the Mist portal name moved
// from Parameters[3] to Parameters[15]".
func TestDungeonsState_NameFallsBackToParam15(t *testing.T) {
	s := NewDungeonsState()
	s.HandleNewRandomDungeonExit(Params{0: int32(1), 1: []float32{0, 0}, 15: "MISTS_SOLO_YELLOW"})

	if got := s.Snapshot()[0].Type; got != DungeonMistSolo {
		t.Errorf("Type = %v, want DungeonMistSolo (name read from Parameters[15] fallback)", got)
	}
}

func TestDungeonsState_RemoveAndClear(t *testing.T) {
	s := NewDungeonsState()
	s.HandleNewRandomDungeonExit(Params{0: int32(1), 1: []float32{0, 0}, 3: "GROUP_1"})
	s.Remove(1)
	if s.Len() != 0 {
		t.Error("Len() should be 0 after Remove")
	}
}
