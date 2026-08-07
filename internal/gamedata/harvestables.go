package gamedata

import (
	"io/fs"
	"sort"
)

// HarvestableTierEntry mirrors one entry of harvestables.json's per-resource-type array
// (web/scripts/data/HarvestablesDatabase.js's tierData).
type HarvestableTierEntry struct {
	Tier         int
	Item         string
	Respawn      int
	Harvest      int
	Tool         bool
	MaxCharges   int
	StartCharges int
	ChargeUp     float64
}

// HarvestablesDatabase validates resource-type/tier/enchant combinations, mirroring
// web/scripts/data/HarvestablesDatabase.js.
type HarvestablesDatabase struct {
	validCombos map[[3]int]struct{} // keyed by [typeNumber-bucket-agnostic: resourceIndex, tier, enchant]
	tiersByType map[string]map[int]struct{}
}

var resourceTypeOrder = []string{"WOOD", "ROCK", "FIBER", "HIDE", "ORE"}

// resourceTypeIndex returns resourceTypeOrder's index for a resource type, or -1 if unknown -
// used only as an internal composite-key component, not part of the public API.
func resourceTypeIndex(resourceType string) int {
	for i, t := range resourceTypeOrder {
		if t == resourceType {
			return i
		}
	}
	return -1
}

// LoadHarvestables reads harvestables.min.json(.gz) from fsys. The JSON shape is
// { "WOOD": [{tier, item, respawn, ...}, ...], "ROCK": [...], ... }.
func LoadHarvestables(fsys fs.FS, filename string) (*HarvestablesDatabase, error) {
	var raw map[string][]HarvestableTierEntry
	if err := readJSON(fsys, filename, &raw); err != nil {
		return nil, err
	}

	db := &HarvestablesDatabase{
		validCombos: make(map[[3]int]struct{}),
		tiersByType: make(map[string]map[int]struct{}),
	}
	for resourceType, entries := range raw {
		idx := resourceTypeIndex(resourceType)
		tiers := make(map[int]struct{})
		for _, e := range entries {
			tiers[e.Tier] = struct{}{}
		}
		db.tiersByType[resourceType] = tiers

		if idx == -1 {
			continue // unknown resource type - keep tier data but nothing typeNumber-related can reference it
		}
		for tier := range tiers {
			for enchant := range 5 {
				db.validCombos[[3]int{idx, tier, enchant}] = struct{}{}
			}
		}
	}
	return db, nil
}

// IsValidResource reports whether resourceType/tier/enchant is a real combination.
func (d *HarvestablesDatabase) IsValidResource(resourceType string, tier, enchant int) bool {
	idx := resourceTypeIndex(resourceType)
	if idx == -1 {
		return false
	}
	_, ok := d.validCombos[[3]int{idx, tier, enchant}]
	return ok
}

// ResourceTypeFromTypeNumber mirrors HarvestablesDatabase.js's getResourceTypeFromTypeNumber:
// the wire protocol identifies a harvestable's resource by a 0-27 typeNumber, banded by type.
func ResourceTypeFromTypeNumber(typeNumber int) string {
	switch {
	case typeNumber >= 0 && typeNumber <= 5:
		return "WOOD"
	case typeNumber >= 6 && typeNumber <= 10:
		return "ROCK"
	case typeNumber >= 11 && typeNumber <= 15:
		return "FIBER"
	case typeNumber >= 16 && typeNumber <= 22:
		return "HIDE"
	case typeNumber >= 23 && typeNumber <= 27:
		return "ORE"
	default:
		return ""
	}
}

// IsValidResourceByTypeNumber mirrors HarvestablesDatabase.js's isValidResourceByTypeNumber.
func (d *HarvestablesDatabase) IsValidResourceByTypeNumber(typeNumber, tier, enchant int) bool {
	resourceType := ResourceTypeFromTypeNumber(typeNumber)
	if resourceType == "" {
		return false
	}
	return d.IsValidResource(resourceType, tier, enchant)
}

// ValidTiers returns the sorted list of tiers a resource type has data for.
func (d *HarvestablesDatabase) ValidTiers(resourceType string) []int {
	tiers := d.tiersByType[resourceType]
	if len(tiers) == 0 {
		return nil
	}
	out := make([]int, 0, len(tiers))
	for t := range tiers {
		out = append(out, t)
	}
	sort.Ints(out)
	return out
}
