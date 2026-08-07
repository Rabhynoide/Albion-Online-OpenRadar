package overlay

import (
	"github.com/nospy/albion-openradar/internal/syncsettings"
)

func readSettings(appDir string) map[string]string {
	settings, err := syncsettings.ReadAll(appDir)
	if err != nil {
		return map[string]string{}
	}
	return settings
}

// settingsPanel is a read-only view onto settings-sync.json - the same file
// web/scripts/utils/SettingsSync.js write-throughs to from the web app's Resources/Enemies/
// Settings pages, which remain the only place these get edited. The overlay never writes here
// itself: it only reads, to decide what to draw (e.g. Draw()'s per-EnemyType visibility
// filter) - deliberately kept simple rather than duplicating a settings UI inside the overlay
// window, see docs/technical/NATIVE_OVERLAY_CLIENT.md.
type settingsPanel struct {
	appDir   string
	snapshot map[string]string
}

func newSettingsPanel(appDir string) *settingsPanel {
	p := &settingsPanel{appDir: appDir}
	p.refresh()
	return p
}

// refresh re-reads settings-sync.json - cheap enough to call once per Update() tick so a
// change made on the web app while the overlay is running takes effect on the very next frame,
// without re-reading the file once per entity per frame.
func (p *settingsPanel) refresh() {
	p.snapshot = readSettings(p.appDir)
}

// isOn reports a boolean setting's current value, defaulting to true (visible) for a setting
// that's never been set - matches the web Settings/Resources/Enemies pages' own default-checked
// checkboxes.
func (p *settingsPanel) isOn(settingID string) bool {
	if v, ok := p.snapshot[settingID]; ok {
		return v == "true"
	}
	return true
}
