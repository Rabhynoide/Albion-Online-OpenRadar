package settingsui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// enemyPresetKeys mirrors applyEnemyPreset's `settings` map in enemies.gohtml's inline script -
// every key it can touch, all defaulting to false before a preset is applied.
var enemyPresetKeys = []string{
	"settingAllEnemies", "settingNormalEnemy", "settingEnchantedEnemy", "settingMiniBossEnemy",
	"settingBossEnemy", "settingBossCrystalSpider", "settingBossFairyDragon", "settingBossVeilWeaver",
	"settingBossGriffin", "settingAvaloneDrones", "settingShowEventEnemies",
}

var bossPresetKeys = []string{
	"settingBossEnemy", "settingBossCrystalSpider", "settingBossFairyDragon", "settingBossVeilWeaver",
	"settingBossGriffin",
}

func newEnemiesPage(s *Store) fyne.CanvasObject {
	checks := make(map[string]*widget.Check, len(enemyPresetKeys)+6)

	all := syncCheck(s, "settingAllEnemies", "Tout", false)
	normal := syncCheck(s, "settingNormalEnemy", "Normal", false)
	enchanted := syncCheck(s, "settingEnchantedEnemy", "Enchanté", false)
	miniBoss := syncCheck(s, "settingMiniBossEnemy", "Mini-Boss", false)
	boss := syncCheck(s, "settingBossEnemy", "Boss", false)
	checks["settingAllEnemies"] = all
	checks["settingNormalEnemy"] = normal
	checks["settingEnchantedEnemy"] = enchanted
	checks["settingMiniBossEnemy"] = miniBoss
	checks["settingBossEnemy"] = boss

	// HasToCheckCheckAllEnemies: keep "All" in sync whenever the 4 classic types change,
	// without re-entering itself while the "All" handler is driving them (updatingAllEnemies).
	updatingAll := false
	syncAllCheckbox := func() {
		if updatingAll {
			return
		}
		allChecked := normal.Checked && enchanted.Checked && miniBoss.Checked && boss.Checked
		if allChecked != all.Checked {
			all.SetChecked(allChecked)
		}
	}
	for _, c := range []*widget.Check{normal, enchanted, miniBoss, boss} {
		orig := c.OnChanged
		c.OnChanged = func(v bool) {
			orig(v)
			syncAllCheckbox()
		}
	}
	all.OnChanged = func(v bool) {
		s.SetBool("settingAllEnemies", v)
		updatingAll = true
		for _, c := range []*widget.Check{normal, enchanted, miniBoss, boss} {
			c.SetChecked(v)
		}
		updatingAll = false
	}

	minHealth := syncCheck(s, "settingShowMinimumHealthEnemies", "Afficher les ennemis avec un PV minimum", false)
	minHealthEntry := syncNumberEntry(s, "settingTextMinimumHealthEnemies", 2100, 0, 1e9)
	unmanaged := syncCheck(s, "settingShowUnmanagedEnemies", "Afficher les IDs non gérés (debug)", false)
	minHealthEntry.Disable()
	if minHealth.Checked {
		minHealthEntry.Enable()
	}
	minHealth.OnChanged = func(v bool) {
		s.SetBool("settingShowMinimumHealthEnemies", v)
		if v {
			minHealthEntry.Enable()
			if !unmanaged.Checked {
				unmanaged.SetChecked(true)
			}
		} else {
			minHealthEntry.Disable()
		}
	}

	classic := container.NewVBox(
		widget.NewLabelWithStyle("Ennemis classiques", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		all, normal, enchanted, miniBoss, boss,
		widget.NewSeparator(),
		minHealth,
		container.NewHBox(widget.NewLabel("PV minimum"), minHealthEntry),
		unmanaged,
	)

	checks["settingBossCrystalSpider"] = syncCheck(s, "settingBossCrystalSpider", "Crystal Spider", false)
	checks["settingBossFairyDragon"] = syncCheck(s, "settingBossFairyDragon", "Fairy Dragon", false)
	checks["settingBossVeilWeaver"] = syncCheck(s, "settingBossVeilWeaver", "Veil Weaver", false)
	checks["settingBossGriffin"] = syncCheck(s, "settingBossGriffin", "Griffin", false)
	mists := container.NewVBox(
		widget.NewLabelWithStyle("Boss des Brumes (non implémenté)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		checks["settingBossCrystalSpider"], checks["settingBossFairyDragon"],
		checks["settingBossVeilWeaver"], checks["settingBossGriffin"],
	)

	checks["settingAvaloneDrones"] = syncCheck(s, "settingAvaloneDrones", "Drones Avaloniens", false)
	checks["settingShowEventEnemies"] = syncCheck(s, "settingShowEventEnemies", "Ennemis d'évènement", false)
	other := container.NewVBox(
		widget.NewLabelWithStyle("Autres ennemis", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		checks["settingAvaloneDrones"], checks["settingShowEventEnemies"],
	)

	debug := container.NewVBox(
		widget.NewLabelWithStyle("Debug", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingEnemiesHealthBar", "Afficher la barre de vie", false),
		syncCheck(s, "settingEnemiesID", "Afficher l'ID", false),
		syncCheck(s, "settingEnemiesTier", "Afficher le tier", false),
		syncCheck(s, "settingEnemiesName", "Afficher le nom", false),
		syncCheck(s, "settingEnemiesCategoryBadge", "Afficher le badge de catégorie", false),
	)

	presets := container.NewHBox(
		widget.NewButton("Tout", func() { applyEnemyPreset(s, checks, "all") }),
		widget.NewButton("Boss", func() { applyEnemyPreset(s, checks, "bosses") }),
		widget.NewButton("Mini-Boss+", func() { applyEnemyPreset(s, checks, "miniboss") }),
		widget.NewButton("Effacer", func() { applyEnemyPreset(s, checks, "clear") }),
	)

	return container.NewVScroll(container.NewVBox(presets, widget.NewSeparator(), classic,
		widget.NewSeparator(), mists, widget.NewSeparator(), other, widget.NewSeparator(), debug))
}

// applyEnemyPreset ports enemies.gohtml's window.applyEnemyPreset exactly: it always starts from
// every key false, then flips the preset's subset to true, for both "all" and "clear" (an empty
// subset) as well as "bosses"/"miniboss".
func applyEnemyPreset(s *Store, checks map[string]*widget.Check, preset string) {
	values := make(map[string]bool, len(enemyPresetKeys))
	for _, k := range enemyPresetKeys {
		values[k] = false
	}
	switch preset {
	case "all":
		for k := range values {
			values[k] = true
		}
	case "bosses":
		for _, k := range bossPresetKeys {
			values[k] = true
		}
	case "miniboss":
		values["settingMiniBossEnemy"] = true
		for _, k := range bossPresetKeys {
			values[k] = true
		}
	}
	// Fixed iteration order (not a map range) and a silent widget update, matching the JS's own
	// `el.checked = val` (no change-event dispatch): applying a preset must not cascade through
	// the "All" checkbox's own click handler, or Go's randomized map order would make the last
	// key processed (which could be "settingAllEnemies" itself, forcing the 4 classic checks back
	// to its own value) nondeterministically clobber the others.
	for _, key := range enemyPresetKeys {
		v := values[key]
		s.SetBool(key, v)
		if c, ok := checks[key]; ok {
			silentSetChecked(c, v)
		}
	}
}

// silentSetChecked updates a checkbox's displayed state without invoking its OnChanged callback
// - used when programmatic logic (a preset button) needs to reflect a value that's already been
// persisted through some other path, without re-triggering that path's side effects.
func silentSetChecked(c *widget.Check, v bool) {
	onChanged := c.OnChanged
	c.OnChanged = nil
	c.SetChecked(v)
	c.OnChanged = onChanged
}
