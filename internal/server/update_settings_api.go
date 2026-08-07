package server

import (
	"net/http"

	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/updatecheck"
)

// UpdateSettingsAPI exposes the result of the launch-time GitHub release check
// (cmd/radar's startUpdateCheck, cached in network.json's UpdateCheck section - see
// internal/updatecheck) to the frontend. It never makes an outbound call itself: "is an
// update available" is recomputed on every GET from the persisted LatestVersion against this
// server's own currentVersion, so a manual binary update or a fresh check result is always
// reflected without needing its own network round-trip per page load.
type UpdateSettingsAPI struct {
	appDir         string
	currentVersion string
}

func NewUpdateSettingsAPI(appDir, currentVersion string) *UpdateSettingsAPI {
	return &UpdateSettingsAPI{appDir: appDir, currentVersion: currentVersion}
}

func (a *UpdateSettingsAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/update", a.handleGet)
	mux.HandleFunc("POST /api/settings/update/dismiss", a.handleDismiss)
}

type updateStatus struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseURL     string `json:"releaseUrl"`
}

func (a *UpdateSettingsAPI) status(cfg capture.Config) updateStatus {
	available := updatecheck.IsNewer(a.currentVersion, cfg.UpdateCheck.LatestVersion) &&
		cfg.UpdateCheck.LatestVersion != cfg.UpdateCheck.DismissedVersion
	return updateStatus{
		Available:      available,
		CurrentVersion: a.currentVersion,
		LatestVersion:  cfg.UpdateCheck.LatestVersion,
		ReleaseURL:     cfg.UpdateCheck.ReleaseURL,
	}
}

func (a *UpdateSettingsAPI) handleGet(w http.ResponseWriter, _ *http.Request) {
	cfg, err := capture.ReadConfig(a.appDir)
	if err != nil {
		http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, a.status(cfg))
}

// handleDismiss marks the currently-known latest version as dismissed so it stops being
// reported as available - until a later check finds something newer still.
func (a *UpdateSettingsAPI) handleDismiss(w http.ResponseWriter, _ *http.Request) {
	var newCfg capture.Config
	if err := capture.MutateConfig(a.appDir, func(cfg *capture.Config) {
		cfg.UpdateCheck.DismissedVersion = cfg.UpdateCheck.LatestVersion
		newCfg = *cfg
	}); err != nil {
		http.Error(w, "write config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, a.status(newCfg))
}
