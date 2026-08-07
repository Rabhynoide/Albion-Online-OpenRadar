package radarstate

import (
	"strings"

	"github.com/nospy/albion-openradar/internal/photon"
	"github.com/nospy/albion-openradar/internal/photon/eventcodes"
	"github.com/nospy/albion-openradar/internal/photon/operationcodes"
)

// Router is a Go port of web/scripts/core/EventRouter.js's onEvent/onRequest/onResponse
// dispatch - the single place that routes already-decoded Photon events into the right
// handler(s), mirroring EventRouter.js's switch statement event-code-for-event-code (see that
// file for the authoritative mapping this was transcribed from). Registered as a second,
// in-process subscriber alongside internal/server's WebSocket broadcast (see
// cmd/radar/main.go's onPhotonEvent/onPhotonRequest/onPhotonResponse) - both consume the same
// already-decoded *photon.EventData etc., neither knows about the other.
type Router struct {
	Players        *PlayersState
	Mobs           *MobsState
	Harvestables   *HarvestablesState
	Chests         *ChestsState
	Dungeons       *DungeonsState
	Fishing        *FishingState
	LocalTreasures *LocalTreasuresState
	MistsDungeon   *MistsDungeonState
	WispCage       *WispCageState
	PartyRoster    *PartyRoster
	Session        *Session
	ZoneGraph      *ZoneGraph
}

// HandleEvent ports EventRouter.js's onEvent. id mirrors `parseInt(Parameters[0])`, used by
// several branches (Leave, NewCharacter, CharacterEquipmentChanged, Mounted) exactly as in JS.
//
// The dispatch key is Parameters[252], exactly as EventRouter.js's own `const eventCode =
// Parameters[252]` reads it - NOT event.Code. event.Code is the raw single-byte Photon
// envelope field (internal/photon.EventData.Code, capped at 255); most of eventcodes.go's
// Albion-specific codes exceed that (e.g. NewLootChest=393), so the real code only ever lives
// in the wire dictionary's own 252 entry - PostProcessEvent (internal/photon/events.go) only
// falls back to event.Code when the dictionary omits 252 entirely, and JS never even looks at
// the raw envelope code, only ever the dictionary field.
func (r *Router) HandleEvent(event *photon.EventData) {
	p := Params(event.Parameters)
	id, _ := paramInt(p, 0)
	code, _ := paramInt(p, 252)

	switch code {
	case eventcodes.Leave:
		r.Players.Remove(id)
		r.Mobs.RemoveMist(id)
		r.Mobs.Remove(id)
		r.Dungeons.Remove(id)
		r.Chests.Remove(id)
		r.Fishing.RemoveFish(id)
		r.WispCage.RemoveCage(id)
		r.MistsDungeon.RemovePortal(id)
		r.LocalTreasures.Remove(id)

	case eventcodes.Move:
		// PostProcessEvent already extracted posX/posY into Parameters[4]/[5] from the raw
		// move buffer (see internal/photon/events.go's extractMovePositions).
		x, xok := paramFloat32(p, 4)
		y, yok := paramFloat32(p, 5)
		if xok && yok {
			r.Mobs.UpdateMistPosition(id, x, y)
			r.Mobs.UpdatePosition(id, x, y)
		}

	case eventcodes.NewCharacter:
		r.Players.HandleNewCharacter(id, p)

	case eventcodes.NewSimpleHarvestableObjectList:
		r.Harvestables.HandleNewSimpleHarvestableObjectList(p)

	case eventcodes.NewHarvestableObject:
		r.Harvestables.HandleNewHarvestableObject(id, p)

	case eventcodes.HarvestableChangeState:
		r.Harvestables.HandleHarvestableChangeState(p)

	case eventcodes.MobChangeState:
		r.Mobs.HandleEnchantChanged(p)

	case eventcodes.RegenerationHealthChanged:
		r.Players.HandleHealthUpdate(p)
		r.Mobs.HandleHealthRegen(p)

	case eventcodes.HealthUpdate:
		r.Players.HandleLooseHealthUpdate(p)
		r.Mobs.HandleHealthUpdate(p)

	case eventcodes.HealthUpdates:
		r.Mobs.HandleHealthUpdateBulk(p)

	case eventcodes.CharacterEquipmentChanged:
		r.Players.HandleEquipmentChanged(id, p)

	case eventcodes.NewMob:
		r.Mobs.HandleNewMob(p)

	case eventcodes.Mounted:
		r.Players.HandleMounted(id, p)

	case eventcodes.NewRandomDungeonExit:
		// A tag at Parameters[15] starting with "MISTS_DUNGEON" (Knightfall Abbey) is
		// intercepted here and routed to MistsDungeon entirely - it never reaches
		// DungeonsState, and uses Parameters[1] as position (not [8] like a regular dungeon).
		if tag, ok := paramString(p, 15); ok && strings.HasPrefix(tag, "MISTS_DUNGEON") {
			if x, y, ok := paramPosition(p, 1); ok {
				r.MistsDungeon.AddPortal(id, x, y, tag)
			}
			return
		}
		r.Dungeons.HandleNewRandomDungeonExit(p)

	case eventcodes.NewLootChest:
		r.Chests.HandleNewLootChest(p)

	case eventcodes.NewCagedObject:
		r.WispCage.HandleNewCagedObject(p)

	case eventcodes.CagedObjectStateUpdated:
		r.WispCage.HandleCagedObjectStateUpdated(p)

	case eventcodes.LocalTreasuresUpdate:
		r.LocalTreasures.HandleLocalTreasuresUpdate(p)

	case eventcodes.NewFishingZoneObject:
		r.Fishing.HandleNewFishingZoneObject(p)

	case eventcodes.FishingFinished:
		r.Fishing.HandleFishingFinished(p)

	case eventcodes.ChangeFlaggingFinished:
		faction, ok := paramInt(p, 1)
		if ok {
			r.Players.HandleFactionChanged(id, faction)
		}

	case eventcodes.PartyJoined:
		r.PartyRoster.HandlePartyJoined(p)

	case eventcodes.PartyPlayerLeft:
		r.PartyRoster.HandlePartyPlayerLeft(p)

	case eventcodes.PartyDisbanded:
		r.PartyRoster.HandlePartyDisbanded()

	case eventcodes.MistsPlayerJoinedInfo:
		// Simplified from EventRouter.js's applyMapChange: records the zone change (for
		// GPS/pvp-type resolution) but not the fuller Mist-portal-chain PvP-inheritance
		// tracking - see internal/radarstate's package doc and
		// docs/technical/NATIVE_OVERLAY_CLIENT.md for the follow-up.
		if newMapID, ok := paramString(p, 2); ok && newMapID != "" {
			if isJoined, ok := paramBool(p, 3); ok && isJoined {
				r.Session.ChangeZone(newMapID, r.ZoneGraph)
			}
		}
	}
}

