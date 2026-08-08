package settingsui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// newIgnoreListPage is a port of internal/templates/pages/ignorelist.gohtml. It persists to the
// exact same "ignoreList" syncsettings key (a JSON array of exact player-name strings, no
// normalization) that cmd/radar/overlay_setup.go's isIgnoredPlayer already reads for both the
// overlay and the -overlay-only radarstate.PlayersState.
func newIgnoreListPage(s *Store) fyne.CanvasObject {
	var names []string
	s.GetJSON("ignoreList", &names)

	count := widget.NewLabel(strconv.Itoa(len(names)))

	var listWidget *widget.List
	save := func() {
		s.SetJSON("ignoreList", names)
		count.SetText(strconv.Itoa(len(names)))
		listWidget.Refresh()
	}

	listWidget = widget.NewList(
		func() int { return len(names) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewLabel(""), layout.NewSpacer(), widget.NewButton("Retirer", nil))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			row, ok := o.(*fyne.Container)
			if !ok {
				return
			}
			label, ok := row.Objects[0].(*widget.Label)
			if !ok {
				return
			}
			label.SetText(names[i])
			btn, ok := row.Objects[2].(*widget.Button)
			if !ok {
				return
			}
			btn.OnTapped = func() {
				names = append(names[:i], names[i+1:]...)
				save()
			}
		},
	)

	entry := widget.NewEntry()
	entry.SetPlaceHolder("Nom du joueur...")
	add := func() {
		name := strings.TrimSpace(entry.Text)
		if name == "" {
			return
		}
		for _, n := range names {
			if n == name {
				entry.SetText("")
				return
			}
		}
		names = append(names, name)
		entry.SetText("")
		save()
	}
	entry.OnSubmitted = func(string) { add() }

	top := container.NewBorder(nil, nil, nil, widget.NewButton("Ajouter", add), entry)
	header := container.NewBorder(nil, nil, nil, count,
		widget.NewLabelWithStyle("Joueurs ignorés", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(container.NewVBox(top, header), nil, nil, nil, listWidget)
}
