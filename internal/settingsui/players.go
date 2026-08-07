package settingsui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// newPlayersPage is a 1:1 port of internal/templates/pages/players.gohtml - every checkbox
// defaults to false and settingMaxPlayersDisplay defaults to 50 (1-100), exactly matching that
// page's bindCheckbox/settingsSync.getNumber calls.
func newPlayersPage(s *Store) fyne.CanvasObject {
	display := container.NewVBox(
		widget.NewLabelWithStyle("Affichage", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingShowPlayers", "Afficher les joueurs sur le radar", false),
		syncCheck(s, "settingItems", "Afficher l'équipement", false),
		syncCheck(s, "settingShowSpells", "Afficher les sorts", false),
		syncCheck(s, "settingShowPlayerHealthBar", "Afficher la barre de vie", false),
	)

	alerts := container.NewVBox(
		widget.NewLabelWithStyle("Alertes", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingFlash", "Flash à l'écran", false),
		syncCheck(s, "settingSound", "Alerte sonore", false),
		syncCheck(s, "settingFlashDangerousPlayer", "Bordure pulsante si hostile à proximité", false),
	)

	types := container.NewVBox(
		widget.NewLabelWithStyle("Types de joueurs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingPassivePlayers", "Passifs", false),
		syncCheck(s, "settingFactionPlayers", "Faction", false),
		syncCheck(s, "settingDangerousPlayers", "Hostiles", false),
	)

	limit := container.NewVBox(
		widget.NewLabelWithStyle("Limites", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Nombre maximum de joueurs affichés (1-100)"),
		syncNumberEntry(s, "settingMaxPlayersDisplay", 50, 1, 100),
	)

	return container.NewVScroll(container.NewVBox(display, widget.NewSeparator(), alerts,
		widget.NewSeparator(), types, widget.NewSeparator(), limit))
}
