package radarstate

import (
	"testing"
	"testing/fstest"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

func TestCalculateEnchantment(t *testing.T) {
	two := 2
	minusOne := -1
	ten := 10
	if got := calculateEnchantment(nil); got != 0 {
		t.Errorf("calculateEnchantment(nil) = %d, want 0", got)
	}
	if got := calculateEnchantment(&two); got != 2 {
		t.Errorf("calculateEnchantment(2) = %d, want 2", got)
	}
	if got := calculateEnchantment(&minusOne); got != 0 {
		t.Errorf("calculateEnchantment(-1) = %d, want 0 (clamped)", got)
	}
	if got := calculateEnchantment(&ten); got != 4 {
		t.Errorf("calculateEnchantment(10) = %d, want 4 (clamped)", got)
	}
}

func TestMob_CurrentHPAndHealthPercent(t *testing.T) {
	m := Mob{Health: 128, MaxHealth: 1000}
	if got := m.HealthPercent(); got != 50 { // round(128/255*100) = round(50.196) = 50
		t.Errorf("HealthPercent() = %d, want 50", got)
	}
	if got := m.CurrentHP(); got != 502 { // round(128/255*1000) = round(501.96) = 502
		t.Errorf("CurrentHP() = %d, want 502", got)
	}
}

func TestClassifyEnemyCategory(t *testing.T) {
	tests := []struct {
		name, category, uniqueName string
		want                       EnemyType
	}{
		{"boss category", "boss", "T5_MOB_BOSS", EnemyBoss},
		{"miniboss category", "miniboss", "SOMETHING", EnemyMiniBoss},
		{"champion category", "champion", "SOMETHING", EnemyEnchanted},
		{"rd_elite category", "rd_elite", "SOMETHING", EnemyMiniBoss},
		{"rd_solo category", "rd_solo", "SOMETHING", EnemyEnchanted},
		{"unknown category defaults to Normal", "some_unknown_cat", "SOMETHING", EnemyNormal},
		{"VETERAN name heuristic overrides static category", "static", "T6_MOB_CROSSBOWMAN_VETERAN", EnemyMiniBoss},
		{"VETERAN_CHAMPION excluded from VETERAN heuristic", "champion", "T6_MOB_VETERAN_CHAMPION", EnemyEnchanted},
		{"ELITE name heuristic", "static", "T6_MOB_SOMETHING_ELITE", EnemyMiniBoss},
		{"BOSS name heuristic", "static", "T6_MOB_SOMETHING_BOSS", EnemyBoss},
		{"MINIBOSS in name excluded from BOSS heuristic", "static", "T6_MOB_MINIBOSS_THING", EnemyNormal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyEnemyCategory(tt.category, tt.uniqueName); got != tt.want {
				t.Errorf("classifyEnemyCategory(%q, %q) = %v, want %v", tt.category, tt.uniqueName, got, tt.want)
			}
		})
	}
}

func TestSettingNameForEnemyType(t *testing.T) {
	if got := SettingNameForEnemyType(EnemyNormal); got != "settingNormalEnemy" {
		t.Errorf("SettingNameForEnemyType(Normal) = %q", got)
	}
	if got := SettingNameForEnemyType(EnemyMistBoss); got != "" {
		t.Errorf("SettingNameForEnemyType(MistBoss) = %q, want \"\" (no dedicated setting)", got)
	}
}

func hostileMobsDB(t *testing.T) *gamedata.MobsDatabase {
	t.Helper()
	fsys := fstest.MapFS{
		"mobs.json": mapFile(`[
			{"u":"T5_MOB_BOSS_KEEPER","t":5,"c":"boss","n":"@BOSS"},
			{"u":"T4_MOB_STANDARD","t":4,"c":"standard","n":"@STD"},
			{"u":"CRITTER_HIDE","t":3,"c":"critter","n":"@C","l":"HIDE_CRITTER","lt":3}
		]`),
	}
	db, err := gamedata.LoadMobs(fsys, "mobs.json")
	if err != nil {
		t.Fatalf("LoadMobs: %v", err)
	}
	return db
}

func TestMobsState_HandleNewMob_HostileBoss(t *testing.T) {
	db := hostileMobsDB(t)
	bossID, _ := db.GetIDByName("T5_MOB_BOSS_KEEPER")
	s := NewMobsState(db)

	s.HandleNewMob(Params{0: int32(1), 1: int32(bossID), 2: int32(255), 7: []float32{10, 20}, 13: int32(50000)})

	snap := s.MobSnapshot()
	if len(snap) != 1 {
		t.Fatalf("MobSnapshot len = %d, want 1", len(snap))
	}
	m := snap[0]
	if m.Type != EnemyBoss || m.Tier != 5 || m.Name != "T5_MOB_BOSS_KEEPER" || !m.Identified {
		t.Errorf("mob = %+v", m)
	}
}

