// Package settingsui is the native Fyne settings window (cmd/radar -settings) - a Go port of
// the configuration pages in internal/templates/pages/*.gohtml + their web/scripts JS, so that
// configuring OpenRadar no longer requires a browser. It replaces the TUI dashboard the same way
// internal/overlay replaces it in -overlay mode; unlike the overlay, this process keeps running
// the HTTP/WebSocket server itself (it still needs to serve the Market page, the only web page
// left once this phase's pages are removed) and launches the map overlay as a separate child
// process on demand - see run.go's doc comment for why the two can't share one process.
package settingsui

import (
	"strconv"

	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/logger"
	"github.com/nospy/albion-openradar/internal/syncsettings"
)

// Store is a thin Go equivalent of web/scripts/utils/SettingsSync.js's typed accessors, over
// the same settings-sync.json file (internal/syncsettings) the web app and internal/overlay's
// read-only settingsPanel already use. Every read re-reads the file rather than caching, so a
// change made in the overlay's read-only view or a still-running web page (Market) is always
// reflected - same "always-fresh" reasoning as cmd/radar/overlay_setup.go's readSettingBool.
type Store struct {
	appDir string
}

func NewStore(appDir string) *Store {
	return &Store{appDir: appDir}
}

func (s *Store) GetBool(key string, def bool) bool {
	settings, err := syncsettings.ReadAll(s.appDir)
	if err != nil {
		return def
	}
	v, ok := settings[key]
	if !ok {
		return def
	}
	return v == "true"
}

func (s *Store) SetBool(key string, value bool) {
	if err := syncsettings.Set(s.appDir, key, strconv.FormatBool(value)); err != nil {
		logger.PrintWarn("SETTINGS", "write %s failed: %v", key, err)
	}
}

func (s *Store) GetString(key, def string) string {
	settings, err := syncsettings.ReadAll(s.appDir)
	if err != nil {
		return def
	}
	v, ok := settings[key]
	if !ok || v == "" {
		return def
	}
	return v
}

func (s *Store) SetString(key, value string) {
	if err := syncsettings.Set(s.appDir, key, value); err != nil {
		logger.PrintWarn("SETTINGS", "write %s failed: %v", key, err)
	}
}

func (s *Store) GetNumber(key string, def float64) float64 {
	settings, err := syncsettings.ReadAll(s.appDir)
	if err != nil {
		return def
	}
	v, ok := settings[key]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func (s *Store) SetNumber(key string, value float64) {
	if err := syncsettings.Set(s.appDir, key, strconv.FormatFloat(value, 'f', -1, 64)); err != nil {
		logger.PrintWarn("SETTINGS", "write %s failed: %v", key, err)
	}
}

// GetJSON decodes a JSON-encoded value into out, leaving out untouched (caller-supplied zero
// value/default stands) if the key is absent or unparsable.
// All returns every persisted key/value pair - used by the Settings page's debug-data export.
func (s *Store) All() (map[string]string, error) {
	return syncsettings.ReadAll(s.appDir)
}

func (s *Store) GetJSON(key string, out any) {
	settings, err := syncsettings.ReadAll(s.appDir)
	if err != nil {
		return
	}
	raw, ok := settings[key]
	if !ok {
		return
	}
	_ = json.Unmarshal([]byte(raw), out)
}

func (s *Store) SetJSON(key string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		logger.PrintWarn("SETTINGS", "marshal %s failed: %v", key, err)
		return
	}
	if err := syncsettings.Set(s.appDir, key, string(data)); err != nil {
		logger.PrintWarn("SETTINGS", "write %s failed: %v", key, err)
	}
}

// Clear deletes every persisted key - the native "Reset Settings" action. Unlike the web
// Settings page's "Reset Settings" button (which only clears browser localStorage, never
// settings-sync.json - a gap noted while planning this package), this actually resets the
// shared file every process reads from.
func (s *Store) Clear() {
	settings, err := syncsettings.ReadAll(s.appDir)
	if err != nil {
		return
	}
	for key := range settings {
		if err := syncsettings.Delete(s.appDir, key); err != nil {
			logger.PrintWarn("SETTINGS", "delete %s failed: %v", key, err)
		}
	}
}
