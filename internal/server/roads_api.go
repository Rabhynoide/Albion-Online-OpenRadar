package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/hub"
	"github.com/nospy/albion-openradar/internal/roads"
)

// hubRequestTimeout bounds how long a Hub round-trip can hold up a browser request
// before RoadsAPI falls back to (GET) or gives up forwarding to (POST) the local store.
const hubRequestTimeout = 3 * time.Second

// RoadsAPI exposes runtime-discovered zone connections (Avalonian Roads and similar
// non-static exits) so the frontend GPS graph can merge them into its pathfinding data
// and persist them across sessions. When a Hub is configured (see internal/hub), this
// backend relays to it, keeping the local roads.json as an offline fallback/cache -
// the browser-facing API shape never changes.
type RoadsAPI struct {
	appDir     string
	httpClient *http.Client
}

func NewRoadsAPI(appDir string) *RoadsAPI {
	return &RoadsAPI{
		appDir:     appDir,
		httpClient: &http.Client{Timeout: hubRequestTimeout},
	}
}

func (a *RoadsAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/roads/edges", a.handleList)
	mux.HandleFunc("POST /api/roads/edges", a.handleAdd)
}

func (a *RoadsAPI) handleList(w http.ResponseWriter, _ *http.Request) {
	cfg, _ := capture.ReadConfig(a.appDir)
	if cfg.Hub.Enabled {
		if edges, ok := a.fetchHubEdges(cfg.Hub); ok {
			writeJSON(w, http.StatusOK, edges)
			return
		}
	}

	store, err := roads.ReadStore(a.appDir)
	if err != nil {
		http.Error(w, "read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, store.Edges)
}

// fetchHubEdges tries the configured Hub, returning ok=false on any failure so the
// caller can fall back to the local store instead of surfacing an error to the browser.
func (a *RoadsAPI) fetchHubEdges(cfg capture.HubConfig) ([]roads.Edge, bool) {
	req, err := http.NewRequest(http.MethodGet, cfg.URL+"/api/roads/edges", http.NoBody)
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

	var edges []roads.Edge
	if err := json.NewDecoder(resp.Body).Decode(&edges); err != nil {
		return nil, false
	}
	return edges, true
}

type addEdgeBody struct {
	From string      `json:"from"`
	To   string      `json:"to"`
	Pos  *[2]float64 `json:"pos"`
}

func (a *RoadsAPI) handleAdd(w http.ResponseWriter, r *http.Request) {
	var body addEdgeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.From == "" || body.To == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}
	if err := roads.MutateStore(a.appDir, func(s *roads.Store) {
		roads.AddEdge(s, body.From, body.To, body.Pos)
	}); err != nil {
		http.Error(w, "persist: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if cfg, _ := capture.ReadConfig(a.appDir); cfg.Hub.Enabled {
		a.forwardToHub(cfg.Hub, body)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// forwardToHub best-effort mirrors a locally-persisted edge to the configured Hub.
// Failures are not surfaced to the browser: the local write already succeeded, and
// the next successful GET (local or Hub) will still reflect this edge.
func (a *RoadsAPI) forwardToHub(cfg capture.HubConfig, body addEdgeBody) {
	payload, err := json.Marshal(body)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, cfg.URL+"/api/roads/edges", bytes.NewReader(payload))
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
