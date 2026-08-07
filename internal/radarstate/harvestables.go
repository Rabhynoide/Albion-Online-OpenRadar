package radarstate

import (
	"math"
	"strconv"
	"sync"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

// Harvestable mirrors web/scripts/handlers/HarvestablesHandler.js's Harvestable record.
// MobileTypeID is nil for a purely static resource (mirrors mobileTypeId being
// null/undefined/-1/65535 in the JS - all four spellings collapse to "static" there too).
type Harvestable struct {
	ID           int
	Type         int    // raw typeNumber (0-27)
	StringType   string // "Fiber"/"Hide"/"Log"/"Ore"/"Rock"
	Tier         int
	PosX         float32
	PosY         float32
	Charges      int // enchantment level (0-4)
	Size         int
	MobileTypeID *int
	Alerted      bool // one-shot sound-alert flag, see maybeAlertResource
}

func (h Harvestable) isLiving() bool {
	return h.MobileTypeID != nil && *h.MobileTypeID != 65535 && *h.MobileTypeID != -1
}

// EnchantGrid mirrors the Resources settings page's per-resource-type checkbox grid, as
// persisted by settingsSync.setJSON: keys "e0".."e4", each an 8-element tier(1-8) bool array.
type EnchantGrid map[string][]bool

func (g EnchantGrid) cell(enchant, tier int) bool {
	if g == nil || tier < 1 || tier > 8 || enchant < 0 || enchant > 4 {
		return false
	}
	row, ok := g["e"+strconv.Itoa(enchant)]
	if !ok || tier > len(row) {
		return false
	}
	return row[tier-1]
}

var livingSettingKeyByName = map[string]string{
	"Fiber": "settingLivingFiberEnchants", "Hide": "settingLivingHideEnchants",
	"Log": "settingLivingWoodEnchants", "Ore": "settingLivingOreEnchants", "Rock": "settingLivingRockEnchants",
}

var staticSettingKeyByName = map[string]string{
	"Fiber": "settingStaticFiberEnchants", "Hide": "settingStaticHideEnchants",
	"Log": "settingStaticWoodEnchants", "Ore": "settingStaticOreEnchants", "Rock": "settingStaticRockEnchants",
}

// harvestableTypeMap converts gamedata's WOOD/ROCK/FIBER/HIDE/ORE naming to this package's
// Fiber/Hide/Log/Ore/Rock naming, mirroring HarvestablesHandler.js's GetStringType typeMap.
var harvestableTypeMap = map[string]string{
	"WOOD": "Log", "ROCK": "Rock", "FIBER": "Fiber", "HIDE": "Hide", "ORE": "Ore",
}

// HarvestablesState is a Go port of HarvestablesHandler.js. Sound alerts are surfaced via
// PendingAlerts() rather than played directly (no browser Audio API here) - see
// internal/radarstate package doc.
//
// pendingAlerts is guarded by its own mutex, separate from entityList's internal locking - same
// reasoning as PlayersState's alertsMu: written from the Photon-dispatch goroutine
// (maybeAlertResource), read from internal/overlay's Ebiten goroutine (PendingAlerts).
type HarvestablesState struct {
	items        *entityList[int, Harvestable]
	harvestables *gamedata.HarvestablesDatabase
	mobs         *gamedata.MobsDatabase
	// getEnchantGrid resolves a Resources-page settings key (e.g. "settingStaticHideEnchants")
	// to its persisted grid - injected so this package stays settings-store-agnostic.
	getEnchantGrid func(key string) EnchantGrid
	soundEnabled   func() bool

	lastRangeCheck int64 // unix nano, throttles removeNotInRange like the JS's _lastRangeCheckAt

	alertsMu      sync.Mutex
	pendingAlerts []Harvestable
}

func NewHarvestablesState(
	harvestables *gamedata.HarvestablesDatabase,
	mobs *gamedata.MobsDatabase,
	getEnchantGrid func(key string) EnchantGrid,
	soundEnabled func() bool,
) *HarvestablesState {
	return &HarvestablesState{
		items:          newEntityList[int, Harvestable](),
		harvestables:   harvestables,
		mobs:           mobs,
		getEnchantGrid: getEnchantGrid,
		soundEnabled:   soundEnabled,
	}
}

// resolveStringType mirrors GetStringType's living/static branch selection: living resources
// use MobsDatabase (typeNumber is meaningless for them), static resources use
// HarvestablesDatabase's typeNumber bands.
func (s *HarvestablesState) resolveStringType(typeNumber int, mobileTypeID *int) string {
	if mobileTypeID != nil && *mobileTypeID != 65535 && *mobileTypeID != -1 && s.mobs != nil {
		if mob, ok := s.mobs.GetByID(*mobileTypeID); ok && mob.ResourceType != "" {
			return mob.ResourceType
		}
	}
	resourceType := gamedata.ResourceTypeFromTypeNumber(typeNumber)
	return harvestableTypeMap[resourceType] // "" (unmapped) mirrors GetStringType's "" fallback
}

// maybeAlertResource mirrors HarvestablesHandler.js's maybeAlertResource: queues a one-shot
// sound alert the first time a harvestable's current tier/enchant match the player's resource
// filters, using the same living/static filter split the drawing layer uses to decide whether
// to render it at all - matches HarvestablesDrawing.js/MobsDrawing.js's own resolution exactly
// (see resolveStringType).
func (s *HarvestablesState) maybeAlertResource(h *Harvestable) {
	if h.Alerted || s.soundEnabled == nil || !s.soundEnabled() || s.getEnchantGrid == nil {
		return
	}
	keyMap := staticSettingKeyByName
	if h.isLiving() {
		keyMap = livingSettingKeyByName
	}
	key, ok := keyMap[h.StringType]
	if !ok {
		return
	}
	grid := s.getEnchantGrid(key)
	if !grid.cell(h.Charges, h.Tier) {
		return
	}
	h.Alerted = true
	s.alertsMu.Lock()
	s.pendingAlerts = append(s.pendingAlerts, *h)
	s.alertsMu.Unlock()
}

// PendingAlerts drains and returns harvestables that just became sound-alert-worthy.
func (s *HarvestablesState) PendingAlerts() []Harvestable {
	s.alertsMu.Lock()
	defer s.alertsMu.Unlock()
	out := s.pendingAlerts
	s.pendingAlerts = nil
	return out
}

// ShouldRender reports whether h currently matches the Resources page's tier/enchant filter for
// its resource type (living vs static) - mirrors web/scripts/drawings/HarvestablesDrawing.js's
// own filterFn gate (LivingResourceFilter.js's shouldRenderLivingResource/
// shouldRenderStaticResource). That gate lives in the DRAWING layer in the web app, separate from
// HarvestablesHandler.js (which only gates the sound alert, see maybeAlertResource above) - a
// distinction the initial internal/overlay port missed, drawing every tracked harvestable
// unconditionally. Nothing is shown for a tier/enchant cell nobody has checked on the Resources
// page, matching that page's own all-unchecked-by-default grid.
func (s *HarvestablesState) ShouldRender(h Harvestable) bool {
	if s.getEnchantGrid == nil {
		return true
	}
	keyMap := staticSettingKeyByName
	if h.isLiving() {
		keyMap = livingSettingKeyByName
	}
	key, ok := keyMap[h.StringType]
	if !ok {
		return false
	}
	return s.getEnchantGrid(key).cell(h.Charges, h.Tier)
}

// addHarvestable mirrors addHarvestable: on first sight, creates the entity and checks the
// alert gate; on a later re-observation, only charges/stringType are refreshed (position/size
// stay as first observed - UpdateHarvestable is the one that also refreshes size).
func (s *HarvestablesState) addHarvestable(id, typeNumber, tier int, posX, posY float32, charges, size int, mobileTypeID *int) {
	stringType := s.resolveStringType(typeNumber, mobileTypeID)

	if _, exists := s.items.get(id); !exists {
		h := Harvestable{
			ID: id, Type: typeNumber, StringType: stringType, Tier: tier,
			PosX: posX, PosY: posY, Charges: charges, Size: size, MobileTypeID: mobileTypeID,
		}
		s.maybeAlertResource(&h)
		s.items.replace(id, h)
		return
	}

	s.items.mutate(id, func(h *Harvestable) {
		h.Charges = charges
		if stringType != "" {
			h.StringType = stringType
		}
	})
}

// updateHarvestable mirrors UpdateHarvestable: like addHarvestable, but an existing entity
// also has its Size refreshed (used by the individual-spawn event, where size legitimately
// changes on re-observation; the batch-spawn path uses addHarvestable instead).
func (s *HarvestablesState) updateHarvestable(id, typeNumber, tier int, posX, posY float32, charges, size int, mobileTypeID *int) {
	if _, exists := s.items.get(id); !exists {
		s.addHarvestable(id, typeNumber, tier, posX, posY, charges, size, mobileTypeID)
		return
	}
	stringType := s.resolveStringType(typeNumber, mobileTypeID)
	s.items.mutate(id, func(h *Harvestable) {
		h.Charges = charges
		h.Size = size
		if stringType != "" {
			h.StringType = stringType
		}
	})
}

// HandleNewHarvestableObject ports newHarvestableObject (Event 40, individual spawn/update):
// type=[5], mobileTypeId=[6], tier=[7], location=[8], size=[10]??0, enchant=[11]??0.
func (s *HarvestablesState) HandleNewHarvestableObject(id int, p Params) {
	typeNumber, ok := paramInt(p, 5)
	if !ok {
		return
	}
	tier := paramIntDefault(p, 7, 0)
	x, y, ok := paramPosition(p, 8)
	if !ok {
		return
	}
	enchant := paramIntDefault(p, 11, 0)
	size := paramIntDefault(p, 10, 0)

	var mobileTypeID *int
	if v, ok := paramInt(p, 6); ok {
		mobileTypeID = &v
	}

	s.updateHarvestable(id, typeNumber, tier, x, y, enchant, size, mobileTypeID)
}

// HandleNewSimpleHarvestableObjectList ports newSimpleHarvestableObject (Event 38, batch
// spawn): parallel arrays ids=[0], types=[1], tiers=[2], positions=[3] (flat [x0,y0,x1,y1,...],
// NOT wrapped in the "data" quirk unlike 0/1/2/4), counts=[4]. Enchant is always 0 here - Event
// 46 corrects it once known (see HandleHarvestableChangeState).
func (s *HarvestablesState) HandleNewSimpleHarvestableObjectList(p Params) {
	ids := paramIntSlice(p, 0)
	types := paramIntSlice(p, 1)
	tiers := paramIntSlice(p, 2)
	positions := paramFloat32Slice(p, 3)
	counts := paramIntSlice(p, 4)
	if ids == nil || types == nil || tiers == nil || counts == nil {
		return
	}

	for i, id := range ids {
		if i >= len(types) || i >= len(tiers) || i >= len(counts) || i*2+1 >= len(positions) {
			continue
		}
		s.addHarvestable(id, types[i], tiers[i], positions[i*2], positions[i*2+1], 0, counts[i], nil)
	}
}

// HandleHarvestableChangeState ports HarvestUpdateEvent (Event 46): id=[0], newSize=[1]
// (absent = depleted, remove), enchant=[2] (updates if present and different). Also re-checks
// the alert gate, since batch spawns (Event 38) start at enchant=0 and only become
// identifiable once this event corrects it.
func (s *HarvestablesState) HandleHarvestableChangeState(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	newSize, hasSize := paramInt(p, 1)
	if !hasSize {
		s.items.remove(id)
		return
	}
	enchant, hasEnchant := paramInt(p, 2)

	s.items.mutate(id, func(h *Harvestable) {
		h.Size = newSize
		if hasEnchant {
			h.Charges = enchant
		}
		s.maybeAlertResource(h)
	})
}

// RemoveNotInRange mirrors removeNotInRange: throttled to RANGE_CHECK_INTERVAL_MS (harvestables
// are stationary, so re-filtering by distance more often than the player can meaningfully cover
// ground is wasted work) - drops every entity further than 80 units from (lpX, lpY).
func (s *HarvestablesState) RemoveNotInRange(lpX, lpY float32, nowUnixNano int64) {
	const rangeCheckIntervalNs = int64(250 * 1_000_000) // 250ms
	if nowUnixNano-s.lastRangeCheck < rangeCheckIntervalNs {
		return
	}
	s.lastRangeCheck = nowUnixNano

	s.items.mu.Lock()
	defer s.items.mu.Unlock()
	for id, h := range s.items.items {
		dx, dy := float64(lpX-h.PosX), float64(lpY-h.PosY)
		if math.Sqrt(dx*dx+dy*dy) > 80 {
			delete(s.items.items, id)
			delete(s.items.lastSeen, id)
		}
	}
}

// UpdateHarvested mirrors updateHarvestable(harvestableId, count): decrements Size by count,
// removing the entity once its last stack is harvested (Size<=0). Named UpdateHarvested here
// (not UpdateHarvestable) to avoid colliding with this file's own internal updateHarvestable.
func (s *HarvestablesState) UpdateHarvested(id, count int) {
	remove := false
	s.items.mutate(id, func(h *Harvestable) {
		h.Size -= count
		if h.Size <= 0 {
			remove = true
		}
	})
	if remove {
		s.items.remove(id)
	}
}

func (s *HarvestablesState) Remove(id int) { s.items.remove(id) }
func (s *HarvestablesState) Clear()        { s.items.clear() }
func (s *HarvestablesState) Len() int      { return s.items.len() }
func (s *HarvestablesState) Snapshot() []Harvestable {
	return s.items.snapshot()
}
func (s *HarvestablesState) CleanupStale(maxAgeMs int) int {
	return s.items.cleanupStale(msToDuration(maxAgeMs))
}

// EnforceMaxSize mirrors enforceMaxSize: keeps only the maxSize most-recently-seen entries.
func (s *HarvestablesState) EnforceMaxSize(maxSize int) int {
	return s.items.enforceMaxSize(maxSize)
}
