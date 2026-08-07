package radarstate

import "strings"

// Chest mirrors web/scripts/handlers/ChestsHandler.js's Chest record.
type Chest struct {
	ID     int
	PosX   float32
	PosY   float32
	Name   string
	Rarity *int // nil when the event carried no rarity (JS: `Parameters[5] ?? null`)
}

// ChestsState is a Go port of ChestsHandler.js.
type ChestsState struct {
	chests *entityList[int, Chest]
}

func NewChestsState() *ChestsState {
	return &ChestsState{chests: newEntityList[int, Chest]()}
}

// HandleNewLootChest ports ChestsHandler.js's addChestEvent: id=[0], position=[1], name=[3]
// (or [4] if [3] mentions "mist" - Mist treasure chests carry their real name one slot later),
// rarity=[5].
func (s *ChestsState) HandleNewLootChest(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	x, y, ok := paramPosition(p, 1)
	if !ok {
		return
	}
	name, _ := paramString(p, 3)
	if strings.Contains(strings.ToLower(name), "mist") {
		name, _ = paramString(p, 4)
	}
	var rarity *int
	if r, ok := paramInt(p, 5); ok {
		rarity = &r
	}

	s.chests.upsert(id, func() Chest {
		return Chest{ID: id, PosX: x, PosY: y, Name: name, Rarity: rarity}
	})
}

func (s *ChestsState) Remove(id int)     { s.chests.remove(id) }
func (s *ChestsState) Clear()            { s.chests.clear() }
func (s *ChestsState) Snapshot() []Chest { return s.chests.snapshot() }
func (s *ChestsState) Len() int          { return s.chests.len() }
func (s *ChestsState) CleanupStale(maxAgeMs int) int {
	return s.chests.cleanupStale(msToDuration(maxAgeMs))
}
