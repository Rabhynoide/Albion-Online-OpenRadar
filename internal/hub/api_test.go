package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

const testSecret = "group-secret"

func newTestAPI(t *testing.T) *http.ServeMux {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	api := NewAPI(store, testSecret)
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}

func authedRequest(method, path string, body []byte) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set(SecretHeader, testSecret)
	return r
}

func TestAPI_HealthNoAuthRequired(t *testing.T) {
	mux := newTestAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAPI_ListWithoutSecretRejected(t *testing.T) {
	mux := newTestAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

func TestAPI_ListWithWrongSecretRejected(t *testing.T) {
	mux := newTestAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/roads/edges", nil)
	req.Header.Set(SecretHeader, "wrong")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

func TestAPI_ListEmpty(t *testing.T) {
	mux := newTestAPI(t)
	req := authedRequest(http.MethodGet, "/api/roads/edges", nil)
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

func TestAPI_PostThenGetReflectsEdge(t *testing.T) {
	mux := newTestAPI(t)

	body, _ := json.Marshal(map[string]any{"from": "4206", "to": "TNL-001", "pos": []float64{1.5, -2.5}})
	req := authedRequest(http.MethodPost, "/api/roads/edges", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status %d, body=%s", rec.Code, rec.Body.String())
	}

	req2 := authedRequest(http.MethodGet, "/api/roads/edges", nil)
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
}

func TestAPI_PostWithoutSecretRejected(t *testing.T) {
	mux := newTestAPI(t)
	body, _ := json.Marshal(map[string]any{"from": "A", "to": "B"})
	req := httptest.NewRequest(http.MethodPost, "/api/roads/edges", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

func TestAPI_PostMissingFromOrTo(t *testing.T) {
	mux := newTestAPI(t)
	body, _ := json.Marshal(map[string]any{"from": "", "to": "TNL-001"})
	req := authedRequest(http.MethodPost, "/api/roads/edges", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestAPI_PostMalformedBody(t *testing.T) {
	mux := newTestAPI(t)
	req := authedRequest(http.MethodPost, "/api/roads/edges", []byte("{not json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid body") {
		t.Errorf("body should mention invalid body: %s", rec.Body.String())
	}
}

func TestAPI_PostUpsertsExistingEdge(t *testing.T) {
	mux := newTestAPI(t)

	first, _ := json.Marshal(map[string]any{"from": "A", "to": "B"})
	mux.ServeHTTP(httptest.NewRecorder(), authedRequest(http.MethodPost, "/api/roads/edges", first))

	second, _ := json.Marshal(map[string]any{"from": "A", "to": "B", "pos": []float64{9, 9}})
	mux.ServeHTTP(httptest.NewRecorder(), authedRequest(http.MethodPost, "/api/roads/edges", second))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/roads/edges", nil))
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected upsert to keep a single edge, got %+v", got)
	}
}
