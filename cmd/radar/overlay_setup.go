package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nospy/albion-openradar/internal/gamedata"
	"github.com/nospy/albion-openradar/internal/overlay"
	"github.com/nospy/albion-openradar/internal/radarstate"
	"github.com/nospy/albion-openradar/internal/roads"
	"github.com/nospy/albion-openradar/internal/syncsettings"
)

// buildOverlayState loads game data and constructs every radarstate handler the native
// -overlay window needs, wired into a Router ready to receive already-decoded Photon
// events/requests/responses (see onPhotonEvent/onPhotonRequest/onPhotonResponse). Mirrors the
// browser's DatabaseLoader.js + Utils.js's initRadar() wiring, just in Go and without a
// WebSocket hop.
func buildOverlayState(appDir string) (*overlay.State, *radarstate.Router, error) {
	dataFS := os.DirFS(appDir + "/web/ao-bin-dumps")

	items, err := gamedata.LoadItems(dataFS, "items.min.json.gz")
	if err != nil {
		return nil, nil, fmt.Errorf("load items: %w", err)
	}
	mobsDB, err := gamedata.LoadMobs(dataFS, "mobs.min.json.gz")
	if err != nil {
		return nil, nil, fmt.Errorf("load mobs: %w", err)
	}
	harvestablesDB, err := gamedata.LoadHarvestables(dataFS, "harvestables.min.json.gz")
	if err != nil {
		return nil, nil, fmt.Errorf("load harvestables: %w", err)
	}
	zonesDB, err := gamedata.LoadZones(dataFS, "zones.json.gz")
	if err != nil {
		return nil, nil, fmt.Errorf("load zones: %w", err)
	}
	staticEdges, err := gamedata.LoadZoneGraph(dataFS, "zone-graph.json.gz")
	if err != nil {
		return nil, nil, fmt.Errorf("load zone graph: %w", err)
	}

	roadsStore, err := roads.ReadStore(appDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read roads store: %w", err)
	}

	zoneGraph := radarstate.NewZoneGraph(appDir)
	zoneGraph.LoadFromEdges(staticEdges, roadsStore.Edges)

	partyRoster := radarstate.NewPartyRoster()
	playersState := radarstate.NewPlayersState(partyRoster, zonesDB, func(nickname string) bool {
		return isIgnoredPlayer(appDir, nickname)
	})
	harvestablesState := radarstate.NewHarvestablesState(
		harvestablesDB, mobsDB,
		func(key string) radarstate.EnchantGrid { return readEnchantGrid(appDir, key) },
		func() bool { return readSettingBool(appDir, "settingResourceSound") },
	)

	router := &radarstate.Router{
		Players:        playersState,
		Mobs:           radarstate.NewMobsState(mobsDB),
		Harvestables:   harvestablesState,
		Chests:         radarstate.NewChestsState(),
		Dungeons:       radarstate.NewDungeonsState(),
		Fishing:        radarstate.NewFishingState(),
		LocalTreasures: radarstate.NewLocalTreasuresState(),
		MistsDungeon:   radarstate.NewMistsDungeonState(),
		WispCage:       radarstate.NewWispCageState(),
		PartyRoster:    partyRoster,
		Session:        radarstate.NewSession(),
		ZoneGraph:      zoneGraph,
	}

	state := &overlay.State{
		Harvestables:   router.Harvestables,
		Mobs:           router.Mobs,
		Players:        router.Players,
		Chests:         router.Chests,
		Dungeons:       router.Dungeons,
		Fishing:        router.Fishing,
		LocalTreasures: router.LocalTreasures,
		MistsDungeon:   router.MistsDungeon,
		WispCage:       router.WispCage,
		Session:        router.Session,
		ZoneGraph:      router.ZoneGraph,
		Items:          items,
		MobsDB:         mobsDB,
		Zones:          zonesDB,
	}
	return state, router, nil
}

// readSettingBool/readEnchantGrid/isIgnoredPlayer read live from settings-sync.json on every
// call (internal/syncsettings, the same file web/scripts/utils/SettingsSync.js write-throughs
// to) rather than caching, so a change made on the web Settings/Resources page while the
// overlay is running takes effect on the very next event - matching SettingsSync.js's own
// always-fresh localStorage reads. A missing/unreadable file degrades to defaults rather than
// erroring, consistent with syncsettings.ReadAll's own missing-file handling.
func readSettingBool(appDir, key string) bool {
	settings, err := syncsettings.ReadAll(appDir)
	if err != nil {
		return false
	}
	return settings[key] == "true"
}

func readEnchantGrid(appDir, key string) radarstate.EnchantGrid {
	settings, err := syncsettings.ReadAll(appDir)
	if err != nil {
		return nil
	}
	raw, ok := settings[key]
	if !ok {
		return nil
	}
	var grid radarstate.EnchantGrid
	if err := json.Unmarshal([]byte(raw), &grid); err != nil {
		return nil
	}
	return grid
}

func isIgnoredPlayer(appDir, nickname string) bool {
	settings, err := syncsettings.ReadAll(appDir)
	if err != nil {
		return false
	}
	raw, ok := settings["ignoreList"]
	if !ok {
		return false
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return false
	}
	for _, name := range list {
		if name == nickname {
			return true
		}
	}
	return false
}
