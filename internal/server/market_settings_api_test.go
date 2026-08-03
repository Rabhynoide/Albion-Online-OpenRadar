package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nospy/albion-openradar/internal/capture"
)

func newMarketSettingsTestMux(dir string) *http.ServeMux {
	api := NewMarketSettingsAPI(dir)
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}

func TestMarketSettings_GetDefaultsToEurope(t *testing.T) {
	dir := t.TempDir()
	mux := newMarketSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/market", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["region"] != "europe" {
		t.Errorf("region = %v, want europe", body["region"])
	}
}

func TestMarketSettings_GetReturnsCurrentConfig(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		Market: capture.MarketConfig{Region: "americas"},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newMarketSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/market", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["region"] != "americas" {
		t.Errorf("region = %v, want americas", body["region"])
	}
}

func TestMarketSettings_PostUpdatesRegion(t *testing.T) {
	dir := t.TempDir()
	mux := newMarketSettingsTestMux(dir)

	body, _ := json.Marshal(map[string]any{"region": "asia"})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/market", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	cfg, err := capture.ReadConfig(dir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.Market.Region != "asia" {
		t.Errorf("Market.Region = %q, want asia", cfg.Market.Region)
	}
}

func TestMarketSettings_PostInvalidRegionRejected(t *testing.T) {
	dir := t.TempDir()
	mux := newMarketSettingsTestMux(dir)

	body, _ := json.Marshal(map[string]any{"region": "mars"})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/market", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	cfg, err := capture.ReadConfig(dir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.Market.Region != "" {
		t.Errorf("Market.Region = %q, invalid region should not have been persisted", cfg.Market.Region)
	}
}

func TestMarketSettings_PostInvalidJsonRejected(t *testing.T) {
	dir := t.TempDir()
	mux := newMarketSettingsTestMux(dir)

	req := httptest.NewRequest(http.MethodPost, "/api/settings/market", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestMarketSettings_PostPreservesOtherConfigSections(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		CaptureInterfaces: []capture.PersistedInterface{{Name: "X"}},
		Hub:               capture.HubConfig{Enabled: true, URL: "http://hub.local:8090"},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newMarketSettingsTestMux(dir)

	body, _ := json.Marshal(map[string]any{"region": "americas"})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/market", bytes.NewReader(body))
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
	if !cfg.Hub.Enabled || cfg.Hub.URL != "http://hub.local:8090" {
		t.Errorf("Hub config changed unexpectedly: %+v", cfg.Hub)
	}
}
