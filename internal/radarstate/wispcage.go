package radarstate

// WispCage mirrors web/scripts/handlers/WispCageHandler.js's Cage record.
type WispCage struct {
	ID   int
	PosX float32
	PosY float32
	Name string
}

// WispCageState is a Go port of WispCageHandler.js. The `settingCage` gate the JS applies is
// enforced by the caller (internal/overlay reads the same syncsettings key before invoking
// these) rather than baked in here, so this package stays render/settings-agnostic.
type WispCageState struct {
	cages *entityList[int, WispCage]
}

func NewWispCageState() *WispCageState {
	return &WispCageState{cages: newEntityList[int, WispCage]()}
}

// HandleNewCagedObject ports WispCageHandler.js's newCageEvent: id=[0], position=[2], name=[4].
func (s *WispCageState) HandleNewCagedObject(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	x, y, ok := paramPosition(p, 2)
	if !ok {
		return
	}
	name, _ := paramString(p, 4)

	s.cages.upsert(id, func() WispCage {
		return WispCage{ID: id, PosX: x, PosY: y, Name: name}
	})
}

// HandleCagedObjectStateUpdated ports WispCageHandler.js's cageOpenedEvent: an opened cage is
// removed from the map (id=[0]).
func (s *WispCageState) HandleCagedObjectStateUpdated(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	s.cages.remove(id)
}

func (s *WispCageState) RemoveCage(id int)    { s.cages.remove(id) }
func (s *WispCageState) Clear()               { s.cages.clear() }
func (s *WispCageState) Snapshot() []WispCage { return s.cages.snapshot() }
func (s *WispCageState) Len() int             { return s.cages.len() }
func (s *WispCageState) CleanupStale(maxAgeMs int) int {
	return s.cages.cleanupStale(msToDuration(maxAgeMs))
}
