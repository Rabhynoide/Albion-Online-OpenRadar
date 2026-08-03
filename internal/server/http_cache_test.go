package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nospy/albion-openradar/internal/logger"
	"github.com/nospy/albion-openradar/internal/templates"
)

const testAppDir = "../.."

// zeroModTimeFS mirrors embed.FS, whose FileInfo.ModTime is the zero time
// (embed/embed.go: func (f *file) ModTime() time.Time { return time.Time{} }).
// Serving from os.DirFS directly would expose a real modtime and let
// http.ServeContent emit a Last-Modified header that production never has.
type zeroModTimeFS struct{ inner fs.FS }

type zeroModTimeFile struct{ fs.File }

type zeroModTimeInfo struct{ fs.FileInfo }

func (zeroModTimeInfo) ModTime() time.Time { return time.Time{} }

func (f zeroModTimeFile) Stat() (fs.FileInfo, error) {
	i, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return zeroModTimeInfo{i}, nil
}

func (f zeroModTimeFile) Seek(offset int64, whence int) (int64, error) {
	s, ok := f.File.(io.Seeker)
	if !ok {
		return 0, fs.ErrInvalid
	}
	return s.Seek(offset, whence)
}

func (f zeroModTimeFile) ReadDir(n int) ([]fs.DirEntry, error) {
	d, ok := f.File.(fs.ReadDirFile)
	if !ok {
		return nil, fs.ErrInvalid
	}
	return d.ReadDir(n)
}

func (z zeroModTimeFS) Open(name string) (fs.File, error) {
	f, err := z.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return zeroModTimeFile{f}, nil
}

func newTestServer(t *testing.T, version string, devMode bool) *HTTPServer {
	t.Helper()
	return newTestServerBuild(t, version, "2026-07-09T10:00:00Z", devMode)
}

func newTestServerBuild(t *testing.T, version, buildTime string, devMode bool) *HTTPServer {
	t.Helper()

	log := logger.New(t.TempDir(), false)
	tmpl, err := templates.NewEngineDev(testAppDir + "/internal/templates")
	if err != nil {
		t.Fatalf("template engine: %v", err)
	}

	wrap := func(dir string) fs.FS {
		disk := os.DirFS(testAppDir + dir)
		if devMode {
			return disk
		}
		return zeroModTimeFS{disk}
	}

	s := &HTTPServer{
		mux:     http.NewServeMux(),
		logger:  log,
		images:  wrap("/web/images"),
		scripts: wrap("/web/scripts"),
		data:    wrap("/web/ao-bin-dumps"),
		sounds:  wrap("/web/sounds"),
		styles:  wrap("/web/styles"),
		tmpl:    tmpl,
		version: version,
		assetID: buildID(version, buildTime),
		devMode: devMode,
	}
	s.settingsAPI = NewSettingsAPI(t.TempDir(), log, nil, t.TempDir())
	s.roadsAPI = NewRoadsAPI(t.TempDir())
	s.hubSettingsAPI = NewHubSettingsAPI(t.TempDir())
	s.setupRoutes()
	return s
}

func do(s *HTTPServer, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, http.NoBody)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

// staticRoutes covers every static handler: gzipFSHandlerDirect (scripts,
// styles, ao-bin-dumps), fsHandler (images, sounds) and fsHandlerWithFallback
// (Items, Spells).
var staticRoutes = []string{
	"/scripts/core/DatabaseLoader.js",
	"/styles/app.css",
	"/images/icon.png",
	"/images/Items/T4_BAG.webp",
	"/images/Spells/SPELL_GENERIC.webp",
	"/sounds/player.mp3",
	"/ao-bin-dumps/harvestables.min.json",
}

func TestETagDistinguishesTwoBuildsOfTheSameVersion(t *testing.T) {
	first := newTestServerBuild(t, "2.2.3", "2026-07-09T10:00:00Z", false)
	second := newTestServerBuild(t, "2.2.3", "2026-07-09T11:00:00Z", false)

	a := do(first, http.MethodGet, "/scripts/core/DatabaseLoader.js", nil).Header().Get("Etag")
	b := do(second, http.MethodGet, "/scripts/core/DatabaseLoader.js", nil).Header().Get("Etag")

	if a == "" || b == "" {
		t.Fatalf("missing Etag: %q, %q", a, b)
	}
	if a == b {
		t.Errorf("both builds emit %s: rebuilding a tag with new assets would 304 forever", a)
	}
}

