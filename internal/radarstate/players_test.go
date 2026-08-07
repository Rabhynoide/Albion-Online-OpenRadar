package radarstate

import (
	"testing"
	"testing/fstest"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

func TestIsPlayerThreat(t *testing.T) {
	tests := []struct {
		name    string
		faction int
		pvpType string
		want    bool
	}{
		{"safe zone never alerts", FactionHostile, "safe", false},
		{"black zone always alerts", FactionPassive, "black", true},
		{"red zone alerts only hostile", FactionHostile, "red", true},
		{"red zone does not alert passive", FactionPassive, "red", false},
		{"yellow zone alerts only hostile", FactionHostile, "yellow", true},
		{"unknown pvpType defaults like red/yellow", FactionHostile, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPlayerThreat(tt.faction, tt.pvpType); got != tt.want {
				t.Errorf("IsPlayerThreat(%d, %q) = %v, want %v", tt.faction, tt.pvpType, got, tt.want)
			}
		})
	}
}

func mapFile(content string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(content)}
}

func testZonesDB(t *testing.T) *gamedata.ZonesDatabase {
	t.Helper()
	fsys := fstest.MapFS{
		"zones.json": mapFile(`{
			"safe-zone": {"name":"Safe","type":"X","pvpType":"safe","tier":1,"file":"f","bounds":{"min":[0,0],"max":[1,1]}},
			"black-zone": {"name":"Black","type":"X","pvpType":"black","tier":1,"file":"f","bounds":{"min":[0,0],"max":[1,1]}}
		}`),
	}
	db, err := gamedata.LoadZones(fsys, "zones.json")
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}
	return db
}

func TestPlayersState_HandleNewCharacter(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.HandleNewCharacter(1, Params{
		1: "Alice", 8: "GuildX", 53: int32(FactionPassive), 51: "AllianceY",
		40: []int32{100, 200}, 43: []int32{5},
	})

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	p := snap[0]
	if p.Nickname != "Alice" || p.GuildName != "GuildX" || p.AllianceName != "AllianceY" || p.Faction != FactionPassive {
		t.Errorf("player = %+v", p)
	}
}

func TestPlayersState_HardCeilingOf100(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	for i := range 105 {
		s.HandleNewCharacter(i, Params{1: "P", 53: int32(FactionPassive)})
	}
	if s.Len() != 100 {
		t.Errorf("Len() = %d, want 100 (hard ceiling)", s.Len())
	}
}

func TestPlayersState_NewHostilePlayerQueuesAlert(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("black-zone")

	s.HandleNewCharacter(1, Params{1: "Enemy", 53: int32(FactionHostile)})

	alerts := s.PendingAlerts()
	if len(alerts) != 1 || alerts[0] != "Enemy" {
		t.Errorf("PendingAlerts() = %v, want [Enemy]", alerts)
	}
	// Draining should clear the queue.
	if got := s.PendingAlerts(); len(got) != 0 {
		t.Errorf("second PendingAlerts() = %v, want empty", got)
	}
}

func TestPlayersState_ExcludedPlayerNeverAlerts(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(name string) bool { return name == "Friend" })
	s.SetCurrentZone("black-zone")

	s.HandleNewCharacter(1, Params{1: "Friend", 53: int32(FactionHostile)})

	if alerts := s.PendingAlerts(); len(alerts) != 0 {
		t.Errorf("PendingAlerts() = %v, want empty (ignored player)", alerts)
	}
}

