package hub

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_ListEmpty(t *testing.T) {
	s := newTestStore(t)
	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %+v", edges)
	}
}

func TestStore_UpsertThenList(t *testing.T) {
	s := newTestStore(t)
	pos := &[2]float64{1.5, -2.5}
	if err := s.UpsertEdge("4206", "TNL-001", pos); err != nil {
		t.Fatalf("UpsertEdge: %v", err)
	}

	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %+v", edges)
	}
	e := edges[0]
	if e.From != "4206" || e.To != "TNL-001" {
		t.Errorf("edge fields wrong: %+v", e)
	}
	if e.Pos == nil || e.Pos[0] != 1.5 || e.Pos[1] != -2.5 {
		t.Errorf("pos wrong: %+v", e.Pos)
	}
	if e.DiscoveredAt.IsZero() {
		t.Error("discoveredAt not set")
	}
}

func TestStore_UpsertWithoutPos(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertEdge("A", "B", nil); err != nil {
		t.Fatalf("UpsertEdge: %v", err)
	}
	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 1 || edges[0].Pos != nil {
		t.Errorf("expected 1 edge with nil pos, got %+v", edges)
	}
}

func TestStore_UpsertRefreshesExistingEdge(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertEdge("A", "B", nil); err != nil {
		t.Fatalf("UpsertEdge #1: %v", err)
	}
	pos := &[2]float64{9, 9}
	if err := s.UpsertEdge("A", "B", pos); err != nil {
		t.Fatalf("UpsertEdge #2: %v", err)
	}

	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected upsert to keep a single edge, got %+v", edges)
	}
	if edges[0].Pos == nil || edges[0].Pos[0] != 9 {
		t.Errorf("expected refreshed pos, got %+v", edges[0].Pos)
	}
}

func TestStore_DistinctEdgesCoexist(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertEdge("A", "B", nil); err != nil {
		t.Fatalf("UpsertEdge A->B: %v", err)
	}
	if err := s.UpsertEdge("B", "A", nil); err != nil {
		t.Fatalf("UpsertEdge B->A: %v", err)
	}
	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 distinct directed edges, got %+v", edges)
	}
}

func TestStore_DeleteEdgeRemovesMatchingEdge(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertEdge("A", "B", nil); err != nil {
		t.Fatalf("UpsertEdge: %v", err)
	}
	if err := s.UpsertEdge("A", "C", nil); err != nil {
		t.Fatalf("UpsertEdge: %v", err)
	}

	if err := s.DeleteEdge("A", "B"); err != nil {
		t.Fatalf("DeleteEdge: %v", err)
	}

	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 1 || edges[0].To != "C" {
		t.Fatalf("expected only A->C to remain, got %+v", edges)
	}
}

func TestStore_DeleteEdgeMissingEdgeIsNoOp(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertEdge("A", "B", nil); err != nil {
		t.Fatalf("UpsertEdge: %v", err)
	}

	if err := s.DeleteEdge("X", "Y"); err != nil {
		t.Fatalf("DeleteEdge should not error on a nonexistent edge: %v", err)
	}

	edges, err := s.ListEdges()
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("existing edge should be untouched, got %+v", edges)
	}
}
