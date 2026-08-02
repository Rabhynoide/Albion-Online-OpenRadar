package syncsettings

import "testing"

func TestReadAll_MissingFileReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()

	settings, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("expected empty map, got %+v", settings)
	}
}

func TestSet_PersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()

	if err := Set(dir, "settingChestGreen", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	settings, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if settings["settingChestGreen"] != "true" {
		t.Errorf("expected settingChestGreen=true, got %+v", settings)
	}
}

func TestSet_UpsertsWithoutLosingOtherKeys(t *testing.T) {
	dir := t.TempDir()
	if err := Set(dir, "ignoreList", `["Alice"]`); err != nil {
		t.Fatalf("seed Set: %v", err)
	}

	if err := Set(dir, "settingMistSolo", "false"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	settings, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if settings["ignoreList"] != `["Alice"]` {
		t.Errorf("ignoreList lost by unrelated Set: %+v", settings)
	}
	if settings["settingMistSolo"] != "false" {
		t.Errorf("settingMistSolo not persisted: %+v", settings)
	}
}

func TestSet_OverwritesExistingKey(t *testing.T) {
	dir := t.TempDir()
	if err := Set(dir, "settingChestGreen", "true"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}

	if err := Set(dir, "settingChestGreen", "false"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	settings, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if settings["settingChestGreen"] != "false" {
		t.Errorf("expected settingChestGreen=false, got %+v", settings)
	}
}

func TestDelete_RemovesKeyWithoutAffectingOthers(t *testing.T) {
	dir := t.TempDir()
	if err := Set(dir, "settingChestGreen", "true"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	if err := Set(dir, "settingMistSolo", "false"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}

	if err := Delete(dir, "settingChestGreen"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	settings, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if _, ok := settings["settingChestGreen"]; ok {
		t.Errorf("settingChestGreen was not deleted: %+v", settings)
	}
	if settings["settingMistSolo"] != "false" {
		t.Errorf("settingMistSolo lost by unrelated Delete: %+v", settings)
	}
}

func TestDelete_AbsentKeyIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := Set(dir, "settingMistSolo", "false"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}

	if err := Delete(dir, "doesNotExist"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	settings, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if settings["settingMistSolo"] != "false" {
		t.Errorf("unrelated key affected by no-op delete: %+v", settings)
	}
}
