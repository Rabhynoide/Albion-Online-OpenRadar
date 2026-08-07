package settingsui

import (
	"strconv"

	"fyne.io/fyne/v2/widget"
)

// syncCheck builds a checkbox bound to a syncsettings boolean key: initial state read from the
// store, every toggle written straight back - mirrors the web pages' bindCheckbox pattern
// (SettingsSync.js's getBool/setBool via a "change" listener) with no local buffering.
func syncCheck(s *Store, key, label string, def bool) *widget.Check {
	c := widget.NewCheck(label, func(v bool) { s.SetBool(key, v) })
	c.SetChecked(s.GetBool(key, def))
	return c
}

// syncNumberEntry builds a clamped numeric entry bound to a syncsettings number key. Out-of-range
// or unparsable input is simply not persisted (the field keeps whatever the user typed until they
// fix it) - mirrors the web pages' own "if (val >= min && val <= max) setNumber(...)" guards.
func syncNumberEntry(s *Store, key string, def, min, max float64) *widget.Entry {
	e := widget.NewEntry()
	e.SetText(strconv.FormatFloat(s.GetNumber(key, def), 'f', -1, 64))
	e.OnChanged = func(text string) {
		v, err := strconv.ParseFloat(text, 64)
		if err != nil || v < min || v > max {
			return
		}
		s.SetNumber(key, v)
	}
	return e
}
