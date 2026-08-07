package radarstate

import (
	"testing"
	"testing/fstest"

	"github.com/nospy/albion-openradar/internal/gamedata"
	"github.com/nospy/albion-openradar/internal/photon"
	"github.com/nospy/albion-openradar/internal/photon/eventcodes"
	"github.com/nospy/albion-openradar/internal/photon/operationcodes"
)

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	fsys := fstest.MapFS{
		"mobs.json": mapFile(`[{"u":"T5_MOB_BOSS","t":5,"c":"boss","n":"@B"}]`),
	}
	mobsDB, err := gamedata.LoadMobs(fsys, "mobs.json")
	if err != nil {
		t.Fatalf("LoadMobs: %v", err)
	}
	dir := t.TempDir()
	zoneGraph := NewZoneGraph(dir)
	zoneGraph.LoadFromEdges(nil, nil)

	return &Router{
		Players:        NewPlayersState(NewPartyRoster(), nil, func(string) bool { return false }),
		Mobs:           NewMobsState(mobsDB),
		Harvestables:   NewHarvestablesState(nil, mobsDB, func(string) EnchantGrid { return nil }, func() bool { return false }),
		Chests:         NewChestsState(),
		Dungeons:       NewDungeonsState(),
		Fishing:        NewFishingState(),
		LocalTreasures: NewLocalTreasuresState(),
		MistsDungeon:   NewMistsDungeonState(),
		WispCage:       NewWispCageState(),
		PartyRoster:    NewPartyRoster(),
		Session:        NewSession(),
		ZoneGraph:      zoneGraph,
	}
}

// eventWithCode builds an EventData the way internal/photon actually produces one - the real
// dispatch key lives in Parameters[252], mirroring PostProcessEvent's own fallback semantics
// (this is the exact bug the router.go rewrite caught: event.Code alone can't hold codes >255).
func eventWithCode(code int, extra Params) *photon.EventData {
	params := map[byte]interface{}{252: int32(code)}
	for k, v := range extra {
		params[k] = v
	}
	return &photon.EventData{Parameters: params}
}

func TestRouter_HandleEvent_UsesParameters252NotRawCode(t *testing.T) {
	r := newTestRouter(t)
	// NewLootChest(393) can never fit in EventData.Code (a byte) - if the router mistakenly
	// dispatched on event.Code instead of Parameters[252], this would never reach ChestsState.
	event := eventWithCode(eventcodes.NewLootChest, Params{0: int32(1), 1: []float32{10, 20}, 3: "Chest"})

	r.HandleEvent(event)

	if r.Chests.Len() != 1 {
		t.Fatal("NewLootChest event should have reached ChestsState")
	}
}

func TestRouter_HandleEvent_Leave_RemovesFromEveryList(t *testing.T) {
	r := newTestRouter(t)
	r.Chests.HandleNewLootChest(Params{0: int32(42), 1: []float32{0, 0}, 3: "C"})
	r.Fishing.HandleNewFishingZoneObject(Params{0: int32(42), 1: []float32{0, 0}, 4: "FISH"})
	r.WispCage.HandleNewCagedObject(Params{0: int32(42), 2: []float32{0, 0}, 4: "Cage"})

	r.HandleEvent(eventWithCode(eventcodes.Leave, Params{0: int32(42)}))

	if r.Chests.Len() != 0 {
		t.Error("Chests entries should be removed after Leave")
	}
	if got := len(r.Fishing.Snapshot()); got != 0 {
		t.Errorf("Fishing entries = %d, want 0 after Leave", got)
	}
	if r.WispCage.Len() != 0 {
		t.Error("WispCage entries should be removed after Leave")
	}
}

func TestRouter_HandleEvent_Move_UpdatesMobPosition(t *testing.T) {
	r := newTestRouter(t)
	r.Mobs.HandleNewMob(Params{0: int32(1), 1: int32(0), 2: int32(255), 7: []float32{0, 0}})

	// PostProcessEvent already places decoded coords at Parameters[4]/[5] before HandleEvent
	// ever sees the event - simulated directly here.
	r.HandleEvent(eventWithCode(eventcodes.Move, Params{0: int32(1), 4: float32(99), 5: float32(88)}))

	m := r.Mobs.MobSnapshot()[0]
	if m.PosX != 99 || m.PosY != 88 {
		t.Errorf("mob position = (%v,%v), want (99,88)", m.PosX, m.PosY)
	}
}

func TestRouter_HandleEvent_NewRandomDungeonExit_MistsDungeonTagRoutesToMistsDungeonState(t *testing.T) {
	r := newTestRouter(t)

	r.HandleEvent(eventWithCode(eventcodes.NewRandomDungeonExit, Params{
		0: int32(1), 1: []float32{10, 20}, 15: "MISTS_DUNGEON_ABBEY",
	}))

	if r.MistsDungeon.Len() != 1 {
		t.Error("a MISTS_DUNGEON-tagged exit should route to MistsDungeonState, not DungeonsState")
	}
	if len(r.Dungeons.Snapshot()) != 0 {
		t.Error("DungeonsState should not receive a MISTS_DUNGEON-tagged exit")
	}
}

