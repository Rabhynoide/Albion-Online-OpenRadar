package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nospy/albion-openradar/internal/syncsettings"
)

func newSettingsSyncTestMux(dir string) *http.ServeMux {
	api := NewSettingsSyncAPI(dir)
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}

func TestSettingsSync_GetOnEmptyStoreReturnsEmptyObject(t *testing.T) {
	dir := t.TempDir()
	mux := newSettingsSyncTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/sync", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("expected empty object, got %+v", body)
	}
}

func TestSettingsSync_PostThenGetRoundTrips(t *testing.T) {
	dir := t.TempDir()
	mux := newSettingsSyncTestMux(dir)

	postBody, _ := json.Marshal(map[string]string{"key": "settingChestGreen", "value": "true"})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/sync", bytes.NewReader(postBody))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST status %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings/sync", http.NoBody)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["settingChestGreen"] != "true" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestSettingsSync_PostMissingKeyReturnsBadRequest(t *testing.T) {
	dir := t.TempDir()
	mux := newSettingsSyncTestMux(dir)

	postBody, _ := json.Marshal(map[string]string{"value": "true"})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/sync", bytes.NewReader(postBody))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestSettingsSync_PostInvalidJsonReturnsBadRequest(t *testing.T) {
	dir := t.TempDir()
	mux := newSettingsSyncTestMux(dir)

	req := httptest.NewRequest(http.MethodPost, "/api/settings/sync", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestSettingsSync_DeleteRemovesKey(t *testing.T) {
	dir := t.TempDir()
	if err := syncsettings.Set(dir, "settingChestGreen", "true"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	mux := newSettingsSyncTestMux(dir)

	req := httptest.NewRequest(http.MethodDelete, "/api/settings/sync?key=settingChestGreen", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	settings, err := syncsettings.ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if _, ok := settings["settingChestGreen"]; ok {
		t.Errorf("key not deleted: %+v", settings)
	}
}

func TestSettingsSync_DeleteMissingKeyParamReturnsBadRequest(t *testing.T) {
	dir := t.TempDir()
	mux := newSettingsSyncTestMux(dir)

	req := httptest.NewRequest(http.MethodDelete, "/api/settings/sync", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}
