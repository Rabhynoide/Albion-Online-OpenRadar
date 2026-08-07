package gamedata

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

// Item mirrors web/scripts/data/ItemsDatabase.js's per-item record exactly.
type Item struct {
	Name        string // UniqueName, e.g. "T4_BAG" or "T8_OFF_SPIKEDSHIELD_MORGANA@2"
	Tier        int
	ItemPower   int
	Enchant     int
	Category    string
	Subcategory string
}

// minifiedItem mirrors tools/update-ao-data.ts's minifyItems() output shape.
type minifiedItem struct {
	N   string `json:"n"`
	P   int    `json:"p"`
	Cat string `json:"cat"`
	Sub string `json:"sub"`
}

// ItemsDatabase looks items up by their sequential ID (index+1 in items.min.json, matching
// the game's own server-assigned item type IDs - see ItemsDatabase.js's identical convention).
type ItemsDatabase struct {
	byID map[int]Item
}

var tierPrefixRe = regexp.MustCompile(`^T(\d+)_`)

// LoadItems reads items.min.json(.gz) from fsys and parses it into an ItemsDatabase.
// Sequential IDs start at 1 (index 0 in the JSON array = game ID 1), matching
// ItemsDatabase.js's identical convention.
func LoadItems(fsys fs.FS, filename string) (*ItemsDatabase, error) {
	var raw []minifiedItem
	if err := readJSON(fsys, filename, &raw); err != nil {
		return nil, err
	}

	db := &ItemsDatabase{byID: make(map[int]Item, len(raw))}
	for i, mi := range raw {
		enchant := 0
		if at := strings.LastIndexByte(mi.N, '@'); at > 0 {
			if n, err := strconv.Atoi(mi.N[at+1:]); err == nil {
				enchant = n
			}
		}
		tier := 0
		if m := tierPrefixRe.FindStringSubmatch(mi.N); m != nil {
			tier, _ = strconv.Atoi(m[1])
		}

		db.byID[i+1] = Item{
			Name:        mi.N,
			Tier:        tier,
			ItemPower:   mi.P,
			Enchant:     enchant,
			Category:    mi.Cat,
			Subcategory: mi.Sub,
		}
	}
	return db, nil
}

// GetByID returns the item with the given sequential (server) ID.
func (d *ItemsDatabase) GetByID(id int) (Item, bool) {
	item, ok := d.byID[id]
	return item, ok
}

// Len reports how many items are loaded.
func (d *ItemsDatabase) Len() int {
	return len(d.byID)
}
