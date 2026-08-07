package overlay

import (
	"strconv"
	"time"

	"github.com/nospy/albion-openradar/internal/syncsettings"
)

// settingsRefreshInterval throttles how often settingsPanel re-reads settings-sync.json. It used
// to re-read on every Update() tick (30/sec, see targetTPS) - harmless on its own, but with
// internal/settingsui writing the same file from a separate process, that much read traffic made
// a transient Windows file-sharing collision on the writer's rename (see
// internal/syncsettings.renameWithRetry) far more likely than it needed to be. 200ms is
// imperceptible for a settings toggle to take effect but cuts read frequency ~6x.
const settingsRefreshInterval = 200 * time.Millisecond

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
	appDir      string
	snapshot    map[string]string
	lastRefresh time.Time
}

func newSettingsPanel(appDir string) *settingsPanel {
	p := &settingsPanel{appDir: appDir}
	p.snapshot = readSettings(p.appDir)
	p.lastRefresh = time.Now()
	return p
}

// refresh re-reads settings-sync.json, throttled to settingsRefreshInterval - called once per
// Update() tick, but only actually hits disk a few times a second (see settingsRefreshInterval's
// doc comment for why that throttle exists).
func (p *settingsPanel) refresh() {
	if time.Since(p.lastRefresh) < settingsRefreshInterval {
		return
	}
	p.lastRefresh = time.Now()
	p.snapshot = readSettings(p.appDir)
}

// isOn reports a boolean setting's current value, defaulting to false (hidden) for a setting
// that's never been set - matches web/scripts/utils/SettingsSync.js's own getBool(key) default
// (no default argument passed anywhere in enemies.gohtml/chests.gohtml's bindCheckbox calls) and
// every one of these pages' checkboxes actually being unchecked out of the box - opt-in
// filtering, not opt-out. (An earlier version of this defaulted to true, which is what
// SettingNameForEnemyType's hostile-enemy filter and the Dungeons filter both silently relied on
// - fixed alongside porting the missing Dungeons filter, see shouldRenderDungeon.)
func (p *settingsPanel) isOn(settingID string) bool {
	return p.snapshot[settingID] == "true"
}

// isOnDefault is isOn with a caller-chosen default instead of always false - for the handful of
// settings the web app itself defaults to true (e.g. settingShowKnightfallAbbey via
// `getBool('settingShowKnightfallAbbey', true)`), unlike the opt-in checkbox pages isOn's own
// default matches.
func (p *settingsPanel) isOnDefault(settingID string, def bool) bool {
	v, ok := p.snapshot[settingID]
	if !ok {
		return def
	}
	return v == "true"
}

// getNumber reports a numeric setting's current value (e.g. settingOverlayZoom,
// settingOverlayMapOpacity - edited from the native settings client's Settings page, see
// docs/technical/NATIVE_SETTINGS_CLIENT.md), defaulting to def for a setting that's never been
// set or isn't parsable as a float.
func (p *settingsPanel) getNumber(settingID string, def float64) float64 {
	v, ok := p.snapshot[settingID]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}