// HandleRequest ports EventRouter.js's onRequest - currently just the local-player-position
// half of its Move handling (opcode 21 legacy / Move). Dispatch key is Parameters[253], same
// reasoning as HandleEvent's Parameters[252] (see its doc comment) - PostProcessRequest only
// falls back to the raw envelope OperationCode when the dictionary omits 253.
func (r *Router) HandleRequest(req *photon.OperationRequest) {
	p := Params(req.Parameters)
	code, _ := paramInt(p, 253)
	if code == 21 || code == operationcodes.Move {
		r.Session.HandleMoveRequest(p)
	}
}

// HandleResponse ports EventRouter.js's onResponse's map-change detection (ChangeCluster,
// legacy op 35, Join). clearHandlers mirrors clearHandlersCallback: entities from the zone
// just left are no longer relevant once a real zone change is confirmed. Dispatch key is
// Parameters[253], same reasoning as HandleEvent (see its doc comment).
func (r *Router) HandleResponse(resp *photon.OperationResponse, clearHandlers func()) {
	p := Params(resp.Parameters)
	code, _ := paramInt(p, 253)

	switch code {
	case operationcodes.ChangeCluster:
		r.handleZoneChangeResponse(p, 0, clearHandlers)
	case 35: // upstream 35 = InventoryStack; this project's wire treats it as legacy map-change.
		r.handleZoneChangeResponse(p, 0, clearHandlers)
	case operationcodes.Join:
		r.Session.HandleJoinPosition(p[9])
		r.handleZoneChangeResponse(p, 8, clearHandlers)
	}
}

func (r *Router) handleZoneChangeResponse(p Params, zoneIDKey byte, clearHandlers func()) {
	newZoneID, ok := paramString(p, zoneIDKey)
	if !ok || newZoneID == "" || newZoneID == r.Session.CurrentZoneID {
		return
	}
	r.Session.ChangeZone(newZoneID, r.ZoneGraph)
	if clearHandlers != nil {
		clearHandlers()
	}
}

// ClearAll mirrors the effect of EventRouter.js's clearHandlersCallback: wipes every
// zone-scoped entity list on a confirmed zone change.
func (r *Router) ClearAll() {
	r.Players.Clear()
	r.Mobs.Clear()
	r.Harvestables.Clear()
	r.Chests.Clear()
	r.Dungeons.Clear()
	r.Fishing.Clear()
	r.LocalTreasures.Clear()
	r.MistsDungeon.Clear()
	r.WispCage.Clear()
}
