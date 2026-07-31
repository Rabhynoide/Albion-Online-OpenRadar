package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/hub"
)

func newRoadsTestMux(api *RoadsAPI) *http.ServeMux {
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}

func TestRoadsAPI_ListEmpty(t *testing.T) {
	api := NewRoadsAPI(t.TempDir())
	mux := newRoadsTestMux(api)

	req := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no edges, got %+v", got)
	}
}

func TestRoadsAPI_PostThenGetReflectsEdge(t *testing.T) {
	dir := t.TempDir()
	api := NewRoadsAPI(dir)
	mux := newRoadsTestMux(api)

	body, _ := json.Marshal(map[string]any{"from": "4206", "to": "TNL-001", "pos": []float64{1.5, -2.5}})
	req := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status %d, body=%s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	var got []map[string]any
	if err := json.NewDecoder(rec2.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 edge, got %+v", got)
	}
	if got[0]["from"] != "4206" || got[0]["to"] != "TNL-001" {
		t.Errorf("edge fields wrong: %+v", got[0])
	}
	if got[0]["discoveredAt"] == nil {
		t.Error("discoveredAt missing")
	}
}

func TestRoadsAPI_PostMissingFromOrTo(t *testing.T) {
	api := NewRoadsAPI(t.TempDir())
	mux := newRoadsTestMux(api)

	body, _ := json.Marshal(map[string]any{"from": "", "to": "TNL-001"})
	req := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestRoadsAPI_PostMalformedBody(t *testing.T) {
	api := NewRoadsAPI(t.TempDir())
	mux := newRoadsTestMux(api)

	req := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid body") {
		t.Errorf("body should mention invalid body: %s", rec.Body.String())
	}
}

func TestRoadsAPI_PostUpsertsExistingEdge(t *testing.T) {
	dir := t.TempDir()
	api := NewRoadsAPI(dir)
	mux := newRoadsTestMux(api)

	first, _ := json.Marshal(map[string]any{"from": "A", "to": "B"})
	req1 := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(first))
	mux.ServeHTTP(httptest.NewRecorder(), req1)

	second, _ := json.Marshal(map[string]any{"from": "A", "to": "B", "pos": []float64{9, 9}})
	req2 := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(second))
	mux.ServeHTTP(httptest.NewRecorder(), req2)

	req3 := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	var got []map[string]any
	if err := json.NewDecoder(rec3.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected upsert to keep a single edge, got %+v", got)
	}
}

func TestRoadsAPI_ListGETOnlyRejectsPOST(t *testing.T) {
	api := NewRoadsAPI(t.TempDir())
	mux := newRoadsTestMux(api)

	req := httptest.NewRequest(http.MethodPut, "/api/roads/edges", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rec.Code)
	}
}

// fakeHub stands in for a real Hub instance in proxy tests, using the real
// internal/hub auth+edge-store shape so the wire contract is exercised, not mocked.
func fakeHub(t *testing.T, secret string) *httptest.Server {
	t.Helper()
	store, err := hub.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	api := hub.NewAPI(store, secret)
	mux := http.NewServeMux()
	api.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func withHubConfig(t *testing.T, dir string, cfg capture.HubConfig) {
	t.Helper()
	if err := capture.WriteConfig(dir, capture.Config{Hub: cfg}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
}

func TestRoadsAPI_ListPrefersHubWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	hubSrv := fakeHub(t, "secret")
	withHubConfig(t, dir, capture.HubConfig{Enabled: true, URL: hubSrv.URL, Secret: "secret"})

	// Seed the Hub directly (not the local store) to prove the response comes from it.
	hubReq, _ := http.NewRequest(http.MethodPost, hubSrv.URL+"/api/roads/edges", bytes.NewReader(
		mustJSON(t, map[string]any{"from": "HUB-A", "to": "HUB-B"})))
	hubReq.Header.Set(hub.SecretHeader, "secret")
	hubReq.Header.Set("Content-Type", "application/json")
	if _, err := http.DefaultClient.Do(hubReq); err != nil {
		t.Fatalf("seed hub: %v", err)
	}

	api := NewRoadsAPI(dir)
	mux := newRoadsTestMux(api)
	req := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0]["from"] != "HUB-A" {
		t.Fatalf("expected Hub edge, got %+v", got)
	}
}

func TestRoadsAPI_ListFallsBackToLocalWhenHubUnreachable(t *testing.T) {
	dir := t.TempDir()
	withHubConfig(t, dir, capture.HubConfig{Enabled: true, URL: "http://127.0.0.1:1", Secret: "secret"})

	api := NewRoadsAPI(dir)
	mux := newRoadsTestMux(api)

	// Write directly to the local store first so we have something to fall back to.
	addBody := mustJSON(t, map[string]any{"from": "LOCAL-A", "to": "LOCAL-B"})
	// Bypass POST (which would itself try to forward to the unreachable Hub - fine,
	// it's best-effort, but we only want to test GET fallback here) by writing the
	// local store through the same handler the app uses.
	postReq := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(addBody))
	mux.ServeHTTP(httptest.NewRecorder(), postReq)

	req := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (fallback should succeed)", rec.Code)
	}

	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0]["from"] != "LOCAL-A" {
		t.Fatalf("expected local fallback edge, got %+v", got)
	}
}

func TestRoadsAPI_PostForwardsToHub(t *testing.T) {
	dir := t.TempDir()
	hubSrv := fakeHub(t, "secret")
	withHubConfig(t, dir, capture.HubConfig{Enabled: true, URL: hubSrv.URL, Secret: "secret"})

	api := NewRoadsAPI(dir)
	mux := newRoadsTestMux(api)

	body := mustJSON(t, map[string]any{"from": "A", "to": "B"})
	req := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status %d", rec.Code)
	}

	// Query the Hub directly to confirm the edge was forwarded.
	hubReq, _ := http.NewRequest(http.MethodGet, hubSrv.URL+"/api/roads/edges", nil)
	hubReq.Header.Set(hub.SecretHeader, "secret")
	resp, err := http.DefaultClient.Do(hubReq)
	if err != nil {
		t.Fatalf("query hub: %v", err)
	}
	defer resp.Body.Close()
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0]["from"] != "A" {
		t.Fatalf("expected edge forwarded to hub, got %+v", got)
	}
}

func TestRoadsAPI_PostSucceedsLocallyEvenWhenHubUnreachable(t *testing.T) {
	dir := t.TempDir()
	withHubConfig(t, dir, capture.HubConfig{Enabled: true, URL: "http://127.0.0.1:1", Secret: "secret"})

	api := NewRoadsAPI(dir)
	mux := newRoadsTestMux(api)

	body := mustJSON(t, map[string]any{"from": "A", "to": "B"})
	req := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status %d, want 200 even though hub is unreachable", rec.Code)
	}
}

func TestRoadsAPI_HubDisabledBehavesLikeToday(t *testing.T) {
	dir := t.TempDir()
	withHubConfig(t, dir, capture.HubConfig{Enabled: false, URL: "http://127.0.0.1:1", Secret: "secret"})

	api := NewRoadsAPI(dir)
	mux := newRoadsTestMux(api)

	req := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty local store, got %+v", got)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
