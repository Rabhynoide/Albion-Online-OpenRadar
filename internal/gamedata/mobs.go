package gamedata

import (
	"io/fs"
	"strings"
)

// mobOffset is the server TypeID offset applied to the mobs.min.json array index - confirmed
// against a real capture (wire 116 -> idx 100), same anchor MobsDatabase.js uses. Re-verify
// with tools/offset-validate after each game-data refresh.
const mobOffset = 16

// Mob mirrors web/scripts/data/MobsDatabase.js's per-mob record.
type Mob struct {
	UniqueName    string
	CombatTier    int
	Category      string // "boss", "avatar", "critter", ...
	NameLocaTag   string
	LootType      string // raw Loot.Harvestable @type, e.g. "HIDE_CRITTER" ("" if none)
	IsHarvestable bool
	ResourceType  string // normalized: "Hide"/"Fiber"/"Log"/"Rock"/"Ore" ("" if not a resource)
	ResourceTier  int
}

type minifiedMob struct {
	U  string `json:"u"`
	T  int    `json:"t"`
	C  string `json:"c"`
	N  string `json:"n"`
	L  string `json:"l"`
	LT int    `json:"lt"`
}

// MobsDatabase looks mobs up by server TypeID (array index + mobOffset).
type MobsDatabase struct {
	byID   map[int]Mob
	byName map[string]int
}

// LoadMobs reads mobs.min.json(.gz) from fsys.
func LoadMobs(fsys fs.FS, filename string) (*MobsDatabase, error) {
	var raw []minifiedMob
	if err := readJSON(fsys, filename, &raw); err != nil {
		return nil, err
	}

	db := &MobsDatabase{
		byID:   make(map[int]Mob, len(raw)),
		byName: make(map[string]int, len(raw)),
	}
	for i, mm := range raw {
		typeID := i + mobOffset

		resourceType := ""
		resourceTier := mm.T
		if mm.L != "" {
			resourceType = normalizeResourceType(mm.L)
			if mm.LT != 0 {
				resourceTier = mm.LT
			}
		}

		db.byID[typeID] = Mob{
			UniqueName:    mm.U,
			CombatTier:    mm.T,
			Category:      mm.C,
			NameLocaTag:   mm.N,
			LootType:      mm.L,
			IsHarvestable: resourceType != "",
			ResourceType:  resourceType,
			ResourceTier:  resourceTier,
		}
		db.byName[mm.U] = typeID
	}
	return db, nil
}

// normalizeResourceType mirrors MobsDatabase.js's _normalizeResourceType exactly: maps a raw
// Loot.Harvestable @type (HIDE_CRITTER, FIBER_GUARDIAN, WOOD_TREASURE, ...) to this project's
// resource-type naming, or "" if the mob isn't a living resource (e.g. silver-coin loot).
func normalizeResourceType(lootType string) string {
	if lootType == "" {
		return ""
	}
	t := strings.ToUpper(lootType)

	if strings.HasPrefix(t, "SILVERCOINS") || strings.HasPrefix(t, "DEADRAT") {
		return ""
	}
	switch {
	case strings.HasPrefix(t, "HIDE"), strings.HasPrefix(t, "LEATHER"):
		return "Hide"
	case strings.HasPrefix(t, "FIBER"):
		return "Fiber"
	case strings.HasPrefix(t, "WOOD"):
		return "Log"
	case strings.HasPrefix(t, "ROCK"), strings.HasPrefix(t, "STONE"):
		return "Rock"
	case strings.HasPrefix(t, "ORE"):
		return "Ore"
	default:
		return ""
	}
}

// GetByID returns the mob with the given server TypeID.
func (d *MobsDatabase) GetByID(typeID int) (Mob, bool) {
	mob, ok := d.byID[typeID]
	return mob, ok
}

// GetIDByName returns the TypeID for a mob's UniqueName, or false if unknown.
func (d *MobsDatabase) GetIDByName(uniqueName string) (int, bool) {
	id, ok := d.byName[uniqueName]
	return id, ok
}

// Len reports how many mobs are loaded.
func (d *MobsDatabase) Len() int {
	return len(d.byID)
}
