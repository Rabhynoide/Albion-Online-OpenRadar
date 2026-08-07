package radarstate

// MistsDungeonPortal mirrors web/scripts/handlers/MistsDungeonHandler.js's record (Knightfall
// Abbey / Mists dungeon entrances).
type MistsDungeonPortal struct {
	ID   int
	PosX float32
	PosY float32
	Name string
}

// MistsDungeonState is a Go port of MistsDungeonHandler.js.
type MistsDungeonState struct {
	portals *entityList[int, MistsDungeonPortal]
}

func NewMistsDungeonState() *MistsDungeonState {
	return &MistsDungeonState{portals: newEntityList[int, MistsDungeonPortal]()}
}

// AddPortal mirrors MistsDungeonHandler.js's addPortal.
func (s *MistsDungeonState) AddPortal(id int, posX, posY float32, name string) {
	s.portals.upsert(id, func() MistsDungeonPortal {
		return MistsDungeonPortal{ID: id, PosX: posX, PosY: posY, Name: name}
	})
}

func (s *MistsDungeonState) RemovePortal(id int) { s.portals.remove(id) }
func (s *MistsDungeonState) Clear()              { s.portals.clear() }
func (s *MistsDungeonState) Snapshot() []MistsDungeonPortal {
	return s.portals.snapshot()
}
func (s *MistsDungeonState) Len() int { return s.portals.len() }

// CleanupStale mirrors MistsDungeonHandler.js's default maxAgeMs (130000, slightly longer than
// the 120000 most other handlers use).
func (s *MistsDungeonState) CleanupStale(maxAgeMs int) int {
	return s.portals.cleanupStale(msToDuration(maxAgeMs))
}
