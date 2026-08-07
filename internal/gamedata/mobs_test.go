package gamedata

import (
	"testing"
	"testing/fstest"
)

func TestLoadMobs(t *testing.T) {
	fsys := fstest.MapFS{
		"mobs.json": mapFile(`[
			{"u":"T4_MOB_WOLF","t":4,"c":"solo","n":"@MOB_WOLF"},
			{"u":"KEEPCRITTER_FOREST_HIDE","t":2,"c":"critter","n":"@KEEP","l":"HIDE_CRITTER","lt":3},
			{"u":"MOB_SILVER","t":1,"c":"critter","n":"@SILVER","l":"SILVERCOINS_LOOT_NORMAL"},
			{"u":"MOB_NO_LT","t":5,"c":"critter","n":"@NOLT","l":"WOOD_GUARDIAN"}
		]`),
	}

	db, err := LoadMobs(fsys, "mobs.json")
	if err != nil {
		t.Fatalf("LoadMobs: %v", err)
	}
	if db.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", db.Len())
	}

	// Server TypeID = index + mobOffset(16).
	wolf, ok := db.GetByID(mobOffset)
	if !ok {
		t.Fatal("GetByID(mobOffset) not found")
	}
	if wolf.IsHarvestable {
		t.Error("a combat mob with no Loot.Harvestable should not be IsHarvestable")
	}

	critter, ok := db.GetByID(mobOffset + 1)
	if !ok {
		t.Fatal("GetByID(mobOffset+1) not found")
	}
	if !critter.IsHarvestable || critter.ResourceType != "Hide" || critter.ResourceTier != 3 {
		t.Errorf("critter = %+v, want IsHarvestable=true ResourceType=Hide ResourceTier=3", critter)
	}

	silverMob, ok := db.GetByID(mobOffset + 2)
	if !ok {
		t.Fatal("GetByID(mobOffset+2) not found")
	}
	if silverMob.IsHarvestable {
		t.Error("SILVERCOINS_* loot should never be treated as a harvestable resource")
	}

	// mob.lt absent (0) -> falls back to the mob's own combat tier, mirroring MobsHandler.js's
	// `mob.lt || tier`.
	noLT, ok := db.GetByID(mobOffset + 3)
	if !ok {
		t.Fatal("GetByID(mobOffset+3) not found")
	}
	if noLT.ResourceType != "Log" || noLT.ResourceTier != 5 {
		t.Errorf("noLT = %+v, want ResourceType=Log ResourceTier=5 (falls back to combat tier)", noLT)
	}

	id, ok := db.GetIDByName("T4_MOB_WOLF")
	if !ok || id != mobOffset {
		t.Errorf("GetIDByName(T4_MOB_WOLF) = (%d, %v), want (%d, true)", id, ok, mobOffset)
	}

	if _, ok := db.GetIDByName("NOT_A_MOB"); ok {
		t.Error("GetIDByName should report false for an unknown name")
	}
}

func TestNormalizeResourceType(t *testing.T) {
	tests := []struct {
		lootType string
		want     string
	}{
		{"HIDE_CRITTER", "Hide"},
		{"HIDE_GUARDIAN", "Hide"},
		{"LEATHER_TREASURE", "Hide"},
		{"FIBER_MINIGUARDIAN", "Fiber"},
		{"WOOD_CRITTER", "Log"},
		{"ROCK_GUARDIAN", "Rock"},
		{"STONE_CRITTER", "Rock"},
		{"ORE_TREASURE", "Ore"},
		{"SILVERCOINS_LOOT_ELITE_BOSS", ""},
		{"DEADRAT_LOOT", ""},
		{"SOMETHING_ELSE", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.lootType, func(t *testing.T) {
			if got := normalizeResourceType(tt.lootType); got != tt.want {
				t.Errorf("normalizeResourceType(%q) = %q, want %q", tt.lootType, got, tt.want)
			}
		})
	}
}
