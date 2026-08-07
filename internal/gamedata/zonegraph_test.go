package gamedata

import (
	"testing"
	"testing/fstest"
)

func TestLoadZoneGraph(t *testing.T) {
	fsys := fstest.MapFS{
		"zone-graph.json": mapFile(`{"edges":[{"from":"0004","to":"4210","pos":[120.5,192.5]},{"from":"0004","to":"0205","pos":null}]}`),
	}
	edges, err := LoadZoneGraph(fsys, "zone-graph.json")
	if err != nil {
		t.Fatalf("LoadZoneGraph: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("len = %d, want 2", len(edges))
	}
	if edges[0].From != "0004" || edges[0].To != "4210" || edges[0].Pos == nil || edges[0].Pos[0] != 120.5 {
		t.Errorf("edges[0] = %+v", edges[0])
	}
	if edges[1].Pos != nil {
		t.Errorf("edges[1].Pos = %v, want nil", edges[1].Pos)
	}
}
