package radarstate

import "strings"

// DungeonType mirrors web/scripts/handlers/DungeonsHandler.js's DungeonType enum.
type DungeonType int

const (
	DungeonSolo DungeonType = iota
	DungeonGroup
	DungeonCorrupted
	DungeonHellgate
	// DungeonMistSolo/DungeonMistGroup route through the Mists settings rather than the
	// Dungeon settings in the JS UI - kept as distinct types here so the overlay's settings
	// filter can tell them apart from a regular Solo/Group dungeon at draw time.
	DungeonMistSolo
	DungeonMistGroup
)

// Dungeon mirrors web/scripts/handlers/DungeonsHandler.js's Dungeon record. Unlike the JS
// version, classification never gates whether an entry is stored - every dungeon is always
// tracked with its Type/Enchant tag, and the overlay's settings filter decides what to draw at
// render time (so toggling a filter on immediately reveals already-tracked entries, instead of
// only affecting entries that arrive after the toggle - a deliberate improvement over the JS
// ingestion-time gate, see internal/radarstate package doc).
type Dungeon struct {
	ID      int
	PosX    float32
	PosY    float32
	Name    string
	Type    DungeonType
	Enchant int
}

// DungeonsState is a Go port of DungeonsHandler.js.
type DungeonsState struct {
	dungeons *entityList[int, Dungeon]
}

func NewDungeonsState() *DungeonsState {
	return &DungeonsState{dungeons: newEntityList[int, Dungeon]()}
}

// HandleNewRandomDungeonExit ports DungeonsHandler.js's dungeonEvent/addDungeon: id=[0],
// position=[1], name=[3] (or [15] post-Knightfall), enchant=[8].
func (s *DungeonsState) HandleNewRandomDungeonExit(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	x, y, ok := paramPosition(p, 1)
	if !ok {
		return
	}
	name := paramStringDefault(p, 3, "")
	if name == "" {
		name = paramStringDefault(p, 15, "")
	}
	enchant := paramIntDefault(p, 8, 0)

	s.dungeons.upsert(id, func() Dungeon {
		return Dungeon{ID: id, PosX: x, PosY: y, Name: name, Type: classifyDungeon(name), Enchant: enchant}
	})
}

// classifyDungeon mirrors DungeonsHandler.js's addDungeon name-sniffing exactly, including
// check order (corrupted dungeons contain "solo" in their name too, so corrupted must be
// checked first; hellgate names like "HELLGATE_2V2_NON_LETHAL" fall through to the final
// group-name-match on their name as a whole, matched last as the JS default/else branch does).
func classifyDungeon(name string) DungeonType {
	upper := strings.ToUpper(name)
	lower := strings.ToLower(name)

	switch {
	case strings.HasPrefix(upper, "MISTS_"):
		if strings.Contains(upper, "_SOLO_") {
			return DungeonMistSolo
		}
		return DungeonMistGroup
	case strings.Contains(lower, "corrupted"):
		return DungeonCorrupted
	case strings.Contains(lower, "solo"):
		return DungeonSolo
	case strings.Contains(lower, "hellgate"):
		return DungeonHellgate
	default:
		return DungeonGroup
	}
}

func (s *DungeonsState) Remove(id int) { s.dungeons.remove(id) }
func (s *DungeonsState) Clear()        { s.dungeons.clear() }
func (s *DungeonsState) Snapshot() []Dungeon {
	return s.dungeons.snapshot()
}
func (s *DungeonsState) Len() int { return s.dungeons.len() }
func (s *DungeonsState) CleanupStale(maxAgeMs int) int {
	return s.dungeons.cleanupStale(msToDuration(maxAgeMs))
}
