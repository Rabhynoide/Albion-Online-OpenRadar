package radarstate

import (
	"testing"
	"testing/fstest"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

func testHarvestablesDB(t *testing.T) *gamedata.HarvestablesDatabase {
	t.Helper()
	fsys := fstest.MapFS{
		"harvestables.json": mapFile(`{"WOOD": [{"tier":4,"item":"T4_WOOD","respawn":1,"harvest":1,"tool":false,"maxcharges":1,"startcharges":1,"chargeup":1}]}`),
	}
	db, err := gamedata.LoadHarvestables(fsys, "harvestables.json")
	if err != nil {
		t.Fatalf("LoadHarvestables: %v", err)
	}
	return db
}

func testMobsDB(t *testing.T) *gamedata.MobsDatabase {
	t.Helper()
	fsys := fstest.MapFS{
		"mobs.json": mapFile(`[{"u":"CRITTER_HIDE","t":4,"c":"critter","n":"@C","l":"HIDE_CRITTER","lt":4}]`),
	}
	db, err := gamedata.LoadMobs(fsys, "mobs.json")
	if err != nil {
		t.Fatalf("LoadMobs: %v", err)
	}
	return db
}

func fullEnchantGrid() EnchantGrid {
	grid := make(EnchantGrid)
	for _, e := range []string{"e0", "e1", "e2", "e3", "e4"} {
		grid[e] = []bool{true, true, true, true, true, true, true, true}
	}
	return grid
}

func TestHarvestablesState_HandleNewHarvestableObject_Static(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	// typeNumber 3 is within the WOOD band (0-5).
	s.HandleNewHarvestableObject(1, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{10, 20}, 10: int32(5), 11: int32(2)})

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	h := snap[0]
	if h.StringType != "Log" || h.Tier != 4 || h.PosX != 10 || h.PosY != 20 || h.Charges != 2 || h.Size != 5 {
		t.Errorf("harvestable = %+v", h)
	}
	if h.isLiving() {
		t.Error("mobileTypeId=65535 should be treated as static")
	}
}

func TestHarvestablesState_HandleNewHarvestableObject_Living(t *testing.T) {
	mobsDB := testMobsDB(t)
	livingID, ok := mobsDB.GetIDByName("CRITTER_HIDE")
	if !ok {
		t.Fatal("fixture mob not found")
	}
	s := NewHarvestablesState(testHarvestablesDB(t), mobsDB, func(string) EnchantGrid { return nil }, func() bool { return false })

	s.HandleNewHarvestableObject(1, Params{5: int32(99), 6: int32(livingID), 7: int32(4), 8: []float32{0, 0}})

	h := s.Snapshot()[0]
	if h.StringType != "Hide" {
		t.Errorf("StringType = %q, want Hide (from MobsDatabase, ignoring the typeNumber)", h.StringType)
	}
	if !h.isLiving() {
		t.Error("a real mobileTypeId should be treated as living")
	}
}

func TestHarvestablesState_BatchSpawn(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	s.HandleNewSimpleHarvestableObjectList(Params{
		0: []int32{1, 2},
		1: []int32{3, 3},
		2: []int32{4, 4},
		3: []float32{10, 20, 30, 40},
		4: []int32{5, 6},
	})

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
	for _, h := range snap {
		if h.Charges != 0 {
			t.Errorf("batch spawn should always start at Charges=0, got %d", h.Charges)
		}
	}
}

func TestHarvestablesState_ChangeStateUpdatesSizeAndEnchant(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	s.HandleNewSimpleHarvestableObjectList(Params{0: []int32{1}, 1: []int32{3}, 2: []int32{4}, 3: []float32{0, 0}, 4: []int32{10}})

	s.HandleHarvestableChangeState(Params{0: int32(1), 1: int32(7), 2: int32(3)})

	h := s.Snapshot()[0]
	if h.Size != 7 || h.Charges != 3 {
		t.Errorf("after change state: %+v, want Size=7 Charges=3", h)
	}
}

func TestHarvestablesState_ChangeStateWithNoSizeRemovesEntity(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	s.HandleNewSimpleHarvestableObjectList(Params{0: []int32{1}, 1: []int32{3}, 2: []int32{4}, 3: []float32{0, 0}, 4: []int32{10}})

	s.HandleHarvestableChangeState(Params{0: int32(1)}) // no Parameters[1] -> depleted

	if s.Len() != 0 {
		t.Error("a change-state event with no size should remove the entity (depleted)")
	}
}

