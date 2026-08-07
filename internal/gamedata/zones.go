package gamedata

import (
	"io/fs"
	"strings"
)

// Bounds mirrors a zone's map-asset bounding box (world units).
type Bounds struct {
	Min [2]float64 `json:"min"`
	Max [2]float64 `json:"max"`
}

// Zone mirrors one entry of zones.json, keyed by zone ID string.
type Zone struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	PvpType string `json:"pvpType"`
	Tier    int    `json:"tier"`
	File    string `json:"file"`
	Bounds  Bounds `json:"bounds"`
}

// ZonesDatabase looks up zone metadata (PvP type, map tile file, bounds) by zone ID, mirroring
// web/scripts/data/ZonesDatabase.js. Mist zones are not present in zones.json - their PvP type
// is derived at runtime from the origin zone they were entered from (SetMistOverride), same as
// the JS version; that override state belongs to internal/radarstate (session state), not here.
type ZonesDatabase struct {
	zones map[string]Zone
}

// LoadZones reads zones.json(.gz) from fsys.
func LoadZones(fsys fs.FS, filename string) (*ZonesDatabase, error) {
	var raw map[string]Zone
	if err := readJSON(fsys, filename, &raw); err != nil {
		return nil, err
	}
	return &ZonesDatabase{zones: raw}, nil
}

// GetZone looks up a zone by ID, falling back to the part before a "-" for compound IDs like
// "1234-5", and applying the Roads of Avalon full-loot-PvP override (zones.json tags
// TUNNEL_ROYAL/TUNNEL_ROYAL_RED as safe/red; both are actually always black) - same two rules
// ZonesDatabase.js's getZone/_applyAvalonRoadsRule apply.
func (d *ZonesDatabase) GetZone(zoneID string) (Zone, bool) {
	zone, ok := d.zones[zoneID]
	if !ok {
		if base, _, found := strings.Cut(zoneID, "-"); found {
			zone, ok = d.zones[base]
		}
	}
	if !ok {
		return Zone{}, false
	}
	if zone.Type == "TUNNEL_ROYAL" || zone.Type == "TUNNEL_ROYAL_RED" {
		zone.PvpType = "black"
	}
	return zone, true
}

// PvpType returns the zone's PvP type, defaulting to "safe" for an unknown zone (matching
// ZonesDatabase.js's getPvpType).
func (d *ZonesDatabase) PvpType(zoneID string) string {
	if zone, ok := d.GetZone(zoneID); ok {
		return zone.PvpType
	}
	return "safe"
}

// Len reports how many zones are loaded.
func (d *ZonesDatabase) Len() int {
	return len(d.zones)
}