// RFC 9110: opaque-tag = DQUOTE *etagc DQUOTE, etagc = %x21 / %x23-7E / obs-text.
func TestETagIsAWellFormedOpaqueTag(t *testing.T) {
	s := newTestServerBuild(t, `fix/"quoted" branch`, "2026-07-09T10:00:00Z", false)

	rec := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", nil)
	etag := rec.Header().Get("Etag")

	if len(etag) < 2 || etag[0] != '"' || etag[len(etag)-1] != '"' {
		t.Fatalf("Etag = %q, want a double-quoted tag", etag)
	}
	for i, c := range []byte(etag[1 : len(etag)-1]) {
		if c <= 0x20 || c >= 0x7f || c == '"' {
			t.Errorf("Etag byte %d = %q is not a legal etagc", i, c)
		}
	}

	replay := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", map[string]string{"If-None-Match": etag})
	if replay.Code != http.StatusNotModified {
		t.Errorf("replaying the emitted Etag gives %d, want 304", replay.Code)
	}
}

func TestRangeIsIgnoredOnGzipEncodedAssets(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	rec := do(s, http.MethodGet, "/ao-bin-dumps/harvestables.min.json", map[string]string{
		"Accept-Encoding": "gzip",
		"Range":           "bytes=100-200",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: a range of a gzip stream cannot be inflated", rec.Code)
	}
	if got := rec.Header().Get("Content-Range"); got != "" {
		t.Errorf("Content-Range = %q, want none", got)
	}
	if _, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes())); err != nil {
		t.Errorf("body is not a decodable gzip stream: %v", err)
	}
}

func TestRangeStillWorksOnIdentityAssets(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	for _, path := range []string{"/images/icon.png", "/ao-bin-dumps/harvestables.min.json"} {
		t.Run(path, func(t *testing.T) {
			rec := do(s, http.MethodGet, path, map[string]string{"Range": "bytes=0-99"})

			if rec.Code != http.StatusPartialContent {
				t.Errorf("status = %d, want 206", rec.Code)
			}
			if rec.Body.Len() != 100 {
				t.Errorf("body = %d bytes, want 100", rec.Body.Len())
			}
		})
	}
}

