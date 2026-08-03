package server

import (
	"net/http"

	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/adp"
	"github.com/nospy/albion-openradar/internal/capture"
)

// MarketSettingsAPI reads/writes this radar client's market-data region (which of the Data
// Project's regional API hosts to query when no Hub is configured/reachable - see
// internal/adp), following the same appDir/network.json-backed partial-update pattern as
// SettingsAPI's logging endpoint.
type MarketSettingsAPI struct {
	appDir string
}

func NewMarketSettingsAPI(appDir string) *MarketSettingsAPI {
	return &MarketSettingsAPI{appDir: appDir}
}

func (a *MarketSettingsAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/market", a.handleGet)
	mux.HandleFunc("POST /api/settings/market", a.handlePost)
}

func (a *MarketSettingsAPI) handleGet(w http.ResponseWriter, _ *http.Request) {
	cfg, err := capture.ReadConfig(a.appDir)
	if err != nil {
		http.Error(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg.Market.Region == "" {
		cfg.Market.Region = string(adp.RegionEurope)
	}
	writeJSON(w, http.StatusOK, cfg.Market)
}

type marketConfigPatch struct {
	Region *string `json:"region"`
}

func (a *MarketSettingsAPI) handlePost(w http.ResponseWriter, r *http.Request) {
	var patch marketConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if patch.Region != nil && !adp.IsValidRegion(*patch.Region) {
		http.Error(w, "region must be one of: europe, americas, asia", http.StatusBadRequest)
		return
	}

	var newMarket capture.MarketConfig
	if err := capture.MutateConfig(a.appDir, func(cfg *capture.Config) {
		if patch.Region != nil {
			cfg.Market.Region = *patch.Region
		}
		newMarket = cfg.Market
	}); err != nil {
		http.Error(w, "write config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, newMarket)
}