// @verified: mirrors HarvestablesHandler.js's maybeAlertResource - a batch-spawned resource
// (enchant always 0) only becomes alert-worthy once Event 46 (HandleHarvestableChangeState)
// corrects its real enchantment level.
func TestHarvestablesState_AlertFiresOnceFilterMatches(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return fullEnchantGrid() }, func() bool { return true })

	s.HandleNewSimpleHarvestableObjectList(Params{0: []int32{1}, 1: []int32{3}, 2: []int32{4}, 3: []float32{0, 0}, 4: []int32{10}})
	if alerts := s.PendingAlerts(); len(alerts) == 0 {
		t.Error("a static WOOD resource with a full grid should alert immediately even at enchant 0")
	}
}

func TestHarvestablesState_AlertDoesNotRefireOnceSet(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return fullEnchantGrid() }, func() bool { return true })
	s.HandleNewSimpleHarvestableObjectList(Params{0: []int32{1}, 1: []int32{3}, 2: []int32{4}, 3: []float32{0, 0}, 4: []int32{10}})
	s.PendingAlerts() // drain

	s.HandleHarvestableChangeState(Params{0: int32(1), 1: int32(5), 2: int32(1)})

	if alerts := s.PendingAlerts(); len(alerts) != 0 {
		t.Errorf("PendingAlerts() = %v, want empty (already alerted once)", alerts)
	}
}

func TestHarvestablesState_NoAlertWhenSoundDisabled(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return fullEnchantGrid() }, func() bool { return false })
	s.HandleNewSimpleHarvestableObjectList(Params{0: []int32{1}, 1: []int32{3}, 2: []int32{4}, 3: []float32{0, 0}, 4: []int32{10}})

	if alerts := s.PendingAlerts(); len(alerts) != 0 {
		t.Errorf("PendingAlerts() = %v, want empty (sound disabled)", alerts)
	}
}

// baseUnixNano stands in for a realistic time.Now().UnixNano() - always far more than 250ms
// past the zero Duration, unlike 0 itself, which would collide with lastRangeCheck's
// zero-value and make the very first RemoveNotInRange call look throttled.
const baseUnixNano = int64(1_700_000_000) * 1_000_000_000

func TestHarvestablesState_RemoveNotInRange(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	s.HandleNewHarvestableObject(1, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{0, 0}})
	s.HandleNewHarvestableObject(2, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{1000, 1000}})

	s.RemoveNotInRange(0, 0, baseUnixNano)

	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (the far entity should be dropped)", s.Len())
	}
}

func TestHarvestablesState_RemoveNotInRangeIsThrottled(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	s.HandleNewHarvestableObject(1, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{1000, 1000}})

	s.RemoveNotInRange(0, 0, baseUnixNano)
	if s.Len() != 0 {
		t.Fatal("first call should apply immediately")
	}

	s.HandleNewHarvestableObject(2, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{1000, 1000}})
	s.RemoveNotInRange(0, 0, baseUnixNano+100_000_000) // 100ms later, still within the 250ms throttle window

	if s.Len() != 1 {
		t.Error("a call within the throttle window should be a no-op, leaving the far entity in place")
	}
}

func TestHarvestablesState_UpdateHarvested(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	s.HandleNewHarvestableObject(1, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{0, 0}, 10: int32(10)})

	s.UpdateHarvested(1, 4)
	if s.Snapshot()[0].Size != 6 {
		t.Errorf("Size = %d, want 6", s.Snapshot()[0].Size)
	}

	s.UpdateHarvested(1, 6)
	if s.Len() != 0 {
		t.Error("depleting the last stack should remove the entity")
	}
}

func TestHarvestablesState_EnforceMaxSize(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	for i := range 5 {
		s.HandleNewHarvestableObject(i, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{0, 0}})
	}

	removed := s.EnforceMaxSize(3)
	if removed != 2 || s.Len() != 3 {
		t.Errorf("EnforceMaxSize: removed=%d len=%d, want removed=2 len=3", removed, s.Len())
	}
}

func TestHarvestablesState_ClearRemoveCleanupStale(t *testing.T) {
	s := NewHarvestablesState(testHarvestablesDB(t), testMobsDB(t), func(string) EnchantGrid { return nil }, func() bool { return false })
	s.HandleNewHarvestableObject(1, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{0, 0}})

	s.Remove(1)
	if s.Len() != 0 {
		t.Error("Len() should be 0 after Remove")
	}

	s.HandleNewHarvestableObject(2, Params{5: int32(3), 6: int32(65535), 7: int32(4), 8: []float32{0, 0}})
	s.Clear()
	if s.Len() != 0 {
		t.Error("Len() should be 0 after Clear")
	}
}
