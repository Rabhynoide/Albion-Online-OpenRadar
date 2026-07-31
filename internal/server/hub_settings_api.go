package server

import (
	"net/http"

	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/capture"
)

// HubSettingsAPI reads/writes this radar client's connection to a self-hosted
// OpenRadar Hub (see internal/hub), following the same appDir/network.json-backed
// partial-update pattern as SettingsAPI's logging endpoint.
type HubSettingsAPI struct {
	appDir string
}

func NewHubSettingsAPI(appDir string) *HubSettingsAPI {
	return &HubSettingsAPI{appDir: appDir}
}

func (a *HubSettingsAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/hub", a.handleGet)
	mux.HandleFunc("POST /api/settings/hub", a.handlePost)
}

func (a *HubSettingsAPI) handleGet(w http.ResponseWriter, _ *http.Request) {
	cfg, err := capture.ReadConfig(a.appDir)
	if err != nil {
		http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cfg.Hub)
}

type hubConfigPatch struct {
	Enabled *bool   `json:"enabled"`
	URL     *string `json:"url"`
	Secret  *string `json:"secret"`
}

func (a *HubSettingsAPI) handlePost(w http.ResponseWriter, r *http.Request) {
	var patch hubConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var newHub capture.HubConfig
	if err := capture.MutateConfig(a.appDir, func(cfg *capture.Config) {
		if patch.Enabled != nil {
			cfg.Hub.Enabled = *patch.Enabled
		}
		if patch.URL != nil {
			cfg.Hub.URL = *patch.URL
		}
		if patch.Secret != nil {
			cfg.Hub.Secret = *patch.Secret
		}
		newHub = cfg.Hub
	}); err != nil {
		http.Error(w, "write config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, newHub)
}
