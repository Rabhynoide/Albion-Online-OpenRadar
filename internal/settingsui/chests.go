package settingsui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// newChestsPage is a 1:1 port of internal/templates/pages/chests.gohtml - every checkbox
// defaults to false, matching that page's own bindCheckbox calls (no default arguments).
func newChestsPage(s *Store) fyne.CanvasObject {
	chests := container.NewVBox(
		widget.NewLabelWithStyle("Coffres", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingChestGreen", "Vert", false),
		syncCheck(s, "settingChestBlue", "Bleu", false),
		syncCheck(s, "settingChestPurple", "Violet", false),
		syncCheck(s, "settingChestYellow", "Jaune", false),
	)

	treasures := container.NewVBox(
		widget.NewLabelWithStyle("Trésors locaux", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingLocalTreasures", "Afficher les trésors locaux", false),
	)

	mists := container.NewVBox(
		widget.NewLabelWithStyle("Brumes - Type", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingMistSolo", "Solo", false),
		syncCheck(s, "settingMistDuo", "Duo", false),
		widget.NewLabelWithStyle("Brumes - Niveau d'enchantement", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingMistE0", "E0", false),
		syncCheck(s, "settingMistE1", "E1", false),
		syncCheck(s, "settingMistE2", "E2", false),
		syncCheck(s, "settingMistE3", "E3", false),
		syncCheck(s, "settingMistE4", "E4", false),
		syncCheck(s, "settingCage", "Afficher les cages à esprits", false),
		syncCheck(s, "settingWispSpawn", "Signes d'esprit (avant portail)", false),
		syncCheck(s, "settingWispSpawnDebugID", "Afficher l'ID d'esprit (debug)", false),
		syncCheck(s, "settingShowKnightfallAbbey", "Afficher Knightfall Abbey", false),
	)

	dungeons := container.NewVBox(
		widget.NewLabelWithStyle("Donjons - Type", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingDungeonSolo", "Solo", false),
		syncCheck(s, "settingDungeonDuo", "Groupe", false),
		widget.NewLabelWithStyle("Donjons - Niveau d'enchantement", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingDungeonE0", "E0", false),
		syncCheck(s, "settingDungeonE1", "E1", false),
		syncCheck(s, "settingDungeonE2", "E2", false),
		syncCheck(s, "settingDungeonE3", "E3", false),
		syncCheck(s, "settingDungeonE4", "E4", false),
		widget.NewLabelWithStyle("Donjons - Spécial", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingDungeonCorrupted", "Corrompu (PvP)", false),
		syncCheck(s, "settingDungeonHellgate", "Hellgate (PvP)", false),
	)

	return container.NewVScroll(container.NewVBox(chests, widget.NewSeparator(), treasures,
		widget.NewSeparator(), mists, widget.NewSeparator(), dungeons))
}
