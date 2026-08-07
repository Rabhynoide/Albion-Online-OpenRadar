package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/capture"
)

func newUpdateSettingsTestMux(dir, currentVersion string) *http.ServeMux {
	api := NewUpdateSettingsAPI(dir, currentVersion)
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}

func TestUpdateSettings_GetOnEmptyConfigReportsNoUpdate(t *testing.T) {
	dir := t.TempDir()
	mux := newUpdateSettingsTestMux(dir, "1.0.2")

	req := httptest.NewRequest(http.MethodGet, "/api/settings/update", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var body updateStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Available {
		t.Errorf("Available = true on an empty config, want false")
	}
	if body.CurrentVersion != "1.0.2" {
		t.Errorf("CurrentVersion = %q, want 1.0.2", body.CurrentVersion)
	}
}

func TestUpdateSettings_GetReportsAvailableWhenLatestIsNewer(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		UpdateCheck: capture.UpdateCheckConfig{
			LatestVersion: "1.1.0",
			ReleaseURL:    "https://github.com/Rabhynoide/Albion-Online-OpenRadar/releases/tag/1.1.0",
			LastChecked:   time.Now(),
		},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newUpdateSettingsTestMux(dir, "1.0.2")

	req := httptest.NewRequest(http.MethodGet, "/api/settings/update", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body updateStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Available {
		t.Fatalf("Available = false, want true (1.1.0 > 1.0.2)")
	}
	if body.LatestVersion != "1.1.0" || body.ReleaseURL == "" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestUpdateSettings_GetReportsUnavailableWhenAlreadyUpToDate(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		UpdateCheck: capture.UpdateCheckConfig{LatestVersion: "1.0.2", LastChecked: time.Now()},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newUpdateSettingsTestMux(dir, "1.0.2")

	req := httptest.NewRequest(http.MethodGet, "/api/settings/update", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body updateStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Available {
		t.Errorf("Available = true when latest equals current, want false")
	}
}

func TestUpdateSettings_DismissPreventsAvailableForThatVersion(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		UpdateCheck: capture.UpdateCheckConfig{
			LatestVersion: "1.1.0",
			ReleaseURL:    "https://example.invalid/1.1.0",
			LastChecked:   time.Now(),
		},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mux := newUpdateSettingsTestMux(dir, "1.0.2")

	req := httptest.NewRequest(http.MethodPost, "/api/settings/update/dismiss", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body updateStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Available {
		t.Errorf("Available = true right after dismiss, want false")
	}

	cfg, err := capture.ReadConfig(dir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.UpdateCheck.DismissedVersion != "1.1.0" {
		t.Errorf("DismissedVersion = %q, want 1.1.0", cfg.UpdateCheck.DismissedVersion)
	}

	// A subsequent GET (simulating a page reload) must still report no update.
	req2 := httptest.NewRequest(http.MethodGet, "/api/settings/update", http.NoBody)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	var body2 updateStatus
	if err := json.NewDecoder(rec2.Body).Decode(&body2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body2.Available {
		t.Errorf("Available = true on a later GET after dismiss, want false (dismissal should persist)")
	}
}

func TestUpdateSettings_NewerCheckAfterDismissBecomesAvailableAgain(t *testing.T) {
	dir := t.TempDir()
	if err := capture.WriteConfig(dir, capture.Config{
		UpdateCheck: capture.UpdateCheckConfig{
			LatestVersion:    "1.1.0",
			DismissedVersion: "1.1.0",
			LastChecked:      time.Now(),
		},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	// Simulate a later background check finding an even newer release.
	if err := capture.MutateConfig(dir, func(cfg *capture.Config) {
		cfg.UpdateCheck.LatestVersion = "1.2.0"
	}); err != nil {
		t.Fatalf("mutate config: %v", err)
	}

	mux := newUpdateSettingsTestMux(dir, "1.0.2")
	req := httptest.NewRequest(http.MethodGet, "/api/settings/update", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body updateStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Available {
		t.Errorf("Available = false, want true - 1.2.0 was never dismissed, only 1.1.0 was")
	}
}
