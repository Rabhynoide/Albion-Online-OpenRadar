package radarstate

import (
	"sync"
	"time"

	"github.com/nospy/albion-openradar/internal/gamedata"
	"github.com/nospy/albion-openradar/internal/roads"
)

// staleAfter mirrors ZoneGraph.js's STALE_MS: Roads of Avalon reset well under a week
// in-game; 3 days balances "probably still usable" against "stale enough to mislead".
const staleAfter = 3 * 24 * time.Hour

type edgeSource int

const (
	edgeStatic edgeSource = iota
	edgeDiscovered
	edgeAssumed
)

type graphEdge struct {
	to           string
	pos          *[2]float64
	source       edgeSource
	discoveredAt time.Time
}

// NextHop mirrors ZoneGraph.js's getNextHop result shape.
type NextHop struct {
	NextZoneID string
	ViaPos     *[2]float64
	Hops       int
	Stale      bool
	Assumed    bool
}

// FullPath mirrors ZoneGraph.js's getFullPath result shape.
type FullPath struct {
	Path    []string
	Stale   bool
	Assumed bool
}

// ZoneGraph is a Go port of web/scripts/data/ZoneGraph.js: unweighted BFS pathfinding over a
// static open-world zone adjacency graph (zone-graph.json) layered with runtime-discovered
// connections (Roads of Avalon and similar - see internal/roads, whose Store this reads/writes
// directly in-process rather than through the web client's /api/roads/edges HTTP relay).
type ZoneGraph struct {
	mu        sync.RWMutex
	adjacency map[string][]graphEdge
	loaded    bool
	appDir    string
}

func NewZoneGraph(appDir string) *ZoneGraph {
	return &ZoneGraph{appDir: appDir, adjacency: make(map[string][]graphEdge)}
}

// LoadFromEdges mirrors ZoneGraph.js's loadFromEdges. Callers load the static graph via
// gamedata.LoadZoneGraph and the discovered-edges store via roads.ReadStore(appDir), then pass
// both here - kept as two separate steps (rather than ZoneGraph owning file I/O itself) so this
// type stays easy to unit test, same reasoning as the JS version's own testable seam.
func (g *ZoneGraph) LoadFromEdges(staticEdges []gamedata.StaticEdge, discoveredEdges []roads.Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.adjacency = make(map[string][]graphEdge)
	for _, e := range staticEdges {
		g.addEdgeLocked(e.From, e.To, e.Pos, edgeStatic, time.Time{})
	}
	for _, e := range discoveredEdges {
		g.addEdgeLocked(e.From, e.To, e.Pos, edgeDiscovered, e.DiscoveredAt)
	}
	// Second pass, after every real observed direction is in: assume each discovered edge is
	// reversible unless that reverse was itself separately observed (which already won above).
	for _, e := range discoveredEdges {
		g.addAssumedReverseLocked(e.To, e.From, e.DiscoveredAt)
	}
	g.loaded = true
}

func (g *ZoneGraph) addEdgeLocked(from, to string, pos *[2]float64, source edgeSource, discoveredAt time.Time) {
	if from == "" || to == "" {
		return
	}
	edges := g.adjacency[from]
	for i := range edges {
		if edges[i].to == to {
			edges[i].pos = pos
			edges[i].source = source
			edges[i].discoveredAt = discoveredAt
			return
		}
	}
	g.adjacency[from] = append(edges, graphEdge{to: to, pos: pos, source: source, discoveredAt: discoveredAt})
}

// addAssumedReverseLocked mirrors _addAssumedReverse: the exit position for the reverse
// direction lives in the *other* zone's local coordinates, which ReportTransition never
// observes (only the pre-transition position, in the origin zone, is known) - so the assumed
// reverse carries no viaPos, only "which zone to head back to".
func (g *ZoneGraph) addAssumedReverseLocked(from, to string, discoveredAt time.Time) {
	if from == "" || to == "" || from == to {
		return
	}
	if g.hasEdgeLocked(from, to) {
		return // never downgrade an already-known (real) edge
	}
	g.addEdgeLocked(from, to, nil, edgeAssumed, discoveredAt)
}

func (g *ZoneGraph) hasEdgeLocked(from, to string) bool {
	for _, e := range g.adjacency[from] {
		if e.to == to {
			return true
		}
	}
	return false
}

// HasEdge mirrors hasEdge.
func (g *ZoneGraph) HasEdge(from, to string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.hasEdgeLocked(from, to)
}

// isStale mirrors isStale: a real observation old enough that the road may have reset since.
func isStale(e graphEdge) bool {
	if e.source != edgeDiscovered || e.discoveredAt.IsZero() {
		return false
	}
	return time.Since(e.discoveredAt) > staleAfter
}

func isAssumed(e graphEdge) bool { return e.source == edgeAssumed }

func isUnreliable(e graphEdge) bool { return isAssumed(e) || isStale(e) }

type pathResult struct {
	path        []string
	usedStale   bool
	usedAssumed bool
}

