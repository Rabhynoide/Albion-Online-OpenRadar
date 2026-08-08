package settingsui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type navItem struct {
	label   string
	content fyne.CanvasObject
}

// buildSidebarNav lays out a vertical navigation list on its own panel background - mirroring
// the web app's left sidebar (internal/templates' base layout, see theme.go for the shared color
// palette) - with the selected item highlighted via the theme's primary color, and a stacked
// content area on the right that swaps to the selected page.
func buildSidebarNav(items []navItem) (sidebar, content fyne.CanvasObject) {
	pages := container.NewStack()
	buttons := make([]*widget.Button, len(items))

	show := func(i int) {
		pages.RemoveAll()
		pages.Add(items[i].content)
		for j, b := range buttons {
			if j == i {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.LowImportance
			}
			b.Refresh()
		}
	}

	navBox := container.NewVBox()
	for i, item := range items {
		b := widget.NewButton(item.label, func() { show(i) })
		b.Alignment = widget.ButtonAlignLeading
		buttons[i] = b
		navBox.Add(b)
	}
	show(0)

	bg := canvas.NewRectangle(radarPanel)
	return container.NewStack(bg, container.NewPadded(navBox)), pages
}
