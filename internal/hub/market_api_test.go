package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nospy/albion-openradar/internal/adp"
)

// newTestMarketAPI wires a MarketAPI whose adp.Client points at a fake public-API server, so
// tests control exactly what "the public API" returns without any real network access.
func newTestMarketAPI(t *testing.T, adpHandler http.HandlerFunc) (mux *http.ServeMux, adpCallCount *int) {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	callCount := 0
	wrapped := func(w http.ResponseWriter, r *http.Request) {
		callCount++
		adpHandler(w, r)
	}
	adpSrv := httptest.NewServer(http.HandlerFunc(wrapped))
	t.Cleanup(adpSrv.Close)

	adpClient := adp.NewClient(adp.RegionEurope)
	adpClient.BaseURL = adpSrv.URL

	api := NewMarketAPI(store, testSecret, adpClient)
	mux = http.NewServeMux()
	api.Register(mux)
	return mux, &callCount
}

func fakeADPResponse(entries []adp.PriceEntry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(entries)
	}
}

func TestMarketAPI_GetWithoutSecretRejected(t *testing.T) {
	mux, _ := newTestMarketAPI(t, fakeADPResponse(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/market/prices?items=T4_BAG", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

func TestMarketAPI_GetMissingItemsIsBadRequest(t *testing.T) {
	mux, _ := newTestMarketAPI(t, fakeADPResponse(nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/market/prices", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestMarketAPI_GetOnEmptyCacheFetchesFromADPAndCaches(t *testing.T) {
	entry := adp.PriceEntry{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 8499}
	mux, calls := newTestMarketAPI(t, fakeADPResponse([]adp.PriceEntry{entry}))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/market/prices?items=T4_BAG&locations=Caerleon&qualities=1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []adp.PriceEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 8499 {
		t.Errorf("got = %+v", got)
	}
	if *calls != 1 {
		t.Errorf("ADP calls = %d, want 1", *calls)
	}
}

func TestMarketAPI_GetSecondCallIsCacheHitNoADPCall(t *testing.T) {
	entry := adp.PriceEntry{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 8499}
	mux, calls := newTestMarketAPI(t, fakeADPResponse([]adp.PriceEntry{entry}))
	query := "/api/market/prices?items=T4_BAG&locations=Caerleon&qualities=1"

	mux.ServeHTTP(httptest.NewRecorder(), authedRequest(http.MethodGet, query, nil))
	if *calls != 1 {
		t.Fatalf("expected exactly 1 ADP call after the first request, got %d", *calls)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, query, nil))
	if *calls != 1 {
		t.Errorf("ADP calls = %d after second request, want still 1 (cache hit)", *calls)
	}
	var got []adp.PriceEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 8499 {
		t.Errorf("got = %+v", got)
	}
}

func TestMarketAPI_GetADPUnreachableFallsBackToStaleCache(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.UpsertPrices([]adp.PriceEntry{{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 1}}); err != nil {
		t.Fatalf("seed UpsertPrices: %v", err)
	}
	// Backdate it so the GET below sees it as stale and attempts an ADP refresh.
	if _, err := store.db.Exec(`UPDATE market_prices SET cached_at = '2000-01-01T00:00:00Z'`); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	adpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(adpSrv.Close)
	adpClient := adp.NewClient(adp.RegionEurope)
	adpClient.BaseURL = adpSrv.URL

	api := NewMarketAPI(store, testSecret, adpClient)
	mux := http.NewServeMux()
	api.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/market/prices?items=T4_BAG&locations=Caerleon&qualities=1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (degrade to stale cache), body=%s", rec.Code, rec.Body.String())
	}
	var got []adp.PriceEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 1 {
		t.Errorf("got = %+v, want the stale cached row", got)
	}
}

func TestMarketAPI_GetADPUnreachableAndNoCacheReturnsBadGateway(t *testing.T) {
	adpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(adpSrv.Close)
	store, err := OpenStore(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	adpClient := adp.NewClient(adp.RegionEurope)
	adpClient.BaseURL = adpSrv.URL

	api := NewMarketAPI(store, testSecret, adpClient)
	mux := http.NewServeMux()
	api.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/market/prices?items=T4_BAG&locations=Caerleon&qualities=1", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
}

func TestMarketAPI_GetDefaultsLocationsAndQualitiesWhenOmitted(t *testing.T) {
	entry := adp.PriceEntry{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 8499}
	mux, calls := newTestMarketAPI(t, fakeADPResponse([]adp.PriceEntry{entry}))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/market/prices?items=T4_BAG", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	if *calls != 1 {
		t.Errorf("expected the ADP fallback to be exercised for the default location/quality set")
	}
}

func TestMarketAPI_PostWithoutSecretRejected(t *testing.T) {
	mux, _ := newTestMarketAPI(t, fakeADPResponse(nil))
	body, _ := json.Marshal([]adp.PriceEntry{{ItemID: "T4_BAG", City: "Caerleon", Quality: 1}})
	req := httptest.NewRequest(http.MethodPost, "/api/market/prices", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

func TestMarketAPI_PostIngestsThenGetIsCacheHit(t *testing.T) {
	mux, calls := newTestMarketAPI(t, fakeADPResponse(nil))
	entry := adp.PriceEntry{ItemID: "T4_BAG", City: "Caerleon", Quality: 1, SellPriceMin: 8499}
	body, _ := json.Marshal([]adp.PriceEntry{entry})

	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, authedRequest(http.MethodPost, "/api/market/prices", body))
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST status %d, body=%s", postRec.Code, postRec.Body.String())
	}

	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, authedRequest(http.MethodGet, "/api/market/prices?items=T4_BAG&locations=Caerleon&qualities=1", nil))
	var got []adp.PriceEntry
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SellPriceMin != 8499 {
		t.Errorf("got = %+v", got)
	}
	if *calls != 0 {
		t.Errorf("ADP calls = %d, want 0 (POST already cached it)", *calls)
	}
}

func TestMarketAPI_PostMalformedBodyIsBadRequest(t *testing.T) {
	mux, _ := newTestMarketAPI(t, fakeADPResponse(nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodPost, "/api/market/prices", []byte("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}
