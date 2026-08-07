package radarstate

import (
	"testing"
	"time"

	"github.com/nospy/albion-openradar/internal/gamedata"
	"github.com/nospy/albion-openradar/internal/roads"
)

func TestZoneGraph_GetNextHop_BeforeLoadedReturnsNil(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	if got := g.GetNextHop("A", "B"); got != nil {
		t.Errorf("GetNextHop before load = %+v, want nil", got)
	}
}

func TestZoneGraph_GetNextHop_SameZoneIsZeroHops(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges([]gamedata.StaticEdge{{From: "A", To: "B"}}, nil)

	got := g.GetNextHop("A", "A")
	if got == nil || got.NextZoneID != "A" || got.Hops != 0 || got.ViaPos != nil {
		t.Errorf("GetNextHop(A,A) = %+v", got)
	}
}

func TestZoneGraph_GetNextHop_DirectSingleHop(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	pos := &[2]float64{10, 20}
	g.LoadFromEdges([]gamedata.StaticEdge{{From: "A", To: "B", Pos: pos}}, nil)

	got := g.GetNextHop("A", "B")
	if got == nil || got.NextZoneID != "B" || got.Hops != 1 || got.ViaPos == nil || got.ViaPos[0] != 10 {
		t.Errorf("GetNextHop(A,B) = %+v", got)
	}
}

func TestZoneGraph_GetNextHop_MultiHopReturnsFirstHopOnly(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges([]gamedata.StaticEdge{
		{From: "A", To: "B"}, {From: "B", To: "C"}, {From: "C", To: "D"},
	}, nil)

	got := g.GetNextHop("A", "D")
	if got == nil || got.NextZoneID != "B" || got.Hops != 3 || got.Stale || got.Assumed {
		t.Errorf("GetNextHop(A,D) = %+v, want NextZoneID=B Hops=3", got)
	}
}

func TestZoneGraph_GetNextHop_NoPathReturnsNil(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges([]gamedata.StaticEdge{{From: "A", To: "B"}}, nil)

	if got := g.GetNextHop("A", "Z"); got != nil {
		t.Errorf("GetNextHop(A,Z) = %+v, want nil (no path)", got)
	}
}

func TestZoneGraph_DiscoveredEdgeShortensPath(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges(
		[]gamedata.StaticEdge{{From: "A", To: "B"}, {From: "B", To: "C"}, {From: "C", To: "D"}},
		[]roads.Edge{{From: "A", To: "D", Pos: &[2]float64{9, 9}, DiscoveredAt: time.Now()}},
	)

	got := g.GetNextHop("A", "D")
	if got == nil || got.NextZoneID != "D" || got.Hops != 1 {
		t.Errorf("GetNextHop(A,D) = %+v, want the direct discovered shortcut", got)
	}
}

// @verified: mirrors ZoneGraph.js's "assumed reverse" mechanism - a discovered A->D edge
// implies you can probably head back D->A too, even though that direction was never itself
// observed.
func TestZoneGraph_AssumedReverseAllowsBacktrackingWhenNoOtherPathExists(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges(nil, []roads.Edge{{From: "A", To: "D", DiscoveredAt: time.Now()}})

	got := g.GetNextHop("D", "A")
	if got == nil || !got.Assumed {
		t.Errorf("GetNextHop(D,A) = %+v, want an assumed-reverse path", got)
	}
}

func TestZoneGraph_ReliablePathPreferredOverAssumedShortcut(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges(
		[]gamedata.StaticEdge{{From: "D", To: "C"}, {From: "C", To: "B"}, {From: "B", To: "A"}},
		[]roads.Edge{{From: "A", To: "D", DiscoveredAt: time.Now()}}, // implies an assumed D->A shortcut
	)

	got := g.GetNextHop("D", "A")
	if got == nil || got.Assumed || got.Hops != 3 {
		t.Errorf("GetNextHop(D,A) = %+v, want the reliable 3-hop static path, not the assumed shortcut", got)
	}
}

func TestZoneGraph_StaleDiscoveredEdgeOnlyUsedAsFallback(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	old := time.Now().Add(-4 * 24 * time.Hour) // older than the 3-day staleAfter window
	// No reliable path exists at all (no static graph) - only the stale discovered shortcut.
	g.LoadFromEdges(nil, []roads.Edge{{From: "A", To: "C", DiscoveredAt: old}})

	got := g.GetNextHop("A", "C")
	if got == nil || got.Hops != 1 || !got.Stale {
		t.Errorf("GetNextHop(A,C) = %+v, want the stale 1-hop shortcut flagged Stale=true (no reliable path exists)", got)
	}
}

