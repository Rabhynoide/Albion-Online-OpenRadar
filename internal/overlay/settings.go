package overlay

import (
	"strconv"

	"github.com/nospy/albion-openradar/internal/syncsettings"
)

func readSettings(appDir string) map[string]string {
	settings, err := syncsettings.ReadAll(appDir)
	if err != nil {
		return map[string]string{}
	}
	return settings
}

func writeSettingBool(appDir, key string, value bool) {
	_ = syncsettings.Set(appDir, key, strconv.FormatBool(value))
}

// toggle mirrors one keyboard-driven settings toggle: a key to poll, the syncsettings key it
// reads/writes, a human label for the HUD, and whether it's currently on. Kept intentionally
// simple (a handful of standalone booleans, not the Resources page's full tier x enchant grid -
// see docs/technical/NATIVE_OVERLAY_CLIENT.md for why a full graphical settings panel was
// scoped out of this first pass) but real: each one actually gates what Draw() renders.
type toggle struct {
	key       ebitenKey
	settingID string
	label     string
	on        bool
	wasDown   bool
}

// settingsPanel owns every keyboard-toggle and persists changes to settings-sync.json,
// defaulting each toggle to "on" (matches the web Settings page's own default-visible
// checkboxes) when the key has never been set.
type settingsPanel struct {
	appDir  string
	toggles []*toggle
}

func newSettingsPanel(appDir string) *settingsPanel {
	p := &settingsPanel{appDir: appDir}
	saved := readSettings(appDir)
	add := func(key ebitenKey, settingID, label string) {
		on := true
		if v, ok := saved[settingID]; ok {
			on = v == "true"
		}
		p.toggles = append(p.toggles, &toggle{key: key, settingID: settingID, label: label, on: on})
	}

	add(keyF2, "settingNormalEnemy", "Normal enemies")
	add(keyF3, "settingEnchantedEnemy", "Enchanted enemies")
	add(keyF4, "settingMiniBossEnemy", "Mini-bosses")
	add(keyF5, "settingBossEnemy", "Bosses")
	add(keyF6, "settingResourceSound", "Resource sound alert")

	return p
}

// update polls every toggle's key globally (same reasoning as the click-through F9 toggle -
// see clickthrough_windows.go) and flips+persists on a fresh keydown.
func (p *settingsPanel) update() {
	for _, t := range p.toggles {
		down := isKeyDownGlobally(t.key)
		if down && !t.wasDown {
			t.on = !t.on
			writeSettingBool(p.appDir, t.settingID, t.on)
		}
		t.wasDown = down
	}
}

var keyLabels = map[ebitenKey]string{
	keyF2: "F2", keyF3: "F3", keyF4: "F4", keyF5: "F5", keyF6: "F6", keyF9: "F9",
}

func keyLabel(k ebitenKey) string {
	if label, ok := keyLabels[k]; ok {
		return label
	}
	return "?"
}

func (p *settingsPanel) isOn(settingID string) bool {
	for _, t := range p.toggles {
		if t.settingID == settingID {
			return t.on
		}
	}
	return true // an unmanaged setting defaults to visible, matching the web page's own defaults
}
