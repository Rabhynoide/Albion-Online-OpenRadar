package gamedata

import (
	"testing"
	"testing/fstest"
)

func TestLoadHarvestables(t *testing.T) {
	fsys := fstest.MapFS{
		"harvestables.json": mapFile(`{
			"WOOD": [{"tier":1,"item":"T1_WOOD","respawn":120,"harvest":1,"tool":false,"maxcharges":2,"startcharges":2,"chargeup":1.08},
			         {"tier":2,"item":"T2_WOOD","respawn":60,"harvest":2,"tool":true,"maxcharges":1,"startcharges":1,"chargeup":2.7}],
			"HIDE": [{"tier":4,"item":"T4_HIDE","respawn":300,"harvest":4,"tool":true,"maxcharges":1,"startcharges":1,"chargeup":1}]
		}`),
	}

	db, err := LoadHarvestables(fsys, "harvestables.json")
	if err != nil {
		t.Fatalf("LoadHarvestables: %v", err)
	}

	if !db.IsValidResource("WOOD", 1, 0) {
		t.Error("WOOD tier 1 enchant 0 should be valid")
	}
	if !db.IsValidResource("WOOD", 2, 4) {
		t.Error("WOOD tier 2 enchant 4 should be valid (all 5 enchants generated per tier)")
	}
	if db.IsValidResource("WOOD", 3, 0) {
		t.Error("WOOD tier 3 was never in the fixture, should be invalid")
	}
	if db.IsValidResource("HIDE", 1, 0) {
		t.Error("HIDE only has tier 4 in the fixture, tier 1 should be invalid")
	}
	if db.IsValidResource("ORE", 1, 0) {
		t.Error("ORE has no data at all in the fixture, should be invalid")
	}

	if got := db.ValidTiers("WOOD"); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("ValidTiers(WOOD) = %v, want [1 2]", got)
	}
	if got := db.ValidTiers("ORE"); got != nil {
		t.Errorf("ValidTiers(ORE) = %v, want nil", got)
	}
}

func TestResourceTypeFromTypeNumber(t *testing.T) {
	tests := []struct {
		typeNumber int
		want       string
	}{
		{0, "WOOD"}, {5, "WOOD"},
		{6, "ROCK"}, {10, "ROCK"},
		{11, "FIBER"}, {15, "FIBER"},
		{16, "HIDE"}, {22, "HIDE"},
		{23, "ORE"}, {27, "ORE"},
		{28, ""}, {-1, ""},
	}
	for _, tt := range tests {
		if got := ResourceTypeFromTypeNumber(tt.typeNumber); got != tt.want {
			t.Errorf("ResourceTypeFromTypeNumber(%d) = %q, want %q", tt.typeNumber, got, tt.want)
		}
	}
}

func TestIsValidResourceByTypeNumber(t *testing.T) {
	fsys := fstest.MapFS{
		"harvestables.json": mapFile(`{"WOOD": [{"tier":1,"item":"T1_WOOD","respawn":120,"harvest":1,"tool":false,"maxcharges":2,"startcharges":2,"chargeup":1.08}]}`),
	}
	db, err := LoadHarvestables(fsys, "harvestables.json")
	if err != nil {
		t.Fatalf("LoadHarvestables: %v", err)
	}

	if !db.IsValidResourceByTypeNumber(0, 1, 0) {
		t.Error("typeNumber 0 (WOOD) tier 1 enchant 0 should be valid")
	}
	if db.IsValidResourceByTypeNumber(6, 1, 0) {
		t.Error("typeNumber 6 (ROCK) has no data loaded, should be invalid")
	}
}
