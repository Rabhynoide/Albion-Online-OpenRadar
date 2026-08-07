package overlay

import (
	"testing"

	"github.com/nospy/albion-openradar/internal/radarstate"
)

func onlyOn(keys ...string) func(string) bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return func(k string) bool { return set[k] }
}

func TestShouldRenderDungeon(t *testing.T) {
	tests := []struct {
		name string
		d    radarstate.Dungeon
		on   []string
		want bool
	}{
		{"solo hidden when nothing checked", radarstate.Dungeon{Type: radarstate.DungeonSolo, Enchant: 0}, nil, false},
		{"solo needs both type and enchant", radarstate.Dungeon{Type: radarstate.DungeonSolo, Enchant: 2},
			[]string{"settingDungeonSolo"}, false},
		{"solo shown when both checked", radarstate.Dungeon{Type: radarstate.DungeonSolo, Enchant: 2},
			[]string{"settingDungeonSolo", "settingDungeonE2"}, true},
		{"group uses Duo setting", radarstate.Dungeon{Type: radarstate.DungeonGroup, Enchant: 0},
			[]string{"settingDungeonDuo", "settingDungeonE0"}, true},
		{"corrupted only needs its own toggle", radarstate.Dungeon{Type: radarstate.DungeonCorrupted},
			[]string{"settingDungeonCorrupted"}, true},
		{"hellgate only needs its own toggle", radarstate.Dungeon{Type: radarstate.DungeonHellgate},
			[]string{"settingDungeonHellgate"}, true},
		{"mist solo routes through Mist settings, not Dungeon", radarstate.Dungeon{Type: radarstate.DungeonMistSolo, Enchant: 3},
			[]string{"settingMistSolo", "settingMistE3"}, true},
		{"mist solo not shown via Dungeon settings", radarstate.Dungeon{Type: radarstate.DungeonMistSolo, Enchant: 3},
			[]string{"settingDungeonSolo", "settingDungeonE3"}, false},
		{"mist group routes through Mist Duo", radarstate.Dungeon{Type: radarstate.DungeonMistGroup, Enchant: 1},
			[]string{"settingMistDuo", "settingMistE1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRenderDungeon(tt.d, onlyOn(tt.on...)); got != tt.want {
				t.Errorf("shouldRenderDungeon(%+v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}

func TestShouldRenderChest(t *testing.T) {
	tests := []struct {
		name string
		c    radarstate.Chest
		on   []string
		want bool
	}{
		{"hidden when nothing checked", radarstate.Chest{Name: "STANDARD_CHEST"}, nil, false},
		{"green by standard substring", radarstate.Chest{Name: "STANDARD_CHEST"}, []string{"settingChestGreen"}, true},
		{"green by green substring", radarstate.Chest{Name: "Green_Loot"}, []string{"settingChestGreen"}, true},
		{"blue by uncommon substring", radarstate.Chest{Name: "UNCOMMON_CHEST"}, []string{"settingChestBlue"}, true},
		{"purple by rare substring", radarstate.Chest{Name: "RARE_CHEST"}, []string{"settingChestPurple"}, true},
		{"yellow by legendary substring", radarstate.Chest{Name: "LEGENDARY_CHEST"}, []string{"settingChestYellow"}, true},
		{"setting on but name doesn't match", radarstate.Chest{Name: "LEGENDARY_CHEST"}, []string{"settingChestGreen"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRenderChest(tt.c, onlyOn(tt.on...)); got != tt.want {
				t.Errorf("shouldRenderChest(%+v) = %v, want %v", tt.c, got, tt.want)
			}
		})
	}
}

func TestShouldRenderMist(t *testing.T) {
	tests := []struct {
		name string
		m    radarstate.Mist
		on   []string
		want bool
	}{
		{"hidden when master toggle off", radarstate.Mist{IsSolo: true}, []string{"settingMistE0", "settingMistSolo"}, false},
		{"hidden when enchant not checked", radarstate.Mist{IsSolo: true, Enchant: 2}, []string{"settingWispSpawn", "settingMistSolo"}, false},
		{"solo shown when solo checked", radarstate.Mist{IsSolo: true, Enchant: 1},
			[]string{"settingWispSpawn", "settingMistE1", "settingMistSolo"}, true},
		{"solo hidden when only duo checked", radarstate.Mist{IsSolo: true, Enchant: 1},
			[]string{"settingWispSpawn", "settingMistE1", "settingMistDuo"}, false},
		{"duo shown when duo checked", radarstate.Mist{IsSolo: false, Enchant: 3},
			[]string{"settingWispSpawn", "settingMistE3", "settingMistDuo"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRenderMist(tt.m, onlyOn(tt.on...)); got != tt.want {
				t.Errorf("shouldRenderMist(%+v) = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}

func TestShouldRenderMob(t *testing.T) {
	getNumber := func(_ string, def float64) float64 { return def }
	withThreshold := func(threshold float64) func(string, float64) float64 {
		return func(key string, def float64) float64 {
			if key == "settingTextMinimumHealthEnemies" {
				return threshold
			}
			return def
		}
	}

	tests := []struct {
		name      string
		m         radarstate.Mob
		on        []string
		getNumber func(string, float64) float64
		want      bool
	}{
		{"living resource always defers to MobsState.ShouldRender", radarstate.Mob{Type: radarstate.EnemyLivingSkinnable}, nil, getNumber, true},
		{"identified normal enemy hidden by default", radarstate.Mob{Type: radarstate.EnemyNormal, Identified: true}, nil, getNumber, false},
		{"identified normal enemy shown when checked", radarstate.Mob{Type: radarstate.EnemyNormal, Identified: true},
			[]string{"settingNormalEnemy"}, getNumber, true},
		{"unidentified enemy hidden by default", radarstate.Mob{Type: radarstate.EnemyBoss, Identified: false}, nil, getNumber, false},
		{"unidentified enemy shown via settingShowUnmanagedEnemies regardless of type setting",
			radarstate.Mob{Type: radarstate.EnemyBoss, Identified: false}, []string{"settingShowUnmanagedEnemies"}, getNumber, true},
		{"medium enemy has no dedicated setting, shown once identified", radarstate.Mob{Type: radarstate.EnemyMedium, Identified: true}, nil, getNumber, true},
		{"min health filter hides low-health boss", radarstate.Mob{Type: radarstate.EnemyBoss, Identified: true, MaxHealth: 1000},
			[]string{"settingBossEnemy", "settingShowMinimumHealthEnemies"}, withThreshold(2100), false},
		{"min health filter allows high-health boss", radarstate.Mob{Type: radarstate.EnemyBoss, Identified: true, MaxHealth: 5000},
			[]string{"settingBossEnemy", "settingShowMinimumHealthEnemies"}, withThreshold(2100), true},
		{"drone hidden by default", radarstate.Mob{Type: radarstate.EnemyDrone}, nil, getNumber, false},
		{"drone shown when checked", radarstate.Mob{Type: radarstate.EnemyDrone}, []string{"settingAvaloneDrones"}, getNumber, true},
		{"event enemy hidden by default", radarstate.Mob{Type: radarstate.EnemyEvents}, nil, getNumber, false},
		{"event enemy shown when checked", radarstate.Mob{Type: radarstate.EnemyEvents}, []string{"settingShowEventEnemies"}, getNumber, true},
		{"mist boss always shown, no filter in the web app", radarstate.Mob{Type: radarstate.EnemyMistBoss}, nil, getNumber, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRenderMob(tt.m, onlyOn(tt.on...), tt.getNumber); got != tt.want {
				t.Errorf("shouldRenderMob(%+v) = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}
