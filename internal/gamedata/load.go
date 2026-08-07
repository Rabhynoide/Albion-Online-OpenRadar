// Package gamedata loads the game-data JSON dumps under web/ao-bin-dumps/ (items, mobs,
// harvestables, zones - extracted from Albion Online's own data files by
// tools/update-ao-data.ts) directly into Go structures, for the native overlay client
// (internal/overlay) - a Go port of the equivalent web/scripts/data/*Database.js loaders,
// same source files, same parsing rules, no browser/fetch involved.
package gamedata

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
)

// readJSON reads filename from fsys, transparently gunzipping if it ends in ".gz" (every
// game-data dump under web/ao-bin-dumps/ ships gzip-only, no plain copy committed - see
// tools/compress-game-data.ts), and decodes it into v.
func readJSON(fsys fs.FS, filename string, v any) error {
	f, err := fsys.Open(filename)
	if err != nil {
		return fmt.Errorf("open %s: %w", filename, err)
	}
	defer f.Close()

	var r io.Reader = f
	if hasGzipSuffix(filename) {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("gunzip %s: %w", filename, err)
		}
		defer gz.Close()
		r = gz
	}

	if err := json.NewDecoder(r).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", filename, err)
	}
	return nil
}

func hasGzipSuffix(filename string) bool {
	return len(filename) > 3 && filename[len(filename)-3:] == ".gz"
}
