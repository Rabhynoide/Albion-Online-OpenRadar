package gamedata

import (
	"testing"
	"testing/fstest"
)

func testZonesFixture() fstest.MapFS {
	return fstest.MapFS{
		"zones.json": mapFile(`{
			"1000": {"name":"Lymhurst","type":"PLAYERCITY_SAFEAREA_02","pvpType":"safe","tier":1,"file":"1000_CTY","bounds":{"min":[-305,-305],"max":[295,295]}},
			"0212": {"name":"Bonepool Marsh","type":"OPENPVP","pvpType":"yellow","tier":6,"file":"0212_OP","bounds":{"min":[0,0],"max":[100,100]}},
			"3316": {"name":"Battlebrae Flatland","type":"OPENPVP","pvpType":"black","tier":5,"file":"3316_OP","bounds":{"min":[0,0],"max":[100,100]}},
			"TNL-013": {"name":"Avalon Road","type":"TUNNEL_ROYAL","pvpType":"safe","tier":0,"file":"TNL013","bounds":{"min":[0,0],"max":[100,100]}},
			"TNL-023": {"name":"Avalon Road Red","type":"TUNNEL_ROYAL_RED","pvpType":"red","tier":0,"file":"TNL023","bounds":{"min":[0,0],"max":[100,100]}},
			"TNL-151": {"name":"A Hideout","type":"TUNNEL_HIDEOUT","pvpType":"safe","tier":0,"file":"TNL151","bounds":{"min":[0,0],"max":[100,100]}}
		}`),
	}
}

func TestZonesDatabase_GetZone(t *testing.T) {
	db, err := LoadZones(testZonesFixture(), "zones.json")
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}
	if db.Len() != 6 {
		t.Fatalf("Len() = %d, want 6", db.Len())
	}

	zone, ok := db.GetZone("1000")
	if !ok || zone.Name != "Lymhurst" {
		t.Errorf("GetZone(1000) = %+v, %v", zone, ok)
	}
}

// @verified: mirrors web/scripts/data/_ZonesDatabase.test.js's "ZonesDatabase Avalon Roads
// pvpType" cases - zones.json tags TUNNEL_ROYAL/TUNNEL_ROYAL_RED as safe/red, but the real game
// rule is that all Avalon Roads are full-loot PvP (black); TUNNEL_HIDEOUT is untouched.
func TestZonesDatabase_AvalonRoadsOverride(t *testing.T) {
	db, err := LoadZones(testZonesFixture(), "zones.json")
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}

	if got := db.PvpType("TNL-013"); got != "black" {
		t.Errorf("TUNNEL_ROYAL pvpType = %q, want black (forced despite safe tag)", got)
	}
	if got := db.PvpType("TNL-023"); got != "black" {
		t.Errorf("TUNNEL_ROYAL_RED pvpType = %q, want black (forced despite red tag)", got)
	}
	if got := db.PvpType("TNL-151"); got != "safe" {
		t.Errorf("TUNNEL_HIDEOUT pvpType = %q, want safe (untouched)", got)
	}
	if got := db.PvpType("1000"); got != "safe" {
		t.Errorf("non-tunnel zone pvpType = %q, want safe (untouched)", got)
	}
	if got := db.PvpType("0212"); got != "yellow" {
		t.Errorf("non-tunnel zone pvpType = %q, want yellow (untouched)", got)
	}
}

func TestZonesDatabase_CompoundIDFallsBackToBaseID(t *testing.T) {
	db, err := LoadZones(testZonesFixture(), "zones.json")
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}

	zone, ok := db.GetZone("3316-2")
	if !ok || zone.Name != "Battlebrae Flatland" {
		t.Errorf("GetZone(3316-2) = %+v, %v, want the 3316 zone via base-ID fallback", zone, ok)
	}
}

func TestZonesDatabase_UnknownZoneDefaultsToSafe(t *testing.T) {
	db, err := LoadZones(testZonesFixture(), "zones.json")
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}

	if _, ok := db.GetZone("does-not-exist"); ok {
		t.Error("GetZone should report false for an unknown zone")
	}
	if got := db.PvpType("does-not-exist"); got != "safe" {
		t.Errorf("PvpType(unknown) = %q, want safe (default)", got)
	}
}