// shortestPath mirrors _shortestPath: unweighted BFS, tries reliable-only edges first, falling
// back to also allowing stale/assumed edges only when no fully-reliable path exists.
func (g *ZoneGraph) shortestPath(fromZoneID, toZoneID string, includeUnreliable bool) *pathResult {
	type queueEntry struct {
		id                     string
		path                   []string
		usedStale, usedAssumed bool
	}
	visited := map[string]struct{}{fromZoneID: {}}
	queue := []queueEntry{{id: fromZoneID, path: []string{fromZoneID}}}

	for head := 0; head < len(queue); head++ {
		entry := queue[head]
		for _, edge := range g.adjacency[entry.id] {
			if _, seen := visited[edge.to]; seen {
				continue
			}
			if isUnreliable(edge) && !includeUnreliable {
				continue
			}

			nextPath := append(append([]string{}, entry.path...), edge.to)
			nextUsedStale := entry.usedStale || isStale(edge)
			nextUsedAssumed := entry.usedAssumed || isAssumed(edge)
			if edge.to == toZoneID {
				return &pathResult{path: nextPath, usedStale: nextUsedStale, usedAssumed: nextUsedAssumed}
			}
			visited[edge.to] = struct{}{}
			queue = append(queue, queueEntry{id: edge.to, path: nextPath, usedStale: nextUsedStale, usedAssumed: nextUsedAssumed})
		}
	}
	return nil
}

// GetNextHop mirrors getNextHop: returns only the next hop (zone + exit position), not the
// full route.
func (g *ZoneGraph) GetNextHop(fromZoneID, toZoneID string) *NextHop {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.loaded || fromZoneID == "" || toZoneID == "" {
		return nil
	}
	if fromZoneID == toZoneID {
		return &NextHop{NextZoneID: fromZoneID, Hops: 0}
	}

	result := g.shortestPath(fromZoneID, toZoneID, false)
	if result == nil {
		result = g.shortestPath(fromZoneID, toZoneID, true)
	}
	if result == nil {
		return nil
	}

	nextZoneID := result.path[1]
	var viaPos *[2]float64
	for _, e := range g.adjacency[fromZoneID] {
		if e.to == nextZoneID {
			viaPos = e.pos
			break
		}
	}
	return &NextHop{
		NextZoneID: nextZoneID, ViaPos: viaPos, Hops: len(result.path) - 1,
		Stale: result.usedStale, Assumed: result.usedAssumed,
	}
}

// GetFullPath mirrors getFullPath: the full zone-by-zone route (inclusive of both ends).
func (g *ZoneGraph) GetFullPath(fromZoneID, toZoneID string) *FullPath {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.loaded || fromZoneID == "" || toZoneID == "" {
		return nil
	}
	if fromZoneID == toZoneID {
		return &FullPath{Path: []string{fromZoneID}}
	}

	result := g.shortestPath(fromZoneID, toZoneID, false)
	if result == nil {
		result = g.shortestPath(fromZoneID, toZoneID, true)
	}
	if result == nil {
		return nil
	}
	return &FullPath{Path: result.path, Stale: result.usedStale, Assumed: result.usedAssumed}
}

// RemoveEdge mirrors removeEdge (issue #5: a Road of Avalon reset to a different connection,
// and the staleness window hasn't caught up yet). Static edges are left alone. Persists the
// removal to internal/roads' store; a persistence failure is logged-and-ignored, never allowed
// to break the in-memory graph the player is actively navigating by.
func (g *ZoneGraph) RemoveEdge(from, to string) bool {
	g.mu.Lock()
	removed := g.removeEdgeLocked(from, to)
	g.mu.Unlock()

	if removed {
		_ = roads.MutateStore(g.appDir, func(s *roads.Store) {
			roads.RemoveEdge(s, from, to)
		})
	}
	return removed
}

func (g *ZoneGraph) removeEdgeLocked(from, to string) bool {
	edges := g.adjacency[from]
	idx := -1
	for i, e := range edges {
		if e.to == to && e.source != edgeStatic {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false
	}
	g.adjacency[from] = append(edges[:idx], edges[idx+1:]...)

	reverse := g.adjacency[to]
	for i, e := range reverse {
		if e.to == from && e.source == edgeAssumed {
			g.adjacency[to] = append(reverse[:i], reverse[i+1:]...)
			break
		}
	}
	return true
}

// ReportTransition mirrors reportTransition: called on every zone transition. Only real
// cluster-to-cluster transitions that aren't already explainable by the static or
// previously-discovered graph get recorded - that's exactly the "must be an Avalon Road"
// signal, since static adjacency already covers ordinary open-world exits.
func (g *ZoneGraph) ReportTransition(fromZoneID, toZoneID string, pos *[2]float64) {
	g.mu.Lock()
	if !g.loaded || fromZoneID == "" || toZoneID == "" || fromZoneID == toZoneID || g.hasEdgeLocked(fromZoneID, toZoneID) {
		g.mu.Unlock()
		return
	}
	discoveredAt := time.Now()
	g.addEdgeLocked(fromZoneID, toZoneID, pos, edgeDiscovered, discoveredAt)
	g.addAssumedReverseLocked(toZoneID, fromZoneID, discoveredAt)
	g.mu.Unlock()

	_ = roads.MutateStore(g.appDir, func(s *roads.Store) {
		roads.AddEdge(s, fromZoneID, toZoneID, pos)
	})
}
