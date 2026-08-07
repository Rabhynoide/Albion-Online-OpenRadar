package radarstate

// Fish mirrors web/scripts/handlers/FishingHandler.js's Fish record.
type Fish struct {
	ID              int
	PosX            float32
	PosY            float32
	Type            string
	SizeSpawned     int
	SizeLeftToSpawn int
	TotalSize       int
}

// FishingState is a Go port of FishingHandler.js.
type FishingState struct {
	fishes *entityList[int, Fish]
}

func NewFishingState() *FishingState {
	return &FishingState{fishes: newEntityList[int, Fish]()}
}

// HandleNewFishingZoneObject ports FishingHandler.js's newFishEvent/upsertFish: id=[0],
// position=[1], sizeSpawned=[2], sizeLeftToSpawn=[3], type=[4]. Unlike most handlers, an
// already-known fish is fully updated (size changes as it's caught), not just touched.
func (s *FishingState) HandleNewFishingZoneObject(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	fishType, ok := paramString(p, 4)
	if !ok || fishType == "" {
		return
	}
	x, y, ok := paramPosition(p, 1)
	if !ok {
		return
	}
	sizeSpawned := paramIntDefault(p, 2, 0)
	sizeLeftToSpawn := paramIntDefault(p, 3, 0)

	s.fishes.replace(id, Fish{
		ID: id, PosX: x, PosY: y, Type: fishType,
		SizeSpawned: sizeSpawned, SizeLeftToSpawn: sizeLeftToSpawn,
		TotalSize: sizeSpawned + sizeLeftToSpawn,
	})
}

// HandleFishingFinished ports FishingHandler.js's fishingEnd: id=[0].
func (s *FishingState) HandleFishingFinished(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	s.fishes.remove(id)
}

func (s *FishingState) RemoveFish(id int) { s.fishes.remove(id) }
func (s *FishingState) Clear()            { s.fishes.clear() }
func (s *FishingState) Snapshot() []Fish  { return s.fishes.snapshot() }
func (s *FishingState) Len() int          { return s.fishes.len() }
func (s *FishingState) CleanupStale(maxAgeMs int) int {
	return s.fishes.cleanupStale(msToDuration(maxAgeMs))
}
