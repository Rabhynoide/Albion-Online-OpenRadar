package overlay

import (
	"testing"
	"time"

	"github.com/nospy/albion-openradar/internal/syncsettings"
)

// @verified: matches web/scripts/utils/SettingsSync.js's own getBool(key) default (false) - none
// of enemies.gohtml/chests.gohtml/resources.gohtml's bindCheckbox calls pass a default, so a
// never-touched setting is unchecked in the browser too. isOn used to default to true here,
// which silently showed every hostile enemy type (and, before shouldRenderDungeon existed, meant
// dungeons had no filter to default anything on) until the user explicitly unchecked them -
// the opposite of the actual opt-in design.
func TestNewSettingsPanel_DefaultsToOffWhenUnset(t *testing.T) {
	dir := t.TempDir()
	p := newSettingsPanel(dir)

	if p.isOn("settingNormalEnemy") {
		t.Error("a never-set setting should default to off, matching the web pages' own default-unchecked checkboxes")
	}
}

func TestNewSettingsPanel_LoadsPersistedState(t *testing.T) {
	dir := t.TempDir()
	if err := syncsettings.Set(dir, "settingBossEnemy", "true"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	p := newSettingsPanel(dir)

	if !p.isOn("settingBossEnemy") {
		t.Error("a setting persisted as true should load as on, not the default")
	}
}

func TestSettingsPanel_Refresh_PicksUpChangesFromTheWebApp(t *testing.T) {
	dir := t.TempDir()
	p := newSettingsPanel(dir)
	if p.isOn("settingMiniBossEnemy") {
		t.Fatal("expected default off before any change")
	}

	// Simulates the web app's own Enemies page writing a change while the overlay is running -
	// the overlay never writes here itself, only reads (see settings.go's doc comment).
	if err := syncsettings.Set(dir, "settingMiniBossEnemy", "true"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}
	time.Sleep(settingsRefreshInterval + 10*time.Millisecond) // past the throttle window, see refresh's doc
	p.refresh()

	if !p.isOn("settingMiniBossEnemy") {
		t.Error("refresh() should pick up the change written by the web app")
	}
}

// @verified: settingsRefreshInterval exists specifically to stop hammering settings-sync.json
// (was re-read on every Update() tick, 30/sec) - a real observed bug, not hypothetical: with
// internal/settingsui writing the same file from a separate process, that read frequency made a
// transient Windows file-sharing collision on the writer's rename likely enough that a
// Resources-page checkbox toggle could silently fail to persist (see
// internal/syncsettings.renameWithRetry).
func TestSettingsPanel_Refresh_ThrottledWithinInterval(t *testing.T) {
	dir := t.TempDir()
	p := newSettingsPanel(dir)

	if err := syncsettings.Set(dir, "settingMiniBossEnemy", "true"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}
	p.refresh() // called immediately after construction - within the throttle window

	if p.isOn("settingMiniBossEnemy") {
		t.Error("refresh() called within settingsRefreshInterval of the last read should be a no-op")
	}
}

func TestSettingsPanel_IsOn_UnmanagedSettingDefaultsFalse(t *testing.T) {
	p := newSettingsPanel(t.TempDir())

	if p.isOn("settingSomethingThisPanelDoesNotManage") {
		t.Error("an unrecognized setting ID should default to hidden/false")
	}
}