func TestZoneGraph_ReliableStaticPathPreferredOverStaleShortcut(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	old := time.Now().Add(-4 * 24 * time.Hour)
	g.LoadFromEdges(
		[]gamedata.StaticEdge{{From: "A", To: "B"}, {From: "B", To: "C"}},
		[]roads.Edge{{From: "A", To: "C", DiscoveredAt: old}},
	)

	got := g.GetNextHop("A", "C")
	if got == nil || got.Stale || got.Hops != 2 {
		t.Errorf("GetNextHop(A,C) = %+v, want the reliable 2-hop static path, not the stale shortcut", got)
	}
}

func TestZoneGraph_GetFullPath(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges([]gamedata.StaticEdge{{From: "A", To: "B"}, {From: "B", To: "C"}}, nil)

	got := g.GetFullPath("A", "C")
	want := []string{"A", "B", "C"}
	if got == nil || len(got.Path) != len(want) {
		t.Fatalf("GetFullPath(A,C) = %+v", got)
	}
	for i, z := range want {
		if got.Path[i] != z {
			t.Errorf("Path[%d] = %q, want %q", i, got.Path[i], z)
		}
	}
}

func TestZoneGraph_ReportTransition_RecordsNovelEdgeAndPersists(t *testing.T) {
	dir := t.TempDir()
	g := NewZoneGraph(dir)
	g.LoadFromEdges([]gamedata.StaticEdge{{From: "X", To: "Y"}}, nil) // unrelated static edge, just to mark loaded

	g.ReportTransition("A", "B", &[2]float64{1, 2})

	if !g.HasEdge("A", "B") {
		t.Error("ReportTransition should record the new edge in-memory")
	}
	if !g.HasEdge("B", "A") {
		t.Error("ReportTransition should also record an assumed reverse")
	}

	store, err := roads.ReadStore(dir)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if len(store.Edges) != 1 || store.Edges[0].From != "A" || store.Edges[0].To != "B" {
		t.Errorf("persisted store = %+v, want a single A->B edge", store.Edges)
	}
}

func TestZoneGraph_ReportTransition_AlreadyKnownEdgeIsNoOp(t *testing.T) {
	dir := t.TempDir()
	g := NewZoneGraph(dir)
	g.LoadFromEdges([]gamedata.StaticEdge{{From: "A", To: "B"}}, nil)

	g.ReportTransition("A", "B", nil) // already explainable by the static graph

	store, err := roads.ReadStore(dir)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if len(store.Edges) != 0 {
		t.Errorf("persisted store = %+v, want empty (already-known edge should not be recorded)", store.Edges)
	}
}

func TestZoneGraph_ReportTransition_NoopBeforeLoaded(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.ReportTransition("A", "B", nil)

	if g.HasEdge("A", "B") {
		t.Error("ReportTransition should be a no-op before Load - adjacency is empty so nothing looks known")
	}
}

func TestZoneGraph_RemoveEdge(t *testing.T) {
	dir := t.TempDir()
	g := NewZoneGraph(dir)
	g.LoadFromEdges(nil, []roads.Edge{{From: "A", To: "B", DiscoveredAt: time.Now()}})

	removed := g.RemoveEdge("A", "B")
	if !removed || g.HasEdge("A", "B") {
		t.Error("RemoveEdge should remove the discovered edge")
	}

	store, err := roads.ReadStore(dir)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if len(store.Edges) != 0 {
		t.Errorf("persisted store = %+v, want empty after RemoveEdge", store.Edges)
	}
}

func TestZoneGraph_RemoveEdge_StaticEdgeCannotBeRemoved(t *testing.T) {
	g := NewZoneGraph(t.TempDir())
	g.LoadFromEdges([]gamedata.StaticEdge{{From: "A", To: "B"}}, nil)

	removed := g.RemoveEdge("A", "B")

	if removed {
		t.Error("RemoveEdge should refuse to remove a static edge")
	}
	if !g.HasEdge("A", "B") {
		t.Error("the static edge should still be present")
	}
}
