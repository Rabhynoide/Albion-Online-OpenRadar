package overlay

import (
	"github.com/nospy/albion-openradar/internal/gamedata"
	"github.com/nospy/albion-openradar/internal/radarstate"
)

// State bundles every radarstate/gamedata reference the overlay window reads from - a single
// struct so cmd/radar/main.go has one thing to construct and pass to NewWindow, rather than a
// dozen constructor parameters.
type State struct {
	Harvestables   *radarstate.HarvestablesState
	Mobs           *radarstate.MobsState
	Players        *radarstate.PlayersState
	Chests         *radarstate.ChestsState
	Dungeons       *radarstate.DungeonsState
	Fishing        *radarstate.FishingState
	LocalTreasures *radarstate.LocalTreasuresState
	MistsDungeon   *radarstate.MistsDungeonState
	WispCage       *radarstate.WispCageState
	Session        *radarstate.Session
	ZoneGraph      *radarstate.ZoneGraph

	Items  *gamedata.ItemsDatabase
	MobsDB *gamedata.MobsDatabase
	Zones  *gamedata.ZonesDatabase
}
