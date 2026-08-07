package gamedata

import "io/fs"

// StaticEdge mirrors one entry of zone-graph.json's "edges" array - the fixed open-world zone
// adjacency graph (Roads of Avalon and other runtime-discovered connections are layered on top
// separately, see internal/roads).
type StaticEdge struct {
	From string      `json:"from"`
	To   string      `json:"to"`
	Pos  *[2]float64 `json:"pos"`
}

type zoneGraphFile struct {
	Edges []StaticEdge `json:"edges"`
}

// LoadZoneGraph reads zone-graph.json(.gz) from fsys.
func LoadZoneGraph(fsys fs.FS, filename string) ([]StaticEdge, error) {
	var raw zoneGraphFile
	if err := readJSON(fsys, filename, &raw); err != nil {
		return nil, err
	}
	return raw.Edges, nil
}
