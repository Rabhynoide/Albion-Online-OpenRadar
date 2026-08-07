package overlay

import (
	"testing"

	"github.com/nospy/albion-openradar/internal/syncsettings"
)

func TestNewSettingsPanel_DefaultsToOnWhenUnset(t *testing.T) {
	dir := t.TempDir()
	p := newSettingsPanel(dir)

	if !p.isOn("settingNormalEnemy") {
		t.Error("a never-set toggle should default to on, matching the web Settings page's own default-visible checkboxes")
	}
}

func TestNewSettingsPanel_LoadsPersistedState(t *testing.T) {
	dir := t.TempDir()
	if err := syncsettings.Set(dir, "settingBossEnemy", "false"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	p := newSettingsPanel(dir)

	if p.isOn("settingBossEnemy") {
		t.Error("a toggle persisted as false should load as off, not the default")
	}
}

func TestSettingsPanel_IsOn_UnmanagedSettingDefaultsTrue(t *testing.T) {
	p := newSettingsPanel(t.TempDir())

	if !p.isOn("settingSomethingThisPanelDoesNotManage") {
		t.Error("an unmanaged setting ID should default to visible/true")
	}
}

func TestWriteSettingBool_PersistsAndReadsBack(t *testing.T) {
	dir := t.TempDir()

	writeSettingBool(dir, "settingResourceSound", false)

	settings := readSettings(dir)
	if settings["settingResourceSound"] != "false" {
		t.Errorf("settingResourceSound = %q, want \"false\"", settings["settingResourceSound"])
	}
}

func TestKeyLabel(t *testing.T) {
	if got := keyLabel(keyF2); got != "F2" {
		t.Errorf("keyLabel(keyF2) = %q, want F2", got)
	}
}
