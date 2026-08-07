package settingsui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/nospy/albion-openradar/internal/radarstate"
)

// resourceKind describes one of the 10 Static/Living × Fiber/Hide/Wood/Ore/Rock grids, matching
// web/scripts/utils/ResourcesHelper.js's `resources` array in resources.gohtml's inline script.
type resourceKind struct {
	label string // "Fiber", "Hide", "Wood", "Ore", "Rock"
	kind  string // "Static" or "Living", used verbatim in the storage key
}

// resourceStorageKey mirrors ResourcesHelper.js's getResourceStorageKey.
func resourceStorageKey(label, kind string) string {
	return "setting" + kind + label + "Enchants"
}

func gridCell(g radarstate.EnchantGrid, level string, tier int) bool {
	row, ok := g[level]
	if !ok || tier-1 >= len(row) || tier-1 < 0 {
		return false
	}
	return row[tier-1]
}

func gridSetCell(g radarstate.EnchantGrid, level string, tier int, value bool) {
	row, ok := g[level]
	if !ok {
		row = make([]bool, 8)
		g[level] = row
	}
	if tier-1 >= len(row) || tier-1 < 0 {
		return
	}
	row[tier-1] = value
}

var enchantLevels = []string{"e0", "e1", "e2", "e3", "e4"}

// newResourceGrid ports generateResourceGrid's 5(enchant)×8(tier) checkbox grid - E1-E4 only
// expose tiers 4-8 (index >= 3), matching Albion's own enchanted-resource tier floor - plus the
// per-tier "select all enchants for this tier" buttons (selectAllTierEnchants).
func newResourceGrid(s *Store, label, kind string) fyne.CanvasObject {
	key := resourceStorageKey(label, kind)
	grid := radarstate.EnchantGrid{}
	s.GetJSON(key, &grid)

	checks := make(map[string][8]*widget.Check)
	save := func() { s.SetJSON(key, grid) }

	for _, level := range enchantLevels {
		var row [8]*widget.Check
		checks[level] = row
	}

	tierButtons := container.New(layout.NewGridLayoutWithColumns(8))
	for tier := 1; tier <= 8; tier++ {
		t := tier
		btn := widget.NewButton(fmt.Sprintf("T%d", t), nil)
		btn.OnTapped = func() {
			allChecked := true
			any := false
			for _, level := range enchantLevels {
				c := checks[level][t-1]
				if c == nil {
					continue
				}
				if c.Checked {
					any = true
				} else {
					allChecked = false
				}
			}
			target := !(allChecked && any)
			for _, level := range enchantLevels {
				c := checks[level][t-1]
				if c == nil {
					continue
				}
				c.SetChecked(target)
			}
		}
		tierButtons.Add(btn)
	}

	rows := container.NewVBox(tierButtons)
	for _, level := range enchantLevels {
		lvl := level
		row := container.New(layout.NewGridLayoutWithColumns(8))
		arr := checks[level]
		for tier := 1; tier <= 8; tier++ {
			t := tier
			if lvl != "e0" && t < 4 {
				row.Add(layout.NewSpacer())
				continue
			}
			c := widget.NewCheck("", func(v bool) {
				gridSetCell(grid, lvl, t, v)
				save()
			})
			c.SetChecked(gridCell(grid, lvl, t))
			arr[t-1] = c
			row.Add(c)
		}
		checks[level] = arr
		rows.Add(container.NewHBox(widget.NewLabel(level), row))
	}

	return widget.NewCard(kind, "", rows)
}

// newResourceSection is one collapsible resource-type group (Fiber/Hide/Wood/Ore/Rock), showing
// the Static and Living grids side by side like resources.gohtml's two-column layout.
func newResourceSection(s *Store, label string) fyne.CanvasObject {
	return container.NewGridWithColumns(2,
		newResourceGrid(s, label, "Static"),
		newResourceGrid(s, label, "Living"),
	)
}

func newResourcesPage(s *Store) fyne.CanvasObject {
	accordion := widget.NewAccordion(
		widget.NewAccordionItem("Fiber", newResourceSection(s, "Fiber")),
		widget.NewAccordionItem("Hide", newResourceSection(s, "Hide")),
		widget.NewAccordionItem("Wood", newResourceSection(s, "Wood")),
		widget.NewAccordionItem("Ore", newResourceSection(s, "Ore")),
		widget.NewAccordionItem("Rock", newResourceSection(s, "Rock")),
	)
	accordion.Open(0)

	other := container.NewVBox(
		widget.NewLabelWithStyle("Alertes", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingResourceSound", "Alerte sonore", false),
		widget.NewLabelWithStyle("Autres ressources", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingFishing", "Afficher les zones de pêche", false),
		widget.NewLabelWithStyle("Visuel & outils", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingResourceCount", "Afficher le nombre de ressources", false),
		syncCheck(s, "settingResourceDistance", "Afficher la distance", false),
		syncCheck(s, "settingResourceClusters", "Afficher les clusters", false),
		widget.NewLabel("Rayon du cluster (mètres, 10-100)"),
		syncNumberEntry(s, "settingClusterRadius", 30, 10, 100),
		widget.NewLabel("Taille minimum du cluster (2-10)"),
		syncNumberEntry(s, "settingClusterMinSize", 2, 2, 10),
		syncCheck(s, "settingLivingResourcesHealthBar", "Afficher la barre de vie", false),
		syncCheck(s, "settingLivingResourcesID", "Afficher l'ID (debug)", false),
		syncCheck(s, "settingLivingResourcesName", "Afficher le nom DB (debug)", false),
	)

	return container.NewVScroll(container.NewVBox(accordion, widget.NewSeparator(), other))
}
