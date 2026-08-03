package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nospy/albion-openradar/internal/adp"
	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/hub"
)

// directFetchCacheTTL bounds how long a direct (no-Hub) public-API response is reused for an
// identical query, so re-rendering the same search doesn't hammer the public API. The Hub
// path needs no equivalent cache here - the Hub already caches server-side (see
// internal/hub's market_store.go staleness window).
const directFetchCacheTTL = 60 * time.Second

type cachedMarketEntries struct {
	entries   []adp.PriceEntry
	expiresAt time.Time
}

// MarketAPI exposes item price lookups to the frontend. When a Hub is configured (see
// internal/hub), this backend relays to its /api/market/prices endpoint (which itself
// caches and falls back to the public Data Project API server-side). When no Hub is
// configured, or the Hub is unreachable, this backend queries the public API directly using
// the client's own configured region - the browser-facing API shape never changes.
type MarketAPI struct {
	appDir     string
	httpClient *http.Client

	mu    sync.Mutex
	cache map[string]cachedMarketEntries

	// directFetchBaseURL overrides the public API's base URL for the no-Hub/Hub-unreachable
	// fallback path. Empty in production (the real per-region host is used); tests set it to
	// an httptest.NewServer standing in for the public API.
	directFetchBaseURL string
}

func NewMarketAPI(appDir string) *MarketAPI {
	return &MarketAPI{
		appDir:     appDir,
		httpClient: &http.Client{Timeout: hubRequestTimeout},
		cache:      make(map[string]cachedMarketEntries),
	}
}

func (a *MarketAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/market/prices", a.handleGet)
	mux.HandleFunc("POST /api/market/observations", a.handlePostObservations)
}

// observationRequest mirrors internal/hub's own observationRequest wire shape (kept as a
// separate local type rather than exported/shared - it's two fields, not worth coupling this
// package to internal/hub's internals for).
type observationRequest struct {
	Side    string           `json:"side"`
	Entries []adp.PriceEntry `json:"entries"`
}

// handlePostObservations relays prices the player's own client observed while browsing the
// in-game marketplace (issue #23, Part B - see web/scripts/handlers/MarketHandler.js) to the
// configured Hub. Best-effort and silent: without a Hub there's nowhere for these to go (the
// public API is read-only), so this always responds 204 regardless of whether a Hub is
// configured or whether the forward actually succeeded - same philosophy as
// RoadsAPI.forwardToHub, a contribution failing must never surface as a browser-visible error.
func (a *MarketAPI) handlePostObservations(w http.ResponseWriter, r *http.Request) {
	var body observationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if cfg, _ := capture.ReadConfig(a.appDir); cfg.Hub.Enabled {
		a.forwardObservations(cfg.Hub, body)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *MarketAPI) forwardObservations(cfg capture.HubConfig, body observationRequest) {
	payload, err := json.Marshal(body)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, cfg.URL+"/api/market/prices", bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(hub.SecretHeader, cfg.Secret)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (a *MarketAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	items := splitCSV(r.URL.Query().Get("items"))
	if len(items) == 0 {
		http.Error(w, "items is required", http.StatusBadRequest)
		return
	}
	cities := splitCSV(r.URL.Query().Get("locations"))
	qualities := splitCSVInts(r.URL.Query().Get("qualities"))

	cfg, _ := capture.ReadConfig(a.appDir)
	if cfg.Hub.Enabled {
		if entries, ok := a.fetchHubPrices(cfg.Hub, items, cities, qualities); ok {
			writeJSON(w, http.StatusOK, entries)
			return
		}
	}

	query := r.URL.RawQuery
	if cached, ok := a.cacheGet(query); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	region := adp.Region(cfg.Market.Region)
	client := adp.NewClient(region)
	if a.directFetchBaseURL != "" {
		client.BaseURL = a.directFetchBaseURL
	}
	entries, err := client.FetchPrices(items, cities, qualities)
	if err != nil {
		http.Error(w, "fetch prices: "+err.Error(), http.StatusBadGateway)
		return
	}
	a.cacheSet(query, entries)
	writeJSON(w, http.StatusOK, entries)
}

// fetchHubPrices tries the configured Hub, returning ok=false on any failure so the caller
// can fall back to the direct public-API path instead of surfacing an error to the browser.
// The query is rebuilt from already-parsed values (not forwarded verbatim from the incoming
// request) so untrusted client input never flows directly into an outbound request URL.
func (a *MarketAPI) fetchHubPrices(cfg capture.HubConfig, items, cities []string, qualities []int) ([]adp.PriceEntry, bool) {
	q := url.Values{}
	q.Set("items", strings.Join(items, ","))
	if len(cities) > 0 {
		q.Set("locations", strings.Join(cities, ","))
	}
	if len(qualities) > 0 {
		qs := make([]string, len(qualities))
		for i, quality := range qualities {
			qs[i] = strconv.Itoa(quality)
		}
		q.Set("qualities", strings.Join(qs, ","))
	}

	req, err := http.NewRequest(http.MethodGet, cfg.URL+"/api/market/prices?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, false
	}
	req.Header.Set(hub.SecretHeader, cfg.Secret)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	var entries []adp.PriceEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false
	}
	return entries, true
}

func (a *MarketAPI) cacheGet(key string) ([]adp.PriceEntry, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cached, ok := a.cache[key]
	if !ok || time.Now().After(cached.expiresAt) {
		return nil, false
	}
	return cached.entries, true
}

func (a *MarketAPI) cacheSet(key string, entries []adp.PriceEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cache[key] = cachedMarketEntries{entries: entries, expiresAt: time.Now().Add(directFetchCacheTTL)}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitCSVInts(s string) []int {
	parts := splitCSV(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	return out
}
