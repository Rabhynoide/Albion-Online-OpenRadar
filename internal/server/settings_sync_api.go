package server

import (
	"net/http"

	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/syncsettings"
)

// SettingsSyncAPI persists the frontend's browser-localStorage feature-toggle settings
// (web/scripts/utils/SettingsSync.js) to settings-sync.json - issue #21, so settings survive
// a browser data wipe and can be reused across machines. SettingsSync.js treats this as
// best-effort: localStorage stays the synchronous source of truth for reads, this is a
// write-through on every change plus a one-time hydration on startup.
type SettingsSyncAPI struct {
	appDir string
}

func NewSettingsSyncAPI(appDir string) *SettingsSyncAPI {
	return &SettingsSyncAPI{appDir: appDir}
}

func (a *SettingsSyncAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/sync", a.handleGet)
	mux.HandleFunc("POST /api/settings/sync", a.handlePost)
	mux.HandleFunc("DELETE /api/settings/sync", a.handleDelete)
}

func (a *SettingsSyncAPI) handleGet(w http.ResponseWriter, _ *http.Request) {
	settings, err := syncsettings.ReadAll(a.appDir)
	if err != nil {
		http.Error(w, "read settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

type settingEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (a *SettingsSyncAPI) handlePost(w http.ResponseWriter, r *http.Request) {
	var entry settingEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if entry.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if err := syncsettings.Set(a.appDir, entry.Key, entry.Value); err != nil {
		http.Error(w, "write settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *SettingsSyncAPI) handleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if err := syncsettings.Delete(a.appDir, key); err != nil {
		http.Error(w, "delete setting: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
