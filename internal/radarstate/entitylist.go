package radarstate

import (
	"sort"
	"sync"
	"time"
)

// entityList is a small thread-safe generic store shared by every simple entity-list handler,
// replacing the copy-pasted add/touch/remove/Clear/cleanupStaleEntities methods that appear
// nearly identically in ChestsHandler.js, WispCageHandler.js, MistsDungeonHandler.js,
// LocalTreasuresHandler.js, FishingHandler.js and DungeonsHandler.js. K is the entity's id
// type, V its record type (which must embed a lastUpdateTime the caller can update via touch).
type entityList[K comparable, V any] struct {
	mu       sync.RWMutex
	items    map[K]V
	lastSeen map[K]time.Time
}

func newEntityList[K comparable, V any]() *entityList[K, V] {
	return &entityList[K, V]{
		items:    make(map[K]V),
		lastSeen: make(map[K]time.Time),
	}
}

// upsert adds a new entity or, if id already exists, touches it (refreshes lastSeen) without
// overwriting its fields - mirrors every handler's `if (existing) { existing.touch(); return }`
// early-return-on-duplicate behavior.
func (l *entityList[K, V]) upsert(id K, create func() V) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.items[id]; ok {
		l.lastSeen[id] = time.Now()
		return
	}
	l.items[id] = create()
	l.lastSeen[id] = time.Now()
}

// replace adds a new entity, or overwrites an existing one's fields entirely (for handlers
// like FishingHandler.js's upsertFish, which updates size/position on every re-observation
// rather than only touching the timestamp).
func (l *entityList[K, V]) replace(id K, v V) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items[id] = v
	l.lastSeen[id] = time.Now()
}

// mutate applies fn to a copy of the entity at id (if present), then writes the result back
// and refreshes lastSeen - used by handlers that update specific fields in place (health,
// mounted state, faction) rather than only touching or fully replacing. Reports whether id was
// found.
func (l *entityList[K, V]) mutate(id K, fn func(v *V)) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.items[id]
	if !ok {
		return false
	}
	fn(&v)
	l.items[id] = v
	l.lastSeen[id] = time.Now()
	return true
}

func (l *entityList[K, V]) remove(id K) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.items, id)
	delete(l.lastSeen, id)
}

func (l *entityList[K, V]) get(id K) (V, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	v, ok := l.items[id]
	return v, ok
}

func (l *entityList[K, V]) clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = make(map[K]V)
	l.lastSeen = make(map[K]time.Time)
}

// snapshot returns a copy of every current entity, safe for the render loop to iterate without
// holding the lock (mirrors reading RadarRenderer.js reads straight off each handler's array
// every frame).
func (l *entityList[K, V]) snapshot() []V {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]V, 0, len(l.items))
	for _, v := range l.items {
		out = append(out, v)
	}
	return out
}

// cleanupStale removes entities not seen in maxAge, mirroring every handler's
// cleanupStaleEntities(maxAgeMs). Returns the number removed.
func (l *entityList[K, V]) cleanupStale(maxAge time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	removed := 0
	for id, seen := range l.lastSeen {
		if now.Sub(seen) >= maxAge {
			delete(l.items, id)
			delete(l.lastSeen, id)
			removed++
		}
	}
	return removed
}

func (l *entityList[K, V]) len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}

// enforceMaxSize keeps only the maxSize most-recently-seen entries, removing the rest -
// mirrors every handler's enforceMaxSize(maxSize): sort by lastUpdateTime descending, keep the
// newest. Returns the number removed.
func (l *entityList[K, V]) enforceMaxSize(maxSize int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.items) <= maxSize {
		return 0
	}
	type idAge struct {
		id  K
		age int64
	}
	ordered := make([]idAge, 0, len(l.items))
	for id, seen := range l.lastSeen {
		ordered = append(ordered, idAge{id, seen.UnixNano()})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].age > ordered[j].age })
	removed := 0
	for _, entry := range ordered[maxSize:] {
		delete(l.items, entry.id)
		delete(l.lastSeen, entry.id)
		removed++
	}
	return removed
}

// msToDuration converts a millisecond staleness threshold (matching every JS handler's
// maxAgeMs parameter) to a time.Duration.
func msToDuration(ms int) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
