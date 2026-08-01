package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nospy/albion-openradar/internal/capture"
)

func newHubSettingsTestMux(dir string) *http.ServeMux {
	api := NewHubSettingsAPI(dir)
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}

func TestHubSettings_GetReturnsCurrentConfig(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		Hub: capture.HubConfig{Enabled: true, URL: "http://hub.local:8090", Secret: "s3cr3t"},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newHubSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/hub", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["enabled"] != true || body["url"] != "http://hub.local:8090" || body["secret"] != "s3cr3t" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestHubSettings_GetDefaultsToDisabled(t *testing.T) {
	dir := t.TempDir()
	mux := newHubSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/hub", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["enabled"] != false {
		t.Errorf("expected disabled by default, got %+v", body)
	}
}

func TestHubSettings_PostUpdatesAllFields(t *testing.T) {
	dir := t.TempDir()
	mux := newHubSettingsTestMux(dir)

	body, _ := json.Marshal(map[string]any{
		"enabled": true,
		"url":     "http://hub.local:8090",
		"secret":  "s3cr3t",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/hub", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := capture.ReadConfig(dir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !cfg.Hub.Enabled || cfg.Hub.URL != "http://hub.local:8090" || cfg.Hub.Secret != "s3cr3t" {
		t.Errorf("hub config not persisted correctly: %+v", cfg.Hub)
	}
}

func TestHubSettings_PostPartialBodyLeavesOtherFieldsUntouched(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		Hub: capture.HubConfig{Enabled: true, URL: "http://hub.local:8090", Secret: "s3cr3t"},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newHubSettingsTestMux(dir)

	body, _ := json.Marshal(map[string]any{"enabled": false})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/hub", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	cfg, err := capture.ReadConfig(dir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.Hub.Enabled {
		t.Error("enabled not updated to false")
	}
	if cfg.Hub.URL != "http://hub.local:8090" || cfg.Hub.Secret != "s3cr3t" {
		t.Errorf("URL/Secret changed unexpectedly by partial update: %+v", cfg.Hub)
	}
}

func TestHubSettings_PostInvalidJson(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		Hub: capture.HubConfig{Enabled: true},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newHubSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodPost, "/api/settings/hub", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	cfg, err := capture.ReadConfig(dir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !cfg.Hub.Enabled {
		t.Error("network.json was modified despite invalid JSON")
	}
}

func TestHubSettings_PostPreservesOtherConfigSections(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		CaptureInterfaces: []capture.PersistedInterface{{Name: "X"}},
		Logging:           capture.LoggingConfig{ServerLogsEnabled: true},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newHubSettingsTestMux(dir)

	body, _ := json.Marshal(map[string]any{"enabled": true, "url": "http://hub.local:8090"})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/hub", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	cfg, err := capture.ReadConfig(dir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if len(cfg.CaptureInterfaces) != 1 || cfg.CaptureInterfaces[0].Name != "X" {
		t.Errorf("CaptureInterfaces changed: %+v", cfg.CaptureInterfaces)
	}
	if !cfg.Logging.ServerLogsEnabled {
		t.Error("Logging.ServerLogsEnabled was reset by unrelated hub POST")
	}
}

func TestHubStatus_DisabledReportsEnabledFalse(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		Hub: capture.HubConfig{Enabled: false, URL: "http://127.0.0.1:1"},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newHubSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/hub/status", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["enabled"] != false || body["connected"] != false {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestHubStatus_EnabledAndReachableReportsConnected(t *testing.T) {
	hubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer hubSrv.Close()

	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		Hub: capture.HubConfig{Enabled: true, URL: hubSrv.URL, Secret: "s"},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newHubSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/hub/status", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["enabled"] != true || body["connected"] != true {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestHubStatus_EnabledButUnreachableReportsDisconnected(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		Hub: capture.HubConfig{Enabled: true, URL: "http://127.0.0.1:1", Secret: "s"},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newHubSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/hub/status", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["enabled"] != true || body["connected"] != false {
		t.Errorf("unexpected body: %+v", body)
	}
}
