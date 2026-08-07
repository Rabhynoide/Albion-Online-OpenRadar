package radarstate

import (
	"strings"

	"github.com/nospy/albion-openradar/internal/gamedata"
)

// EnemyType mirrors MobsHandler.js's EnemyType enum.
type EnemyType int

const (
	EnemyLivingHarvestable EnemyType = iota
	EnemyLivingSkinnable
	EnemyNormal
	EnemyMedium
	EnemyEnchanted
	EnemyMiniBoss
	EnemyBoss
	EnemyDrone
	EnemyMistBoss
	EnemyEvents
)

// SettingNameForEnemyType mirrors getSettingNameForEnemyType: only these four enemy types have
// a dedicated Enemies-page filter checkbox ("" for every other type, matching JS's null).
func SettingNameForEnemyType(t EnemyType) string {
	switch t {
	case EnemyNormal:
		return "settingNormalEnemy"
	case EnemyEnchanted:
		return "settingEnchantedEnemy"
	case EnemyMiniBoss:
		return "settingMiniBossEnemy"
	case EnemyBoss:
		return "settingBossEnemy"
	default:
		return ""
	}
}

// Mob mirrors MobsHandler.js's Mob record.
type Mob struct {
	ID               int
	TypeID           int
	PosX             float32
	PosY             float32
	Health           int // normalized 0-255 (current HP percentage)
	MaxHealth        int // real max HP
	EnchantmentLevel int
	Rarity           *int
	Tier             int
	Type             EnemyType
	Name             string // living resource type ("Hide"/"Fiber"/...) or hostile UniqueName
	UniqueName       string
	Identified       bool
	Category         string
	NameLocaTag      string
}

// CurrentHP mirrors Mob.getCurrentHP.
func (m Mob) CurrentHP() int {
	return roundHalfAwayFromZero(float64(m.Health) / 255 * float64(m.MaxHealth))
}

// HealthPercent mirrors Mob.getHealthPercent.
func (m Mob) HealthPercent() int {
	return roundHalfAwayFromZero(float64(m.Health) / 255 * 100)
}

func roundHalfAwayFromZero(f float64) int {
	if f >= 0 {
		return int(f + 0.5)
	}
	return -int(-f + 0.5)
}

// Mist mirrors MobsHandler.js's Mist record (a Mists portal/wisp encountered as a "mob" on the
// wire - distinguished from a real enemy purely by carrying a name in NewMobEvent's payload).
type Mist struct {
	ID      int
	PosX    float32
	PosY    float32
	Name    string
	Enchant int
	IsSolo  bool
}

// calculateEnchantment mirrors calculateEnchantment: clamps to [0,4], defaulting to 0 when
// absent.
func calculateEnchantment(paramsEnchant *int) int {
	if paramsEnchant == nil {
		return 0
	}
	v := *paramsEnchant
	if v < 0 {
		return 0
	}
	if v > 4 {
		return 4
	}
	return v
}

// MobsState is a Go port of MobsHandler.js.
type MobsState struct {
	mobs  *entityList[int, Mob]
	mists *entityList[int, Mist]
	db    *gamedata.MobsDatabase
	// getEnchantGrid resolves a Resources-page settings key, same role as
	// HarvestablesState's field of the same name - set via SetEnchantGrid rather than required
	// at construction, so the many existing tests that don't care about resource filtering (this
	// state's real job is hostile-mob tracking) don't all need updating for it.
	getEnchantGrid func(key string) EnchantGrid
}

func NewMobsState(db *gamedata.MobsDatabase) *MobsState {
	return &MobsState{
		mobs:  newEntityList[int, Mob](),
		mists: newEntityList[int, Mist](),
		db:    db,
	}
}

// SetEnchantGrid wires the Resources-page grid lookup used by ShouldRender - see the field doc.
func (s *MobsState) SetEnchantGrid(fn func(key string) EnchantGrid) {
	s.getEnchantGrid = fn
}

// ShouldRender reports whether m currently matches the Resources page's tier/enchant filter -
// mirrors MobsDrawing.js's EnemyType.LivingSkinnable/LivingHarvestable branch
// (LivingResourceFilter.js's shouldRenderLivingResource), the SAME visual gate
// HarvestablesState.ShouldRender applies to static/other-living resources. Every other EnemyType
// is unaffected and always reports true here - it has its own Enemies-page gate, see
// SettingNameForEnemyType.
func (s *MobsState) ShouldRender(m Mob) bool {
	if m.Type != EnemyLivingHarvestable && m.Type != EnemyLivingSkinnable {
		return true
	}
	if s.getEnchantGrid == nil {
		return true
	}
	key, ok := livingSettingKeyByName[m.Name]
	if !ok {
		return false
	}
	return s.getEnchantGrid(key).cell(m.EnchantmentLevel, m.Tier)
}