func TestPlayersState_PartyMemberNeverAlerts(t *testing.T) {
	roster := NewPartyRoster()
	roster.HandlePartyJoined(Params{9: []string{"Buddy"}})
	s := NewPlayersState(roster, testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("black-zone")

	s.HandleNewCharacter(1, Params{1: "Buddy", 53: int32(FactionHostile)})

	if alerts := s.PendingAlerts(); len(alerts) != 0 {
		t.Errorf("PendingAlerts() = %v, want empty (party member)", alerts)
	}
}

func TestPlayersState_UnknownZoneFallsBackToYellowForAlerting(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("does-not-exist-in-zones-json")

	// yellow alerts hostile (255) but not passive.
	s.HandleNewCharacter(1, Params{1: "Enemy", 53: int32(FactionHostile)})
	if alerts := s.PendingAlerts(); len(alerts) != 1 {
		t.Errorf("hostile in unknown zone should alert (fallback yellow), got %v", alerts)
	}

	s.HandleNewCharacter(2, Params{1: "Passive", 53: int32(FactionPassive)})
	if alerts := s.PendingAlerts(); len(alerts) != 0 {
		t.Errorf("passive in unknown zone should not alert, got %v", alerts)
	}
}

func TestPlayersState_HandleMounted(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.HandleNewCharacter(1, Params{1: "P"})

	s.HandleMounted(1, Params{11: true})
	if !s.Snapshot()[0].Mounted {
		t.Error("expected Mounted=true from Parameters[11]=true")
	}

	s.HandleMounted(1, Params{})
	if s.Snapshot()[0].Mounted {
		t.Error("expected Mounted=false when neither condition matches")
	}

	// The "-1 in slot 10" fallback, kept verbatim from the JS.
	s.HandleMounted(1, Params{10: "-1"})
	if !s.Snapshot()[0].Mounted {
		t.Error("expected Mounted=true from the Parameters[10]==-1 fallback")
	}
}

func TestPlayersState_HandleEquipmentChanged(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.HandleNewCharacter(1, Params{1: "P"})

	s.HandleEquipmentChanged(1, Params{2: []int32{1, 2, 3}})

	got := s.Snapshot()[0].Items
	if len(got) != 3 {
		t.Errorf("Items = %v, want [1 2 3]", got)
	}
}

func TestPlayersState_HealthUpdates(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.HandleNewCharacter(1, Params{1: "P"})

	s.HandleHealthUpdate(Params{0: int32(1), 2: float32(50), 3: float32(100)})
	p := s.Snapshot()[0]
	if p.CurrentHealth != 50 || p.InitialHealth != 100 {
		t.Errorf("after HandleHealthUpdate: %+v", p)
	}

	s.HandleLooseHealthUpdate(Params{0: int32(1), 3: float32(30)})
	p = s.Snapshot()[0]
	if p.CurrentHealth != 30 || p.InitialHealth != 100 {
		t.Errorf("after HandleLooseHealthUpdate: %+v (InitialHealth must be untouched)", p)
	}
}

func TestPlayersState_FactionChangeToHostileAlerts(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("black-zone")
	s.HandleNewCharacter(1, Params{1: "Neutral", 53: int32(FactionPassive)})
	s.PendingAlerts() // drain the (none, passive spawn in black zone still alerts since black=always-threat)

	s.HandleFactionChanged(1, FactionHostile)

	if got := s.Snapshot()[0].Faction; got != FactionHostile {
		t.Errorf("Faction = %d, want %d", got, FactionHostile)
	}
}

func TestPlayersState_FactionChangeAlreadyHostileDoesNotReAlert(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("black-zone")
	s.HandleNewCharacter(1, Params{1: "AlreadyHostile", 53: int32(FactionHostile)})
	s.PendingAlerts() // drain the spawn alert

	s.HandleFactionChanged(1, FactionHostile) // faction "changes" to the same hostile value

	if alerts := s.PendingAlerts(); len(alerts) != 0 {
		t.Errorf("PendingAlerts() = %v, want empty (wasHostile already true, no transition)", alerts)
	}
}

func TestPlayersState_RemoveClearLen(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.HandleNewCharacter(1, Params{1: "A"})
	s.HandleNewCharacter(2, Params{1: "B"})

	s.Remove(1)
	if s.Len() != 1 {
		t.Errorf("Len() = %d after Remove, want 1", s.Len())
	}
	s.Clear()
	if s.Len() != 0 {
		t.Errorf("Len() = %d after Clear, want 0", s.Len())
	}
}

func TestPlayersState_EnforceMaxSize(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	for i := range 5 {
		s.HandleNewCharacter(i, Params{1: "P"})
	}

	removed := s.EnforceMaxSize(3)
	if removed != 2 {
		t.Errorf("EnforceMaxSize removed %d, want 2", removed)
	}
	if s.Len() != 3 {
		t.Errorf("Len() = %d after EnforceMaxSize, want 3", s.Len())
	}
}

func TestPlayersState_FilteredPlayers_BlackZoneUsesShowDangerousOnly(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("black-zone")
	s.HandleNewCharacter(1, Params{1: "Anyone", 53: int32(FactionPassive)})

	if got := s.FilteredPlayers(true, true, false); len(got) != 0 {
		t.Errorf("black zone with showDangerous=false should hide everyone, got %v", got)
	}
	if got := s.FilteredPlayers(false, false, true); len(got) != 1 {
		t.Errorf("black zone with showDangerous=true should show everyone, got %v", got)
	}
}

func TestPlayersState_FilteredPlayers_ExcludesIgnoredAndParty(t *testing.T) {
	roster := NewPartyRoster()
	roster.HandlePartyJoined(Params{9: []string{"Buddy"}})
	s := NewPlayersState(roster, testZonesDB(t), func(name string) bool { return name == "Ignored" })
	s.SetCurrentZone("safe-zone")
	s.HandleNewCharacter(1, Params{1: "Buddy", 53: int32(FactionPassive)})
	s.HandleNewCharacter(2, Params{1: "Ignored", 53: int32(FactionPassive)})
	s.HandleNewCharacter(3, Params{1: "Normal", 53: int32(FactionPassive)})

	got := s.FilteredPlayers(true, true, true)
	if len(got) != 1 || got[0].Nickname != "Normal" {
		t.Errorf("FilteredPlayers = %v, want only Normal", got)
	}
}

func TestPlayersState_PlayersByType(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("safe-zone")
	s.HandleNewCharacter(1, Params{1: "P", 53: int32(FactionPassive)})
	s.HandleNewCharacter(2, Params{1: "F", 53: int32(3)})
	s.HandleNewCharacter(3, Params{1: "H", 53: int32(FactionHostile)})

	hostile, faction, passive := s.PlayersByType(true, true, true)
	if len(hostile) != 1 || len(faction) != 1 || len(passive) != 1 {
		t.Errorf("hostile=%d faction=%d passive=%d, want 1/1/1", len(hostile), len(faction), len(passive))
	}
}

func TestPlayersState_ThreatPlayers(t *testing.T) {
	s := NewPlayersState(NewPartyRoster(), testZonesDB(t), func(string) bool { return false })
	s.SetCurrentZone("black-zone")
	s.HandleNewCharacter(1, Params{1: "Anyone", 53: int32(FactionPassive)})

	threats := s.ThreatPlayers()
	if len(threats) != 1 {
		t.Errorf("ThreatPlayers() len = %d, want 1 (black zone treats everyone as a threat)", len(threats))
	}
}

func TestAverageItemPower(t *testing.T) {
	fsys := fstest.MapFS{
		"items.json": mapFile(`[
			{"n":"WEAPON","p":800,"t":"x","cat":"weapons"},
			{"n":"CAPE","p":600,"t":"x","cat":"capes"},
			{"n":"FOOD","p":400,"t":"x","cat":"food"}
		]`),
	}
	items, err := gamedata.LoadItems(fsys, "items.json")
	if err != nil {
		t.Fatalf("LoadItems: %v", err)
	}

	// index 0 = weapon (combat slot), index 5 = cape (excluded), index 8 = food (combat slot).
	equipments := make([]int, 9)
	equipments[0] = 1
	equipments[5] = 2
	equipments[8] = 3

	got := AverageItemPower(equipments, items)
	if got == nil || *got != 600 { // (800+400)/2 = 600
		t.Errorf("AverageItemPower = %v, want 600 (cape excluded)", got)
	}
}

func TestAverageItemPower_NoValidItemsReturnsNil(t *testing.T) {
	if got := AverageItemPower(nil, nil); got != nil {
		t.Errorf("AverageItemPower(nil) = %v, want nil", got)
	}
	if got := AverageItemPower([]int{0, 0, 0}, nil); got != nil {
		t.Errorf("AverageItemPower(all zero) = %v, want nil", got)
	}
}