func TestMobsState_HandleNewMob_LivingResource(t *testing.T) {
	db := hostileMobsDB(t)
	critterID, _ := db.GetIDByName("CRITTER_HIDE")
	s := NewMobsState(db)

	s.HandleNewMob(Params{0: int32(1), 1: int32(critterID), 2: int32(255), 7: []float32{0, 0}, 33: int32(2)})

	m := s.MobSnapshot()[0]
	if m.Type != EnemyLivingSkinnable || m.Name != "Hide" || m.EnchantmentLevel != 2 {
		t.Errorf("mob = %+v, want Type=LivingSkinnable Name=Hide EnchantmentLevel=2", m)
	}
}

// @verified: mirrors MobsDrawing.js's EnemyType.LivingSkinnable/LivingHarvestable branch
// (LivingResourceFilter.js's shouldRenderLivingResource) - a living-resource mob (a skinnable
// animal like "Lapin des neiges") is a SEPARATE draw-time gate from the Enemies-page filter
// hostile mobs use, and was missing entirely from the overlay before this test was added
// (reported: living-resource mobs always rendered regardless of the Resources page grid).
func TestMobsState_ShouldRender_EmptyGridHidesLivingResource(t *testing.T) {
	db := hostileMobsDB(t)
	critterID, _ := db.GetIDByName("CRITTER_HIDE")
	s := NewMobsState(db)
	s.SetEnchantGrid(func(string) EnchantGrid { return nil })
	s.HandleNewMob(Params{0: int32(1), 1: int32(critterID), 2: int32(255), 7: []float32{0, 0}, 33: int32(2)})
	m := s.MobSnapshot()[0]

	if s.ShouldRender(m) {
		t.Error("ShouldRender should be false when nothing is checked on the Resources page (the default)")
	}
}

func TestMobsState_ShouldRender_FullGridShowsLivingResource(t *testing.T) {
	db := hostileMobsDB(t)
	critterID, _ := db.GetIDByName("CRITTER_HIDE")
	s := NewMobsState(db)
	s.SetEnchantGrid(func(string) EnchantGrid { return fullEnchantGrid() })
	s.HandleNewMob(Params{0: int32(1), 1: int32(critterID), 2: int32(255), 7: []float32{0, 0}, 33: int32(2)})
	m := s.MobSnapshot()[0]

	if !s.ShouldRender(m) {
		t.Error("ShouldRender should be true when the grid has every cell checked")
	}
}

func TestMobsState_ShouldRender_HostileMobsAlwaysTrue(t *testing.T) {
	db := hostileMobsDB(t)
	bossID, _ := db.GetIDByName("T5_MOB_BOSS_KEEPER")
	s := NewMobsState(db)
	s.SetEnchantGrid(func(string) EnchantGrid { return nil }) // would hide a living resource
	s.HandleNewMob(Params{0: int32(1), 1: int32(bossID), 2: int32(255), 7: []float32{10, 20}, 13: int32(50000)})
	m := s.MobSnapshot()[0]

	if !s.ShouldRender(m) {
		t.Error("ShouldRender should always be true for non-living-resource enemy types (their own Enemies-page gate applies separately)")
	}
}

func TestMobsState_ShouldRender_NilGridDefaultsTrue(t *testing.T) {
	db := hostileMobsDB(t)
	critterID, _ := db.GetIDByName("CRITTER_HIDE")
	s := NewMobsState(db) // SetEnchantGrid never called
	s.HandleNewMob(Params{0: int32(1), 1: int32(critterID), 2: int32(255), 7: []float32{0, 0}, 33: int32(2)})
	m := s.MobSnapshot()[0]

	if !s.ShouldRender(m) {
		t.Error("ShouldRender should default to true when no grid function is wired at all")
	}
}

func TestMobsState_HandleNewMob_UnknownMobStaysDefault(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(99999), 2: int32(255), 7: []float32{0, 0}})

	m := s.MobSnapshot()[0]
	if m.Type != EnemyNormal || m.Identified {
		t.Errorf("unknown mob = %+v, want Type=Normal Identified=false", m)
	}
}

// @verified: mirrors AddEnemy's fort/dungeon-NPC low-HP workaround - a spawn default of
// health_normalized=5 (< 10) means "not real data yet", treated as full health (255).
func TestMobsState_LowSpawnHealthTreatedAsFull(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(5), 7: []float32{0, 0}})

	if got := s.MobSnapshot()[0].Health; got != 255 {
		t.Errorf("Health = %d, want 255 (low-spawn-HP workaround)", got)
	}
}