// readAsset must decompress a .gz-only asset for a client that doesn't accept gzip - assets
// like web/ao-bin-dumps/*.json.gz ship with no plain sibling committed (only the pre-compressed
// file), and the identity path (fs.ReadFile(fsys, urlPath)) would 404 without this fallback.
func TestReadAsset_GzOnlySiblingDecompressesForIdentityClient(t *testing.T) {
	var gzBuf bytes.Buffer
	gz := gzip.NewWriter(&gzBuf)
	if _, err := gz.Write([]byte(`{"hello":"world"}`)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	fsys := fstest.MapFS{
		"data.json.gz": &fstest.MapFile{Data: gzBuf.Bytes()},
	}

	body, gzipped, err := readAsset(fsys, "data.json", false)
	if err != nil {
		t.Fatalf("readAsset: %v", err)
	}
	if gzipped {
		t.Error("gzipped = true, want false (identity client)")
	}
	if string(body) != `{"hello":"world"}` {
		t.Errorf("body = %q, want the decompressed JSON", body)
	}
}

// Regression guard: when the client DOES accept gzip and a .gz sibling exists, readAsset must
// keep serving the pre-compressed bytes directly (no unnecessary decompress+recompress).
func TestReadAsset_GzOnlySiblingServedDirectlyForGzipClient(t *testing.T) {
	var gzBuf bytes.Buffer
	gz := gzip.NewWriter(&gzBuf)
	if _, err := gz.Write([]byte(`{"hello":"world"}`)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	fsys := fstest.MapFS{
		"data.json.gz": &fstest.MapFile{Data: gzBuf.Bytes()},
	}

	body, gzipped, err := readAsset(fsys, "data.json", true)
	if err != nil {
		t.Fatalf("readAsset: %v", err)
	}
	if !gzipped {
		t.Error("gzipped = false, want true (gzip-accepting client)")
	}
	if !bytes.Equal(body, gzBuf.Bytes()) {
		t.Error("body does not match the pre-compressed .gz sibling bytes")
	}
}

// Neither a plain file nor a .gz sibling exists - must still 404, not panic or loop.
func TestReadAsset_NeitherPlainNorGzExistsReturnsError(t *testing.T) {
	fsys := fstest.MapFS{}

	_, _, err := readAsset(fsys, "missing.json", false)

	if err == nil {
		t.Fatal("expected an error for a genuinely missing asset")
	}
}

func TestHTMLPagesAreNoCache(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	rec := do(s, http.MethodGet, "/", nil)

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
}

// pageContentMarkers pairs each registered page route with a string unique to that page's own
// content - not present on any other page (in particular not on the Radar page, the silent
// fallback both base.gohtml and content.gohtml use for an unrecognized .Page value).
var pageContentMarkers = []struct {
	path   string
	marker string
}{
	{"/", "mapCanvas"},
	{"/players", "settingShowPlayers"},
	{"/resources", "settingResourceSound"},
	{"/enemies", "settingAllEnemies"},
	{"/chests", "settingChestGreen"},
	{"/market", "marketSearchInput"},
	{"/ignorelist", "ignorePlayerInput"},
	{"/settings", "confirmResetSettings"},
}

// Regression guard: internal/templates/layouts/base.gohtml (full page load) and
// layouts/content.gohtml (HTMX partial swap, SPA in-app navigation) each maintain their OWN
// {{if eq .Page "..."}} dispatch chain - by hand, not generated from a shared list. A page
// added to one chain but not the other silently falls back to rendering the Radar page instead
// of its own, on whichever path was missed. Confirmed as a real bug: the Market page did
// exactly this on every full page load/refresh/direct-URL-visit for a full day after it
// shipped, because only content.gohtml's chain had been updated.
func TestAllPagesRenderTheirOwnContent(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	for _, tc := range pageContentMarkers {
		t.Run(tc.path, func(t *testing.T) {
			full := do(s, http.MethodGet, tc.path, nil)
			if !strings.Contains(full.Body.String(), tc.marker) {
				t.Errorf("full page load of %s does not contain %q - likely fell back to another page", tc.path, tc.marker)
			}

			partial := do(s, http.MethodGet, tc.path, map[string]string{"Hx-Request": "true"})
			if !strings.Contains(partial.Body.String(), tc.marker) {
				t.Errorf("HTMX partial load of %s does not contain %q - likely fell back to another page", tc.path, tc.marker)
			}
		})
	}
}

func TestHTMLPagesVaryOnHxRequest(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	full := do(s, http.MethodGet, "/players", nil)
	partial := do(s, http.MethodGet, "/players", map[string]string{"Hx-Request": "true"})

	if full.Body.Len() == partial.Body.Len() {
		t.Fatal("full page and HTMX partial are identical, this test proves nothing")
	}
	for _, rec := range []*httptest.ResponseRecorder{full, partial} {
		if got := rec.Header().Get("Vary"); got != "Hx-Request" {
			t.Errorf("Vary = %q, want %q: the same URL serves two representations", got, "Hx-Request")
		}
	}
}

func TestAPIResponsesAreNoStore(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	rec := do(s, http.MethodGet, "/api/settings/logging", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: a 404 would carry the header too", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
}

func TestUntrustedVersionEmitsNoETag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version string
		devMode bool
	}{
		{"unversioned build", "dev", false},
		{"empty version", "", false},
		{"dev mode", "2.2.3", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, tc.version, tc.devMode)

			rec := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", nil)

			if got := rec.Header().Get("Etag"); got != "" {
				t.Errorf("Etag = %q, want none", got)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
			}
		})
	}
}

func TestGzipRepresentationHasDistinctETag(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	gzipped := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", map[string]string{
		"Accept-Encoding": "gzip",
	})
	identity := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", nil)

	if got := gzipped.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}
	if a, b := gzipped.Header().Get("Etag"), identity.Header().Get("Etag"); a == b {
		t.Errorf("both representations emit %s, but an ETag identifies a representation", a)
	}
	if got := gzipped.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want %q", got, "Accept-Encoding")
	}
}

func TestIdentityETagDoesNotValidateGzipRepresentation(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	identity := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", nil).Header().Get("Etag")
	rec := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", map[string]string{
		"Accept-Encoding": "gzip",
		"If-None-Match":   identity,
	})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: identity validator must not match gzip bytes", rec.Code)
	}
}