// HandleNewMob ports NewMobEvent: id=[0], typeId=[1], location=[7]??[0,0], health=[2]??255,
// maxHealth=[13]??0, enchant=[33]??0, rarity=[19], name=[32]||[31]. A present name means this
// is actually a Mists portal/wisp, not a real enemy - routed to AddMist instead of AddEnemy,
// exactly mirroring the JS's own name-presence branch.
func (s *MobsState) HandleNewMob(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	typeID := paramIntDefault(p, 1, 0)
	posX, posY, hasLoc := paramPosition(p, 7)
	if !hasLoc {
		posX, posY = 0, 0
	}
	health := paramIntDefault(p, 2, 255)
	maxHealth := paramIntDefault(p, 13, 0)
	enchant := paramIntDefault(p, 33, 0)
	var rarity *int
	if r, ok := paramInt(p, 19); ok {
		rarity = &r
	}

	name := paramStringDefault(p, 32, "")
	if name == "" {
		name = paramStringDefault(p, 31, "")
	}

	if name != "" {
		s.addMist(id, posX, posY, name, enchant)
		return
	}
	s.addEnemy(id, typeID, posX, posY, health, maxHealth, enchant, rarity)
}

// addEnemy mirrors AddEnemy exactly, including the "spawn default HP" workaround (fort/dungeon
// NPCs sometimes spawn with a near-zero health_normalized value that means "not real data yet",
// not "nearly dead" - treated as full health instead).
func (s *MobsState) addEnemy(id, typeID int, posX, posY float32, healthNormalized, maxHealth, enchant int, rarity *int) {
	if _, exists := s.mobs.get(id); exists {
		return
	}

	actualHealth := healthNormalized
	if actualHealth < 10 {
		actualHealth = 255
	}

	mob := Mob{
		ID: id, TypeID: typeID, PosX: posX, PosY: posY,
		Health: actualHealth, MaxHealth: maxHealth, Rarity: rarity, Type: EnemyNormal,
	}

	hasKnownInfo := false
	if dbInfo, ok := s.db.GetByID(typeID); ok {
		hasKnownInfo = true
		if dbInfo.IsHarvestable {
			mob.Tier = dbInfo.ResourceTier
			mob.Name = dbInfo.ResourceType
			mob.UniqueName = dbInfo.UniqueName
			if dbInfo.ResourceType == "Hide" {
				mob.Type = EnemyLivingSkinnable
			} else {
				mob.Type = EnemyLivingHarvestable
			}
		} else {
			mob.Type = classifyEnemyCategory(dbInfo.Category, dbInfo.UniqueName)
			mob.Name = dbInfo.UniqueName
			mob.Tier = dbInfo.ResourceTier
			mob.Category = dbInfo.Category
			mob.NameLocaTag = dbInfo.NameLocaTag
		}
	}

	if mob.Type == EnemyLivingHarvestable || mob.Type == EnemyLivingSkinnable {
		e := enchant
		mob.EnchantmentLevel = calculateEnchantment(&e)
	}
	mob.Identified = mob.Name != "" && hasKnownInfo

	s.mobs.replace(id, mob)
}

// classifyEnemyCategory mirrors _getEnemyTypeFromCategory: name-based heuristics (VETERAN,
// ELITE, BOSS) take priority over the mobs.xml category, then falls back to Enemy for any
// unrecognized category (matching the JS switch's default case).
func classifyEnemyCategory(category, uniqueName string) EnemyType {
	name := strings.ToUpper(uniqueName)

	if strings.Contains(name, "_VETERAN") && !strings.Contains(name, "_VETERAN_CHAMPION") {
		return EnemyMiniBoss
	}
	if strings.Contains(name, "_ELITE") {
		return EnemyMiniBoss
	}
	if strings.Contains(name, "_BOSS") && !strings.Contains(name, "MINIBOSS") {
		return EnemyBoss
	}

	switch strings.ToLower(category) {
	case "boss":
		return EnemyBoss
	case "miniboss", "rd_elite", "rd_veteran":
		return EnemyMiniBoss
	case "champion":
		return EnemyEnchanted
	case "rd_solo":
		return EnemyEnchanted
	default:
		return EnemyNormal
	}
}

func (s *MobsState) Remove(id int) { s.mobs.remove(id) }

