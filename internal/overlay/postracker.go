package overlay

// idPos is one entity's id and current raw game-world position, the minimal input sync()
// needs regardless of which radarstate type (Chest, Mob, Fish, ...) it came from.
type idPos struct {
	id   int
	x, y float32
}

// posTracker maintains per-entity interpolated screen-relative positions (EntityPos.HX/HY)
// across frames, keyed by entity id - shared by every entity type the overlay draws (mobs,
// harvestables, chests, dungeons, ...) rather than each hand-rolling the same
// create-if-missing/interpolate/prune-stale-ids loop.
type posTracker struct {
	positions map[int]*EntityPos
}

func newPosTracker() *posTracker {
	return &posTracker{positions: make(map[int]*EntityPos)}
}

// sync updates every entry's interpolated position toward entries' current raw positions, and
// drops any previously-tracked id no longer present (the entity left/expired in radarstate).
func (t *posTracker) sync(entries []idPos, lpX, lpY, lerpT float64) {
	live := make(map[int]struct{}, len(entries))
	for _, e := range entries {
		live[e.id] = struct{}{}
		pos := t.positions[e.id]
		if pos == nil {
			pos = &EntityPos{}
			t.positions[e.id] = pos
		}
		pos.PosX, pos.PosY = float64(e.x), float64(e.y)
		InterpolateEntity(pos, lpX, lpY, lerpT)
	}
	for id := range t.positions {
		if _, ok := live[id]; !ok {
			delete(t.positions, id)
		}
	}
}

func (t *posTracker) get(id int) *EntityPos {
	return t.positions[id]
}