func TestMatchingIfNoneMatchReturns304(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	for _, path := range staticRoutes {
		t.Run(path, func(t *testing.T) {
			etag := do(s, http.MethodGet, path, nil).Header().Get("Etag")
			rec := do(s, http.MethodGet, path, map[string]string{"If-None-Match": etag})

			if rec.Code != http.StatusNotModified {
				t.Errorf("status = %d, want 304", rec.Code)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %d bytes, want 0", rec.Body.Len())
			}
		})
	}
}

func TestStaleIfNoneMatchReturnsFullBody(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	for _, path := range staticRoutes {
		t.Run(path, func(t *testing.T) {
			rec := do(s, http.MethodGet, path, map[string]string{"If-None-Match": `"from-an-older-build"`})

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if rec.Body.Len() == 0 {
				t.Error("body is empty, want the asset")
			}
		})
	}
}

func TestStaticAssetsAlwaysRevalidate(t *testing.T) {
	s := newTestServer(t, "2.2.3", false)

	for _, path := range staticRoutes {
		t.Run(path, func(t *testing.T) {
			rec := do(s, http.MethodGet, path, nil)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
			}
			if rec.Header().Get("Etag") == "" {
				t.Error("no Etag, so revalidation can never answer 304")
			}
		})
	}
}

// countingFS wraps an fs.FS and counts Open calls, so tests can prove readAssetCached
// actually skips the underlying filesystem (and re-gzipping) on a cache hit.
type countingFS struct {
	inner fs.FS
	opens map[string]int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.opens[name]++
	return c.inner.Open(name)
}

func TestGzipAssetCache_ProdModeSkipsRecompressionOnSecondRequest(t *testing.T) {
	log := logger.New(t.TempDir(), false)
	tmpl, err := templates.NewEngineDev(testAppDir + "/internal/templates")
	if err != nil {
		t.Fatalf("template engine: %v", err)
	}
	counting := &countingFS{inner: zeroModTimeFS{os.DirFS(testAppDir + "/web/scripts")}, opens: map[string]int{}}

	s := &HTTPServer{
		mux:     http.NewServeMux(),
		logger:  log,
		scripts: counting,
		tmpl:    tmpl,
		version: "2.2.3",
		assetID: buildID("2.2.3", "2026-07-09T10:00:00Z"),
		devMode: false,
	}
	s.settingsAPI = NewSettingsAPI(t.TempDir(), log, nil, t.TempDir())
	s.roadsAPI = NewRoadsAPI(t.TempDir())
	s.hubSettingsAPI = NewHubSettingsAPI(t.TempDir())
	s.setupRoutes()

	headers := map[string]string{"Accept-Encoding": "gzip"}
	first := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", headers)
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", first.Code)
	}
	opensAfterFirst := counting.opens["core/DatabaseLoader.js"]
	if opensAfterFirst == 0 {
		t.Fatal("expected the underlying fs.FS to be read on a cache miss")
	}

	second := do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", headers)
	if second.Code != http.StatusOK {
		t.Fatalf("second request status = %d, want 200", second.Code)
	}
	if got := counting.opens["core/DatabaseLoader.js"]; got != opensAfterFirst {
		t.Errorf("underlying fs.FS opened %d more time(s) on a cache hit, want 0 more", got-opensAfterFirst)
	}
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Error("cached response body differs from the original")
	}
}

func TestGzipAssetCache_DevModeAlwaysRereadsForHotReload(t *testing.T) {
	log := logger.New(t.TempDir(), false)
	tmpl, err := templates.NewEngineDev(testAppDir + "/internal/templates")
	if err != nil {
		t.Fatalf("template engine: %v", err)
	}
	counting := &countingFS{inner: os.DirFS(testAppDir + "/web/scripts"), opens: map[string]int{}}

	s := &HTTPServer{
		mux:     http.NewServeMux(),
		logger:  log,
		scripts: counting,
		tmpl:    tmpl,
		version: "2.2.3",
		devMode: true,
	}
	s.settingsAPI = NewSettingsAPI(t.TempDir(), log, nil, t.TempDir())
	s.roadsAPI = NewRoadsAPI(t.TempDir())
	s.hubSettingsAPI = NewHubSettingsAPI(t.TempDir())
	s.setupRoutes()

	headers := map[string]string{"Accept-Encoding": "gzip"}
	do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", headers)
	do(s, http.MethodGet, "/scripts/core/DatabaseLoader.js", headers)

	if got := counting.opens["core/DatabaseLoader.js"]; got < 2 {
		t.Errorf("dev mode opened the file %d time(s) for 2 requests, want at least 2 (no caching)", got)
	}
}