// UpdatePosition ports updateMobPosition.
func (s *MobsState) UpdatePosition(id int, posX, posY float32) {
	s.mobs.mutate(id, func(m *Mob) { m.PosX, m.PosY = posX, posY })
}

// HandleEnchantChanged ports updateEnchantEvent: mobId=[0], enchantmentLevel=[1].
func (s *MobsState) HandleEnchantChanged(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	level := paramIntDefault(p, 1, 0)
	s.mobs.mutate(id, func(m *Mob) { m.EnchantmentLevel = level })
}

// HandleHealthUpdate ports updateMobHealth (Event 6): mobId=[0], currentHP=[3] (real value,
// absent or <=0 = dead -> remove). Converts to normalized 0-255 against the mob's own MaxHealth.
func (s *MobsState) HandleHealthUpdate(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	if _, exists := s.mobs.get(id); !exists {
		return // not a mob (probably a player) - mirrors the JS's silent no-op
	}
	currentHP, hasHP := paramInt(p, 3)
	if !hasHP || currentHP <= 0 {
		s.mobs.remove(id)
		return
	}
	s.mobs.mutate(id, func(m *Mob) {
		if m.MaxHealth > 0 {
			m.Health = roundHalfAwayFromZero(float64(currentHP) / float64(m.MaxHealth) * 255)
		}
	})
}

// HandleHealthUpdateBulk ports updateMobHealthBulk (Event 7): arrays timestamps=[1],
// hpDeltas=[2], currentHPs=[3]. Every synthesized single-update reuses Parameters[0] as the id
// for every entity in the batch - kept verbatim even though the original JS comment flags this
// as possibly inaccurate, since this package translates known (if imperfect) JS behavior.
func (s *MobsState) HandleHealthUpdateBulk(p Params) {
	timestamps := paramIntSlice(p, 1)
	currentHPs := paramIntSlice(p, 3)
	if timestamps == nil || currentHPs == nil {
		return
	}
	sharedID, ok := paramInt(p, 0)
	if !ok {
		return
	}
	for _, hp := range currentHPs {
		s.HandleHealthUpdate(Params{0: int32(sharedID), 3: int32(hp)})
	}
}

// HandleHealthRegen ports updateMobHealthRegen (Event 91): health=[2], already normalized
// 0-255, stored directly with no MaxHealth involvement.
func (s *MobsState) HandleHealthRegen(p Params) {
	id, ok := paramInt(p, 0)
	if !ok {
		return
	}
	health, ok := paramInt(p, 2)
	if !ok {
		return
	}
	s.mobs.mutate(id, func(m *Mob) { m.Health = health })
}

func (s *MobsState) MobSnapshot() []Mob { return s.mobs.snapshot() }
func (s *MobsState) MobLen() int        { return s.mobs.len() }

// addMist mirrors AddMist: touch-on-duplicate, otherwise create.
func (s *MobsState) addMist(id int, posX, posY float32, name string, enchant int) {
	s.mists.upsert(id, func() Mist {
		return Mist{ID: id, PosX: posX, PosY: posY, Name: name, Enchant: enchant, IsSolo: strings.Contains(strings.ToLower(name), "solo")}
	})
}

func (s *MobsState) RemoveMist(id int) { s.mists.remove(id) }

// UpdateMistPosition ports updateMistPosition.
func (s *MobsState) UpdateMistPosition(id int, posX, posY float32) {
	s.mists.mutate(id, func(m *Mist) { m.PosX, m.PosY = posX, posY })
}

// UpdateMistEnchantmentLevel ports updateMistEnchantmentLevel.
func (s *MobsState) UpdateMistEnchantmentLevel(id, enchant int) {
	s.mists.mutate(id, func(m *Mist) { m.Enchant = enchant })
}

func (s *MobsState) MistSnapshot() []Mist { return s.mists.snapshot() }
func (s *MobsState) MistLen() int         { return s.mists.len() }

func (s *MobsState) Clear() {
	s.mobs.clear()
	s.mists.clear()
}

// CleanupStale mirrors cleanupStaleEntities, applied to both lists.
func (s *MobsState) CleanupStale(maxAgeMs int) int {
	return s.mobs.cleanupStale(msToDuration(maxAgeMs)) + s.mists.cleanupStale(msToDuration(maxAgeMs))
}

// EnforceMaxSize mirrors enforceMaxSize's independent per-list caps (default 500 mobs, 50
// mists in the JS - callers pass their own values here).
func (s *MobsState) EnforceMaxSize(maxMobs, maxMists int) int {
	return s.mobs.enforceMaxSize(maxMobs) + s.mists.enforceMaxSize(maxMists)
}
