package gamedata

import (
	"testing"
	"testing/fstest"
)

func mapFile(content string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(content)}
}

func TestLoadItems(t *testing.T) {
	fsys := fstest.MapFS{
		"items.json": mapFile(`[
			{"n":"T1_OFF_SHIELD","p":100,"t":"equipmentitem","cat":"offhands","sub":"shieldtype"},
			{"n":"T8_OFF_SPIKEDSHIELD_MORGANA@2","p":1375,"t":"equipmentitem","cat":"offhands"},
			{"n":"NOT_TIERED_ITEM","p":50,"t":"equipmentitem"}
		]`),
	}

	db, err := LoadItems(fsys, "items.json")
	if err != nil {
		t.Fatalf("LoadItems: %v", err)
	}
	if db.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", db.Len())
	}

	// Sequential IDs start at 1 (index 0 -> game ID 1).
	first, ok := db.GetByID(1)
	if !ok {
		t.Fatal("GetByID(1) not found")
	}
	want := Item{Name: "T1_OFF_SHIELD", Tier: 1, ItemPower: 100, Enchant: 0, Category: "offhands", Subcategory: "shieldtype"}
	if first != want {
		t.Errorf("GetByID(1) = %+v, want %+v", first, want)
	}

	enchanted, ok := db.GetByID(2)
	if !ok {
		t.Fatal("GetByID(2) not found")
	}
	if enchanted.Enchant != 2 {
		t.Errorf("Enchant = %d, want 2 (parsed from @2 suffix)", enchanted.Enchant)
	}
	if enchanted.Tier != 8 {
		t.Errorf("Tier = %d, want 8", enchanted.Tier)
	}

	untiered, ok := db.GetByID(3)
	if !ok {
		t.Fatal("GetByID(3) not found")
	}
	if untiered.Tier != 0 {
		t.Errorf("Tier = %d, want 0 for a name with no T<N>_ prefix", untiered.Tier)
	}

	if _, ok := db.GetByID(99); ok {
		t.Error("GetByID(99) should not be found")
	}
}

func TestLoadItems_FileNotFound(t *testing.T) {
	fsys := fstest.MapFS{}
	if _, err := LoadItems(fsys, "missing.json"); err == nil {
		t.Error("expected an error for a missing file")
	}
}
