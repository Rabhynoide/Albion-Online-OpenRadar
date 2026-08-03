package hub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/nospy/albion-openradar/internal/adp"
)

// defaultCities/defaultQualities mirror what the public API itself returns when a query
// omits locations/qualities - applied here too so a request without either still gets (and
// caches) the full standard set, instead of the store's empty-dimension "nothing to do" guard
// silently returning nothing.
var defaultCities = []string{
	"Black Market", "Bridgewatch", "Caerleon", "Fort Sterling",
	"Lymhurst", "Martlock", "Thetford", "Brecilien",
}
var defaultQualities = []int{1, 2, 3, 4, 5}

// MarketAPI exposes the Hub's cached market-price database over HTTP, backed by the public
// Albion Online Data Project API for anything not already cached fresh. Every endpoint
// requires the SecretHeader, same as the roads API.
type MarketAPI struct {
	store     *Store
	secret    string
	adpClient *adp.Client
}

func NewMarketAPI(store *Store, secret string, adpClient *adp.Client) *MarketAPI {
	return &MarketAPI{store: store, secret: secret, adpClient: adpClient}
}

func (a *MarketAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/market/prices", requireSecret(a.secret, a.handleGet))
	mux.HandleFunc("POST /api/market/prices", requireSecret(a.secret, a.handlePost))
}

func (a *MarketAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	items := splitCSV(r.URL.Query().Get("items"))
	if len(items) == 0 {
		http.Error(w, "items is required", http.StatusBadRequest)
		return
	}
	cities := splitCSV(r.URL.Query().Get("locations"))
	if len(cities) == 0 {
		cities = defaultCities
	}
	qualities := splitCSVInts(r.URL.Query().Get("qualities"))
	if len(qualities) == 0 {
		qualities = defaultQualities
	}

	found, missing, err := a.store.GetPrices(items, cities, qualities)
	if err != nil {
		http.Error(w, "read cache: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(missing) == 0 {
		writeJSON(w, found)
		return
	}

	fresh, err := a.adpClient.FetchPrices(items, cities, qualities)
	if err != nil {
		// Public API unreachable - degrade to whatever we already had cached (even if stale)
		// rather than failing the whole request outright, if there's anything usable at all.
		if stale, staleErr := a.store.GetCachedPrices(items, cities, qualities); staleErr == nil && len(stale) > 0 {
			writeJSON(w, stale)
			return
		}
		http.Error(w, "fetch prices: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.store.UpsertPrices(fresh); err != nil {
		http.Error(w, "cache prices: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, fresh)
}

// observationRequest is the ingestion payload for client-submitted price observations (issue
// #23, Part B). Side matters: a client passively browsing the in-game marketplace only ever
// observes one side of the book at a time (AuctionGetOffers -> sell, AuctionGetRequests ->
// buy), so a plain full-row overwrite would wipe out whatever was already known about the
// other side. Side defaults to "both" (a full-row upsert, same as the original behavior)
// when omitted, for a caller that genuinely has complete data for every entry it submits.
type observationRequest struct {
	Side    string           `json:"side"`
	Entries []adp.PriceEntry `json:"entries"`
}

// handlePost accepts a batch of price observations and caches them - either a full overwrite
// (side "both"/omitted) or a side-preserving partial update (side "sell"/"buy").
func (a *MarketAPI) handlePost(w http.ResponseWriter, r *http.Request) {
	var body observationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	switch body.Side {
	case "sell":
		err = a.store.UpsertSellObservations(body.Entries)
	case "buy":
		err = a.store.UpsertBuyObservations(body.Entries)
	case "", "both":
		err = a.store.UpsertPrices(body.Entries)
	default:
		http.Error(w, `side must be "sell", "buy", "both", or omitted`, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "persist: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
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
