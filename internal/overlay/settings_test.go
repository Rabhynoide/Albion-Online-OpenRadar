package overlay

import (
	"testing"

	"github.com/nospy/albion-openradar/internal/syncsettings"
)

func TestNewSettingsPanel_DefaultsToOnWhenUnset(t *testing.T) {
	dir := t.TempDir()
	p := newSettingsPanel(dir)

	if !p.isOn("settingNormalEnemy") {
		t.Error("a never-set setting should default to on, matching the web Settings page's own default-visible checkboxes")
	}
}

func TestNewSettingsPanel_LoadsPersistedState(t *testing.T) {
	dir := t.TempDir()
	if err := syncsettings.Set(dir, "settingBossEnemy", "false"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	p := newSettingsPanel(dir)

	if p.isOn("settingBossEnemy") {
		t.Error("a setting persisted as false should load as off, not the default")
	}
}

func TestSettingsPanel_Refresh_PicksUpChangesFromTheWebApp(t *testing.T) {
	dir := t.TempDir()
	p := newSettingsPanel(dir)
	if !p.isOn("settingMiniBossEnemy") {
		t.Fatal("expected default on before any change")
	}

	// Simulates the web app's own Enemies page writing a change while the overlay is running -
	// the overlay never writes here itself, only reads (see settings.go's doc comment).
	if err := syncsettings.Set(dir, "settingMiniBossEnemy", "false"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}
	p.refresh()

	if p.isOn("settingMiniBossEnemy") {
		t.Error("refresh() should pick up the change written by the web app")
	}
}

func TestSettingsPanel_IsOn_UnmanagedSettingDefaultsTrue(t *testing.T) {
	p := newSettingsPanel(t.TempDir())

	if !p.isOn("settingSomethingThisPanelDoesNotManage") {
		t.Error("an unrecognized setting ID should default to visible/true")
	}
}
