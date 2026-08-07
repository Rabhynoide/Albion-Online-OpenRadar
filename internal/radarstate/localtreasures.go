package radarstate

import "strings"

// LocalTreasure mirrors web/scripts/handlers/LocalTreasuresHandler.js's Treasure record
// (buried chests, temporary rich resource nodes, smuggler piles, timed special events).
type LocalTreasure struct {
	ID         int
	PosX       float32
	PosY       float32
	Label      string
	StartTicks int
	EndTicks   int
}

// excludedLocalTreasureLabelPrefixes mirrors LocalTreasuresHandler.js's
// EXCLUDED_LABEL_PREFIXES: a SPECIAL_EVENT_* entry shares its entity id with a real NewMob
// (pcap-confirmed 2026-07-30) - drawing it here would duplicate an encounter already shown,
// with better threat info, by mob detection. ANNIVERSARY has no matching mob id and stays.
var excludedLocalTreasureLabelPrefixes = []string{"SPECIAL_EVENT"}

func isDrawableLocalTreasureLabel(label string) bool {
	for _, prefix := range excludedLocalTreasureLabelPrefixes {
		if strings.HasPrefix(label, prefix) {
			return false
		}
	}
	return true
}

// LocalTreasuresState is a Go port of LocalTreasuresHandler.js.
type LocalTreasuresState struct {
	treasures *entityList[int, LocalTreasure]
}

func NewLocalTreasuresState() *LocalTreasuresState {
	return &LocalTreasuresState{treasures: newEntityList[int, LocalTreasure]()}
}

// HandleLocalTreasuresUpdate ports LocalTreasuresHandler.js's handleLocalTreasuresUpdate.
// Unlike every other detection type in this codebase, LocalTreasuresUpdate delivers a full
// resync of every active local treasure in the zone as parallel arrays (ids=[4],
// positions=[5] flattened [x0,y0,x1,y1,...], startTicks=[6], endTicks=[7], labels=[8]) rather
// than one event per entity. Removal still arrives individually via the normal Leave event.
func (s *LocalTreasuresState) HandleLocalTreasuresUpdate(p Params) {
	ids := paramIntSlice(p, 4)
	positions := paramFloat32Slice(p, 5)
	startTicks := paramIntSlice(p, 6)
	endTicks := paramIntSlice(p, 7)
	labels := paramStringSlice(p, 8)

	for i, id := range ids {
		if i >= len(labels) {
			break
		}
		label := labels[i]
		if !isDrawableLocalTreasureLabel(label) {
			continue
		}
		if i*2+1 >= len(positions) {
			continue
		}
		posX, posY := positions[i*2], positions[i*2+1]

		var start, end int
		if i < len(startTicks) {
			start = startTicks[i]
		}
		if i < len(endTicks) {
			end = endTicks[i]
		}

		s.treasures.upsert(id, func() LocalTreasure {
			return LocalTreasure{ID: id, PosX: posX, PosY: posY, Label: label, StartTicks: start, EndTicks: end}
		})
	}
}

func (s *LocalTreasuresState) Remove(id int) { s.treasures.remove(id) }
func (s *LocalTreasuresState) Clear()        { s.treasures.clear() }
func (s *LocalTreasuresState) Snapshot() []LocalTreasure {
	return s.treasures.snapshot()
}
func (s *LocalTreasuresState) Len() int { return s.treasures.len() }
func (s *LocalTreasuresState) CleanupStale(maxAgeMs int) int {
	return s.treasures.cleanupStale(msToDuration(maxAgeMs))
}
