package radarstate

import (
	"sync"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

// Faction values mirror the wire encoding exactly (0=passive, 1-6=faction city, 255=hostile),
// same as web/scripts/handlers/PlayersHandler.js's Player class.
const (
	FactionPassive = 0
	FactionHostile = 255
)

// Player mirrors PlayersHandler.js's Player record. Live player positions are XOR-encrypted
// and not decryptable client-side (see web/scripts/drawings/PlayersDrawing.js), so PosX/PosY
// stay at their zero value here just as in the JS version - only used for API-shape parity,
// never for drawing a dot on the map.
type Player struct {
	ID            int
	Nickname      string
	GuildName     string
	AllianceName  string
	Faction       int
	CurrentHealth float32
	InitialHealth float32
	Equipments    []int
	Spells        []int
	Items         []int
	Mounted       bool
}

func (p Player) IsHostile() bool       { return p.Faction == FactionHostile }
func (p Player) IsPassive() bool       { return p.Faction == FactionPassive }
func (p Player) IsFactionPlayer() bool { return p.Faction >= 1 && p.Faction <= 6 }

// AverageItemPower ports Player.getAverageItemPower: averages the item power of combat
// equipment slots only (0-4 = MainHand/OffHand/Head/Chest/Shoes, 8 = Food; 5=Cape, 6=Mount,
// 7=Bag are excluded), or nil if no valid combat items are equipped.
func AverageItemPower(equipments []int, items *gamedata.ItemsDatabase) *int {
	if len(equipments) == 0 || items == nil {
		return nil
	}
	total, valid := 0, 0
	for i, itemID := range equipments {
		if (i > 4 && i != 8) || itemID <= 0 {
			continue
		}
		item, ok := items.GetByID(itemID)
		if ok && item.ItemPower > 0 {
			total += item.ItemPower
			valid++
		}
	}
	if valid == 0 {
		return nil
	}
	avg := (total + valid/2) / valid // round to nearest, matching JS's Math.round
	return &avg
}

// IsPlayerThreat mirrors PlayersHandler.js's isPlayerThreat exactly: safe zones never alert,
// black zones treat every player as hostile, red/yellow (and the fallback default) only alert
// on faction 255.
func IsPlayerThreat(faction int, pvpType string) bool {
	switch pvpType {
	case "safe":
		return false
	case "black":
		return true
	default: // "red", "yellow", or any unrecognized value
		return faction == FactionHostile
	}
}

// PlayersState is a Go port of PlayersHandler.js. Unlike the JS version, it never reads
// settings or triggers audio/DOM side effects directly (there is no browser here) - filter
// toggles are passed in by the caller at read time (FilteredPlayers/ThreatPlayers), and
// alert-worthy transitions are surfaced via PendingAlerts() for internal/overlay to turn into
// an actual native sound/flash after applying its own settingFlash/settingSound/exclusion
// checks (mirrors maybeAlert's exclusion+settings gate, just split across two packages).
//
// pendingAlerts is guarded by its own mutex, separate from entityList's internal locking: it's
// written from whatever goroutine dispatches Photon events (queueAlert, via HandleNewCharacter/
// HandleFactionChanged) and read from internal/overlay's Ebiten goroutine (PendingAlerts) - a
// real cross-goroutine access, not a hypothetical one, once anything actually calls
// PendingAlerts (nothing did until the overlay's alert wiring was added).
type PlayersState struct {
	players     *entityList[int, Player]
	partyRoster *PartyRoster
	isIgnored   func(nickname string) bool
	zones       *gamedata.ZonesDatabase
	currentZone string
	maxPlayers  int

	alertsMu      sync.Mutex
	pendingAlerts []string // nicknames that just became a threat, drained by PendingAlerts()
}

// NewPlayersState constructs a PlayersState. isIgnored should reflect the current Ignore List
// (see internal/syncsettings's "ignoreList" key) - injected rather than read directly so this
// package stays settings-store-agnostic.
func NewPlayersState(partyRoster *PartyRoster, zones *gamedata.ZonesDatabase, isIgnored func(string) bool) *PlayersState {
	return &PlayersState{
		players:     newEntityList[int, Player](),
		partyRoster: partyRoster,
		zones:       zones,
		isIgnored:   isIgnored,
		maxPlayers:  100, // hard ceiling mirroring handleNewPlayerEvent's Math.min(100, ...)
	}
}

// SetCurrentZone mirrors reading window.currentMapId - the zone-tracking layer (see
// zonetracking.go) should call this on every confirmed zone change.
func (s *PlayersState) SetCurrentZone(zoneID string) {
	s.currentZone = zoneID
}

// getAlertPvpType mirrors PlayersHandler.js's getAlertPvpType: falls back to "yellow" (not
// "safe") for a zone missing from zones.json specifically for alerting, so a hostile player in
// an unmapped zone still triggers an alert (issue #65) instead of being silently suppressed by
// ZonesDatabase.PvpType's more permissive "safe" default used for general UI coloring.
func (s *PlayersState) getAlertPvpType() string {
	if s.zones == nil {
		return "yellow"
	}
	if _, ok := s.zones.GetZone(s.currentZone); !ok {
		return "yellow"
	}
	return s.zones.PvpType(s.currentZone)
}

// isExcludedPlayer mirrors isExcludedPlayer: a player on the Ignore List or in the local
// player's own party is excluded from alerts and from the radar/list entirely.
func (s *PlayersState) isExcludedPlayer(nickname string) bool {
	if s.partyRoster != nil && s.partyRoster.IsPartyMember(nickname) {
		return true
	}
	return s.isIgnored != nil && s.isIgnored(nickname)
}

// HandleNewCharacter ports handleNewPlayerEvent: id (Parameters[0], read by the caller per
// EventRouter.js's own convention) plus nickname=[1], guildName=[8], faction=[53],
// allianceName=[51], equipments=[40], spells=[43]. The settingShowPlayers gate and the
// configurable max-players-displayed cap are the caller's responsibility (overlay reads
// syncsettings); this only enforces the hard ceiling of 100 the JS applies regardless.
func (s *PlayersState) HandleNewCharacter(id int, p Params) {
	if s.players.len() >= s.maxPlayers {
		return
	}
	nickname := paramStringDefault(p, 1, "")
	guildName := paramStringDefault(p, 8, "")
	faction := paramIntDefault(p, 53, FactionPassive)
	allianceName := paramStringDefault(p, 51, "")
	equipments := paramIntSlice(p, 40)
	spells := paramIntSlice(p, 43)

	s.players.upsert(id, func() Player {
		return Player{
			ID: id, Nickname: nickname, GuildName: guildName, Faction: faction,
			AllianceName: allianceName, Equipments: equipments, Spells: spells,
		}
	})

	if IsPlayerThreat(faction, s.getAlertPvpType()) && s.currentZone != "" {
		s.queueAlert(nickname)
	}
}

// queueAlert mirrors maybeAlert's exclusion check - only the actual flash/sound side effect is
// deferred to the caller.
func (s *PlayersState) queueAlert(nickname string) {
	if s.isExcludedPlayer(nickname) {
		return
	}
	s.alertsMu.Lock()
	s.pendingAlerts = append(s.pendingAlerts, nickname)
	s.alertsMu.Unlock()
}

// PendingAlerts drains and returns the nicknames that became a threat since the last call.
func (s *PlayersState) PendingAlerts() []string {
	s.alertsMu.Lock()
	defer s.alertsMu.Unlock()
	out := s.pendingAlerts
	s.pendingAlerts = nil
	return out
}

// HandleMounted ports handleMountedPlayerEvent's slightly odd "-1 in slot 10 also means
// mounted" fallback, kept verbatim even though its origin is undocumented in the JS.
func (s *PlayersState) HandleMounted(id int, p Params) {
	if mounted, ok := paramBool(p, 11); ok && mounted {
		s.setMounted(id, true)
		return
	}
	if paramStringDefault(p, 10, "") == "-1" {
		s.setMounted(id, true)
		return
	}
	s.setMounted(id, false)
}

func (s *PlayersState) setMounted(id int, mounted bool) {
	s.players.mutate(id, func(pl *Player) { pl.Mounted = mounted })
}

// HandleEquipmentChanged ports updateItems: items=[2].
func (s *PlayersState) HandleEquipmentChanged(id int, p Params) {
	items := paramIntSlice(p, 2)
	if items == nil {
		return
	}
	s.players.mutate(id, func(pl *Player) { pl.Items = items })
}

// HandleHealthUpdate ports UpdatePlayerHealth: currentHealth=[2], initialHealth=[3].
func (s *PlayersState) HandleHealthUpdate(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	current, _ := paramFloat32(p, 2)
	initial, _ := paramFloat32(p, 3)
	s.players.mutate(id, func(pl *Player) {
		pl.CurrentHealth = current
		pl.InitialHealth = initial
	})
}

// HandleLooseHealthUpdate ports UpdatePlayerLooseHealth: currentHealth=[3] only.
func (s *PlayersState) HandleLooseHealthUpdate(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	current, ok := paramFloat32(p, 3)
	if !ok {
		return
	}
	s.players.mutate(id, func(pl *Player) { pl.CurrentHealth = current })
}

// HandleFactionChanged ports updatePlayerFaction(id, newFaction) - queues an alert if the
// player just became hostile (matches triggerHostileAlert's wasHostile/isHostile comparison).
func (s *PlayersState) HandleFactionChanged(id, newFaction int) {
	player, ok := s.players.get(id)
	if !ok {
		return
	}
	wasHostile := player.IsHostile()
	s.players.mutate(id, func(pl *Player) { pl.Faction = newFaction })

	if !wasHostile && newFaction == FactionHostile && IsPlayerThreat(newFaction, s.getAlertPvpType()) {
		s.queueAlert(player.Nickname)
	}
}

func (s *PlayersState) Remove(id int) { s.players.remove(id) }
func (s *PlayersState) Clear()        { s.players.clear() }
func (s *PlayersState) Len() int      { return s.players.len() }
func (s *PlayersState) Snapshot() []Player {
	return s.players.snapshot()
}

// CleanupStale mirrors cleanupStaleEntities's default maxAgeMs (300000 = 5 minutes, longer
// than most other handlers' 2-minute default since players move around more).
func (s *PlayersState) CleanupStale(maxAgeMs int) int {
	return s.players.cleanupStale(msToDuration(maxAgeMs))
}

// EnforceMaxSize mirrors enforceMaxSize: if over maxSize, keeps only the most-recently-seen
// entries.
func (s *PlayersState) EnforceMaxSize(maxSize int) int {
	return s.players.enforceMaxSize(maxSize)
}

// FilteredPlayers ports getFilteredPlayers: excluded (ignore list/party) players are always
// dropped; in a black zone every remaining player is a threat, gated only by showDangerous; in
// any other zone, filtering is by the player's own faction category.
func (s *PlayersState) FilteredPlayers(showPassive, showFaction, showDangerous bool) []Player {
	pvpType := "safe"
	if s.zones != nil {
		pvpType = s.zones.PvpType(s.currentZone)
	}

	all := s.players.snapshot()
	out := make([]Player, 0, len(all))
	for _, p := range all {
		if s.isExcludedPlayer(p.Nickname) {
			continue
		}
		if pvpType == "black" {
			if showDangerous {
				out = append(out, p)
			}
			continue
		}
		switch {
		case p.IsPassive():
			if showPassive {
				out = append(out, p)
			}
		case p.IsFactionPlayer():
			if showFaction {
				out = append(out, p)
			}
		default:
			if showDangerous {
				out = append(out, p)
			}
		}
	}
	return out
}

// PlayersByType ports getPlayersByType.
func (s *PlayersState) PlayersByType(showPassive, showFaction, showDangerous bool) (hostile, faction, passive []Player) {
	filtered := s.FilteredPlayers(showPassive, showFaction, showDangerous)

	pvpType := "safe"
	if s.zones != nil {
		pvpType = s.zones.PvpType(s.currentZone)
	}
	if pvpType == "black" {
		return filtered, nil, nil
	}

	for _, p := range filtered {
		switch {
		case p.IsHostile():
			hostile = append(hostile, p)
		case p.IsFactionPlayer():
			faction = append(faction, p)
		case p.IsPassive():
			passive = append(passive, p)
		}
	}
	return hostile, faction, passive
}

// ThreatPlayers ports getThreatPlayers: drives the overlay's pulsing threat-border warning.
func (s *PlayersState) ThreatPlayers() []Player {
	pvpType := s.getAlertPvpType()
	all := s.players.snapshot()
	out := make([]Player, 0)
	for _, p := range all {
		if !s.isExcludedPlayer(p.Nickname) && IsPlayerThreat(p.Faction, pvpType) {
			out = append(out, p)
		}
	}
	return out
}