func TestMobsState_HandleNewMob_MistRoutesToMistList(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 32: "MISTS_SOLO_YELLOW"})

	if s.MobLen() != 0 {
		t.Error("a NewMobEvent carrying a name should never be added to the mob list")
	}
	if s.MistLen() != 1 {
		t.Fatal("it should be added to the mist list instead")
	}
	if !s.MistSnapshot()[0].IsSolo {
		t.Error("a name containing 'solo' should set IsSolo=true")
	}
}

func TestMobsState_DuplicateIDIgnored(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{10, 10}})
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{99, 99}})

	if got := s.MobSnapshot()[0].PosX; got != 10 {
		t.Errorf("PosX = %v, want 10 (second event with a duplicate id should be a no-op)", got)
	}
}

func TestMobsState_UpdatePositionAndEnchant(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}})

	s.UpdatePosition(1, 50, 60)
	m := s.MobSnapshot()[0]
	if m.PosX != 50 || m.PosY != 60 {
		t.Errorf("after UpdatePosition: %+v", m)
	}

	s.HandleEnchantChanged(Params{0: int32(1), 1: int32(3)})
	if got := s.MobSnapshot()[0].EnchantmentLevel; got != 3 {
		t.Errorf("EnchantmentLevel = %d, want 3", got)
	}
}

func TestMobsState_HandleHealthUpdate(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 13: int32(1000)})

	s.HandleHealthUpdate(Params{0: int32(1), 3: int32(500)})
	if got := s.MobSnapshot()[0].Health; got != 128 { // round(500/1000*255) = 127.5 -> 128
		t.Errorf("Health = %d, want 128", got)
	}
}

func TestMobsState_HandleHealthUpdateDeathRemoves(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 13: int32(1000)})

	s.HandleHealthUpdate(Params{0: int32(1)}) // no Parameters[3] -> dead

	if s.MobLen() != 0 {
		t.Error("a health update with no currentHP should remove the mob (death)")
	}
}

func TestMobsState_HandleHealthUpdateIgnoresUnknownID(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleHealthUpdate(Params{0: int32(999), 3: int32(50)}) // no such mob (e.g. a player)

	if s.MobLen() != 0 {
		t.Error("a health update for an unknown id should be a silent no-op")
	}
}

func TestMobsState_HandleHealthUpdateBulk(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 13: int32(1000)})

	s.HandleHealthUpdateBulk(Params{0: int32(1), 1: []int32{0, 1}, 2: []int32{-10, -20}, 3: []int32{900, 700}})

	// Every entry in the bulk uses Parameters[0] as the id (JS quirk kept verbatim) - the last
	// write wins.
	if got := s.MobSnapshot()[0].Health; got == 0 {
		t.Error("bulk health update should have applied at least the last entry")
	}
}

func TestMobsState_HandleHealthRegen(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}})

	s.HandleHealthRegen(Params{0: int32(1), 2: int32(200)})

	if got := s.MobSnapshot()[0].Health; got != 200 {
		t.Errorf("Health = %d, want 200 (stored directly, no MaxHealth conversion)", got)
	}
}

func TestMobsState_MistUpdatesAndRemoval(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 32: "Mist Portal"})

	s.UpdateMistPosition(1, 5, 6)
	s.UpdateMistEnchantmentLevel(1, 2)
	mist := s.MistSnapshot()[0]
	if mist.PosX != 5 || mist.PosY != 6 || mist.Enchant != 2 {
		t.Errorf("mist = %+v", mist)
	}

	s.RemoveMist(1)
	if s.MistLen() != 0 {
		t.Error("RemoveMist should remove the mist")
	}
}

func TestMobsState_ClearRemovesBothLists(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	s.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}})
	s.HandleNewMob(Params{0: int32(2), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 32: "Mist"})

	s.Clear()

	if s.MobLen() != 0 || s.MistLen() != 0 {
		t.Errorf("Clear should empty both lists: mobs=%d mists=%d", s.MobLen(), s.MistLen())
	}
}

func TestMobsState_EnforceMaxSize_IndependentCaps(t *testing.T) {
	s := NewMobsState(hostileMobsDB(t))
	for i := range 5 {
		s.HandleNewMob(Params{0: int32(i), 1: int32(0), 2: int32(255), 7: []float32{0, 0}})
	}
	for i := range 5 {
		s.HandleNewMob(Params{0: int32(100 + i), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 32: "Mist"})
	}

	removed := s.EnforceMaxSize(2, 3)

	if removed != 3+2 || s.MobLen() != 2 || s.MistLen() != 3 {
		t.Errorf("removed=%d mobLen=%d mistLen=%d, want removed=5 mobLen=2 mistLen=3", removed, s.MobLen(), s.MistLen())
	}
}
