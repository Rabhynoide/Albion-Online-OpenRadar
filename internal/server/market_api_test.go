package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nospy/albion-openradar/internal/adp"
	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/hub"
)

func newMarketTestMux(api *MarketAPI) *http.ServeMux {
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}

// fakeMarketHub stands in for a real Hub instance, using the real internal/hub market API
// (backed by a fake public-API server) so the wire contract is exercised end to end.
func fakeMarketHub(t *testing.T, secret string, entries []adp.PriceEntry) *httptest.Server {
	t.Helper()
	adpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(entries)
	}))
	t.Cleanup(adpSrv.Close)
	adpClient := adp.NewClient(adp.RegionEurope)
	adpClient.BaseURL = adpSrv.URL

	store, err := hub.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	api := hub.NewMarketAPI(store, secret, adpClient)
	mux := http.NewServeMux()
	api.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func withMarketConfig(t *testing.T, dir string, cfg capture.Config) {
	t.Helper()
	if err := capture.WriteConfig(dir, cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
}

func TestMarketAPI_MissingItemsIsBadRequest(t *testing.T) {
	api := NewMarketAPI(t.TempDir())
	mux := newMarketTestMux(api)

	req := httptest.NewRequest(http.MethodGet, "/api/market/prices", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestMarketAPI_PrefersHubWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	hubEntry := adp.PriceEntry{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 111}
	hubSrv := fakeMarketHub(t, "secret", []adp.PriceEntry{hubEntry})
	withMarketConfig(t, dir, capture.Config{Hub: capture.HubConfig{Enabled: true, URL: hubSrv.URL, Secret: "secret"}})

	api := NewMarketAPI(dir)
	mux := newMarketTestMux(api)
	req := httptest.NewRequest(http.MethodGet, "/api/market/prices?items=T4_BAG&locations=Caerleon&qualities=1", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []adp.PriceEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 111 {
		t.Fatalf("expected the Hub's price, got %+v", got)
	}
}

func TestMarketAPI_FallsBackToDirectFetchWhenHubUnreachable(t *testing.T) {
	dir := t.TempDir()
	withMarketConfig(t, dir, capture.Config{Hub: capture.HubConfig{Enabled: true, URL: "http://127.0.0.1:1", Secret: "secret"}})

	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]adp.PriceEntry{{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 222}})
	}))
	t.Cleanup(directSrv.Close)

	api := NewMarketAPI(dir)
	api.directFetchBaseURL = directSrv.URL
	mux := newMarketTestMux(api)
	req := httptest.NewRequest(http.MethodGet, "/api/market/prices?items=T4_BAG&locations=Caerleon&qualities=1", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []adp.PriceEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 222 {
		t.Fatalf("expected the direct-fetch fallback price, got %+v", got)
	}
}

func TestMarketAPI_NoHubConfiguredUsesDirectFetch(t *testing.T) {
	dir := t.TempDir() // Hub.Enabled defaults to false - no config written at all.

	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]adp.PriceEntry{{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 333}})
	}))
	t.Cleanup(directSrv.Close)

	api := NewMarketAPI(dir)
	api.directFetchBaseURL = directSrv.URL
	mux := newMarketTestMux(api)
	req := httptest.NewRequest(http.MethodGet, "/api/market/prices?items=T4_BAG", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []adp.PriceEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 333 {
		t.Fatalf("got = %+v", got)
	}
}

func TestMarketAPI_DirectFetchUnreachableReturnsBadGateway(t *testing.T) {
	dir := t.TempDir()

	api := NewMarketAPI(dir)
	api.directFetchBaseURL = "http://127.0.0.1:1"
	mux := newMarketTestMux(api)
	req := httptest.NewRequest(http.MethodGet, "/api/market/prices?items=T4_BAG", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
}

func TestMarketAPI_DirectFetchSecondIdenticalRequestIsCacheHit(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode([]adp.PriceEntry{{ItemID: "T4_BAG", SellPriceMin: 444}})
	}))
	t.Cleanup(directSrv.Close)

	api := NewMarketAPI(dir)
	api.directFetchBaseURL = directSrv.URL
	mux := newMarketTestMux(api)
	query := "/api/market/prices?items=T4_BAG"

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, query, http.NoBody))
	if calls != 1 {
		t.Fatalf("calls after first request = %d, want 1", calls)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, query, http.NoBody))
	if calls != 1 {
		t.Errorf("calls after second request = %d, want still 1 (cache hit)", calls)
	}
	var got []adp.PriceEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 444 {
		t.Errorf("got = %+v", got)
	}
}

func TestMarketAPI_DirectFetchCachesIdenticalQuery(t *testing.T) {
	// Exercises the in-memory cache directly (bypassing HTTP) since MarketAPI's direct path
	// has no seam for injecting a fake ADP server - the cache methods themselves are the
	// unit under test here.
	api := NewMarketAPI(t.TempDir())
	entries := []adp.PriceEntry{{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 42}}

	api.cacheSet("items=T4_BAG", entries)
	got, ok := api.cacheGet("items=T4_BAG")
	if !ok {
		t.Fatal("expected a cache hit for the same query")
	}
	if len(got) != 1 || got[0].SellPriceMin != 42 {
		t.Errorf("got = %+v", got)
	}
}

func TestMarketAPI_CacheExpiresAfterTTL(t *testing.T) {
	api := NewMarketAPI(t.TempDir())
	api.cache["items=T4_BAG"] = cachedMarketEntries{
		entries:   []adp.PriceEntry{{ItemID: "T4_BAG"}},
		expiresAt: time.Now().Add(-time.Second), // already expired
	}

	_, ok := api.cacheGet("items=T4_BAG")
	if ok {
		t.Error("expected a cache miss for an expired entry")
	}
}

func TestMarketAPI_CacheMissForUnknownQuery(t *testing.T) {
	api := NewMarketAPI(t.TempDir())

	_, ok := api.cacheGet("items=NEVER_SET")
	if ok {
		t.Error("expected a cache miss for a query never cached")
	}
}
