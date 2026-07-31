package hub

import (
	"encoding/json"
	"net/http"
)

// API exposes the Hub's shared road-edge database over HTTP. Every endpoint except
// /health requires the SecretHeader to match the Hub's configured secret.
type API struct {
	store  *Store
	secret string
}

func NewAPI(store *Store, secret string) *API {
	return &API{store: store, secret: secret}
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("GET /api/roads/edges", requireSecret(a.secret, a.handleList))
	mux.HandleFunc("POST /api/roads/edges", requireSecret(a.secret, a.handleAdd))
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleList(w http.ResponseWriter, _ *http.Request) {
	edges, err := a.store.ListEdges()
	if err != nil {
		http.Error(w, "list: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, edges)
}

type addEdgeBody struct {
	From string      `json:"from"`
	To   string      `json:"to"`
	Pos  *[2]float64 `json:"pos"`
}

func (a *API) handleAdd(w http.ResponseWriter, r *http.Request) {
	var body addEdgeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.From == "" || body.To == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}
	if err := a.store.UpsertEdge(body.From, body.To, body.Pos); err != nil {
		http.Error(w, "persist: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
