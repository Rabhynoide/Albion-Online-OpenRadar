// Package updatecheck is a small client for GitHub's Releases API, used by cmd/radar to check
// once at launch whether a newer OpenRadar build has been published, and to compare semantic
// version strings. See docs/technical/AUTO_UPDATE_CHECK.md.
package updatecheck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultRepo is the GitHub repo whose releases are checked - the user's own fork, which is
// where actual published (non-draft) releases exist today, not the upstream project.
const DefaultRepo = "Rabhynoide/Albion-Online-OpenRadar"

// Release mirrors the subset of GitHub's release JSON this package needs.
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Client queries GitHub's Releases API for one repo.
type Client struct {
	// BaseURL overrides the default https://api.github.com host, for tests (an
	// httptest.NewServer standing in for the real API). Empty means use the real API.
	BaseURL    string
	repo       string
	httpClient *http.Client
}

// NewClient creates a Client for repo (e.g. "owner/name").
func NewClient(repo string) *Client {
	return &Client{
		repo:       repo,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchLatest fetches the latest published (non-draft, non-prerelease) release. Unlike the
// Albion Online Data Project API, GitHub's API rejects requests with no User-Agent header
// (403 Forbidden) - this is a hard requirement, not an optional nicety.
func (c *Client) FetchLatest() (Release, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	reqURL := fmt.Sprintf("%s/repos/%s/releases/latest", base, c.repo)

	req, err := http.NewRequest(http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return Release{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "OpenRadar-UpdateCheck")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetch latest release: unexpected status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	return release, nil
}

// IsNewer reports whether latest is a newer version than current. Both are plain "X.Y.Z"
// strings (an optional leading "v" and any "-suffix" after the patch number are tolerated and
// ignored - this project's own release tags never carry one today, but the release workflow's
// tag pattern allows it). current being "" or "dev" (an unversioned dev build) always returns
// false - there's nothing meaningful to compare against.
func IsNewer(current, latest string) bool {
	if current == "" || current == "dev" || latest == "" {
		return false
	}
	c := parseVersion(current)
	l := parseVersion(latest)
	for i := range 3 {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseVersion extracts up to three numeric components from a version string, defaulting
// missing or non-numeric components to 0 rather than erroring - a malformed version should be
// treated as "no update available", never crash the launch-time check.
func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