func TestRouter_HandleEvent_NewRandomDungeonExit_RegularDungeonRoutesToDungeonsState(t *testing.T) {
	r := newTestRouter(t)

	r.HandleEvent(eventWithCode(eventcodes.NewRandomDungeonExit, Params{
		0: int32(1), 1: []float32{10, 20}, 3: "SOLO_STANDARD_1",
	}))

	if len(r.Dungeons.Snapshot()) != 1 {
		t.Error("a regular dungeon exit should route to DungeonsState")
	}
	if r.MistsDungeon.Len() != 0 {
		t.Error("MistsDungeonState should not receive a regular dungeon exit")
	}
}

func TestRouter_HandleEvent_HealthUpdate_DispatchesToPlayersLooseAndMobsFull(t *testing.T) {
	r := newTestRouter(t)
	r.Players.HandleNewCharacter(1, Params{1: "P"})
	r.Mobs.HandleNewMob(Params{0: int32(2), 1: int32(0), 2: int32(255), 7: []float32{0, 0}, 13: int32(1000)})

	// Same event code (HealthUpdate=6) fired once per id, matching how a real batch of
	// per-entity events would arrive.
	r.HandleEvent(eventWithCode(eventcodes.HealthUpdate, Params{0: int32(1), 3: int32(50)}))
	r.HandleEvent(eventWithCode(eventcodes.HealthUpdate, Params{0: int32(2), 3: int32(500)}))

	if got := r.Players.Snapshot()[0].CurrentHealth; got != 50 {
		t.Errorf("player CurrentHealth = %v, want 50 (UpdatePlayerLooseHealth)", got)
	}
	if got := r.Mobs.MobSnapshot()[0].Health; got == 0 {
		t.Error("mob Health should have been updated (updateMobHealth)")
	}
}

func TestRouter_HandleEvent_ChangeFlaggingFinished(t *testing.T) {
	r := newTestRouter(t)
	r.Players.HandleNewCharacter(1, Params{1: "P", 53: int32(FactionPassive)})

	r.HandleEvent(eventWithCode(eventcodes.ChangeFlaggingFinished, Params{0: int32(1), 1: int32(FactionHostile)}))

	if got := r.Players.Snapshot()[0].Faction; got != FactionHostile {
		t.Errorf("Faction = %d, want %d", got, FactionHostile)
	}
}

func TestRouter_HandleEvent_PartyLifecycle(t *testing.T) {
	r := newTestRouter(t)

	r.HandleEvent(eventWithCode(eventcodes.PartyJoined, Params{9: []string{"Ally"}}))
	if !r.PartyRoster.IsPartyMember("Ally") {
		t.Fatal("PartyJoined should register the member")
	}

	r.HandleEvent(eventWithCode(eventcodes.PartyDisbanded, nil))
	if r.PartyRoster.IsPartyMember("Ally") {
		t.Error("PartyDisbanded should clear the roster")
	}
}

func requestWithCode(code int, extra Params) *photon.OperationRequest {
	params := map[byte]interface{}{253: int32(code)}
	for k, v := range extra {
		params[k] = v
	}
	return &photon.OperationRequest{Parameters: params}
}

func TestRouter_HandleRequest_MoveUpdatesLocalPosition(t *testing.T) {
	r := newTestRouter(t)

	r.HandleRequest(requestWithCode(operationcodes.Move, Params{1: []float32{5, 6}}))

	if r.Session.LocalX != 5 || r.Session.LocalY != 6 {
		t.Errorf("Session position = (%v,%v), want (5,6)", r.Session.LocalX, r.Session.LocalY)
	}
}

func responseWithCode(code int, extra Params) *photon.OperationResponse {
	params := map[byte]interface{}{253: int32(code)}
	for k, v := range extra {
		params[k] = v
	}
	return &photon.OperationResponse{Parameters: params}
}

func TestRouter_HandleResponse_ChangeClusterTriggersZoneChangeAndClear(t *testing.T) {
	r := newTestRouter(t)
	r.Chests.HandleNewLootChest(Params{0: int32(1), 1: []float32{0, 0}, 3: "C"})
	r.Session.CurrentZoneID = "A"

	cleared := false
	r.HandleResponse(responseWithCode(operationcodes.ChangeCluster, Params{0: "B"}), func() {
		cleared = true
		r.ClearAll()
	})

	if r.Session.CurrentZoneID != "B" {
		t.Errorf("CurrentZoneID = %q, want B", r.Session.CurrentZoneID)
	}
	if !cleared {
		t.Error("clearHandlers callback should have been invoked on a real zone change")
	}
	if r.Chests.Len() != 0 {
		t.Error("ClearAll should have wiped the chest list")
	}
}

func TestRouter_HandleResponse_SameZoneIsNoop(t *testing.T) {
	r := newTestRouter(t)
	r.Session.CurrentZoneID = "A"

	called := false
	r.HandleResponse(responseWithCode(operationcodes.ChangeCluster, Params{0: "A"}), func() { called = true })

	if called {
		t.Error("a ChangeCluster response reporting the already-current zone should be a no-op")
	}
}

func TestRouter_HandleResponse_JoinDecodesPositionAndZone(t *testing.T) {
	r := newTestRouter(t)

	r.HandleResponse(responseWithCode(operationcodes.Join, Params{
		8: "NewZone", 9: []float32{1, 2},
	}), func() {})

	if r.Session.CurrentZoneID != "NewZone" {
		t.Errorf("CurrentZoneID = %q, want NewZone", r.Session.CurrentZoneID)
	}
	if r.Session.LocalX != 1 || r.Session.LocalY != 2 {
		t.Errorf("Session position = (%v,%v), want (1,2)", r.Session.LocalX, r.Session.LocalY)
	}
}
