package radarstate

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/nospy/albion-openradar/internal/photon"
)

func float32LEBuffer(x, y float32) photon.ByteArray {
	buf := make(photon.ByteArray, 8)
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(x))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(y))
	return buf
}

func TestSession_HandleMoveRequest_DirectArray(t *testing.T) {
	s := NewSession()
	s.HandleMoveRequest(Params{1: []float32{10, 20}})

	if s.LocalX != 10 || s.LocalY != 20 {
		t.Errorf("LocalX=%v LocalY=%v, want 10,20", s.LocalX, s.LocalY)
	}
}

func TestSession_HandleMoveRequest_ByteBuffer(t *testing.T) {
	s := NewSession()
	s.HandleMoveRequest(Params{1: float32LEBuffer(15.5, 25.5)})

	if s.LocalX != 15.5 || s.LocalY != 25.5 {
		t.Errorf("LocalX=%v LocalY=%v, want 15.5,25.5", s.LocalX, s.LocalY)
	}
}

func TestSession_HandleJoinPosition(t *testing.T) {
	s := NewSession()
	s.HandleJoinPosition(float32LEBuffer(1, 2))
	if s.LocalX != 1 || s.LocalY != 2 {
		t.Errorf("LocalX=%v LocalY=%v, want 1,2", s.LocalX, s.LocalY)
	}

	s2 := NewSession()
	s2.HandleJoinPosition([]interface{}{float32(3), float32(4)})
	if s2.LocalX != 3 || s2.LocalY != 4 {
		t.Errorf("LocalX=%v LocalY=%v, want 3,4", s2.LocalX, s2.LocalY)
	}
}

func TestSession_ChangeZone_ReportsTransitionToGraph(t *testing.T) {
	dir := t.TempDir()
	graph := NewZoneGraph(dir)
	graph.LoadFromEdges(nil, nil) // mark loaded, empty graph

	s := NewSession()
	s.SetLocalPosition(5, 6)
	s.ChangeZone("A", graph) // first zone: no previous, no report
	s.ChangeZone("B", graph) // A -> B is novel

	if !graph.HasEdge("A", "B") {
		t.Error("ChangeZone should report the A->B transition to the graph")
	}
}

func TestSession_ChangeZone_FirstZoneNeverReports(t *testing.T) {
	dir := t.TempDir()
	graph := NewZoneGraph(dir)
	graph.LoadFromEdges(nil, nil)

	s := NewSession()
	s.ChangeZone("A", graph)

	if graph.HasEdge("", "A") {
		t.Error("the very first zone (no previous) should never be reported as a transition")
	}
}
