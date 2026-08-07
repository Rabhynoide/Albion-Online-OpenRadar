// Package syncsettings persists the frontend's browser-localStorage feature-toggle settings
// (web/scripts/utils/SettingsSync.js) to a file, so they survive a browser data wipe and can
// be reused across machines. Unlike capture.Config, which holds strictly-typed radar/network
// settings, this is a flat, loosely-typed key-value store mirroring exactly what
// SettingsSync.js keeps in localStorage - every value is already a string there, including
// JSON-encoded values like the ignore list array.
package syncsettings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const filename = "settings-sync.json"

// ReadAll returns every persisted key/value pair, or an empty map if the file doesn't exist yet.
func ReadAll(appDir string) (map[string]string, error) {
	path := filepath.Join(appDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var settings map[string]string
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if settings == nil {
		settings = map[string]string{}
	}
	return settings, nil
}

func writeAll(appDir string, settings map[string]string) error {
	path := filepath.Join(appDir, filename)
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := renameWithRetry(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// renameWithRetry works around a transient Windows file-sharing collision: this file is read
// very frequently by a separate process (internal/overlay re-reads it roughly every frame), and
// Go's os.ReadFile doesn't request FILE_SHARE_DELETE, so a rename onto the destination can fail
// with "Access is denied" if it lands mid-read. The reader's handle is only ever held for a fast
// open-read-close, so a handful of short retries reliably clears it - a real, observed failure
// (a Resources-page checkbox toggle silently not persisting, breaking its sound alert), not a
// hypothetical one.
func renameWithRetry(oldpath, newpath string) error {
	var err error
	for range 5 {
		if err = os.Rename(oldpath, newpath); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return err
}

// Set upserts one key atomically (read-modify-write, same pattern as capture.MutateConfig).
func Set(appDir, key, value string) error {
	settings, err := ReadAll(appDir)
	if err != nil {
		return err
	}
	settings[key] = value
	return writeAll(appDir, settings)
}

// Delete removes one key atomically. Deleting an already-absent key is a no-op.
func Delete(appDir, key string) error {
	settings, err := ReadAll(appDir)
	if err != nil {
		return err
	}
	if _, ok := settings[key]; !ok {
		return nil
	}
	delete(settings, key)
	return writeAll(appDir, settings)
}
