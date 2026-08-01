package server

import (
	"net/http"

	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/capture"
)

// HubSettingsAPI reads/writes this radar client's connection to a self-hosted
// OpenRadar Hub (see internal/hub), following the same appDir/network.json-backed
// partial-update pattern as SettingsAPI's logging endpoint. It also exposes a
// lightweight status check so the frontend can show a "connected to the Hub" indicator
// without ever talking to the Hub directly itself (same backend-relay reasoning as
// RoadsAPI's proxying).
type HubSettingsAPI struct {
	appDir     string
	httpClient *http.Client
}

func NewHubSettingsAPI(appDir string) *HubSettingsAPI {
	return &HubSettingsAPI{
		appDir:     appDir,
		httpClient: &http.Client{Timeout: hubRequestTimeout},
	}
}

func (a *HubSettingsAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/hub", a.handleGet)
	mux.HandleFunc("POST /api/settings/hub", a.handlePost)
	mux.HandleFunc("GET /api/hub/status", a.handleStatus)
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

type hubStatus struct {
	Enabled   bool `json:"enabled"`
	Connected bool `json:"connected"`
}

// handleStatus reports whether a Hub is configured and, if so, whether its unauthenticated
// /health endpoint currently responds. A short timeout (hubRequestTimeout) keeps a slow or
// dead Hub from holding up the frontend's polling.
func (a *HubSettingsAPI) handleStatus(w http.ResponseWriter, _ *http.Request) {
	cfg, err := capture.ReadConfig(a.appDir)
	if err != nil {
		http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !cfg.Hub.Enabled {
		writeJSON(w, http.StatusOK, hubStatus{Enabled: false})
		return
	}

	connected := false
	if resp, err := a.httpClient.Get(cfg.Hub.URL + "/health"); err == nil {
		connected = resp.StatusCode == http.StatusOK
		resp.Body.Close()
	}
	writeJSON(w, http.StatusOK, hubStatus{Enabled: true, Connected: connected})
}
