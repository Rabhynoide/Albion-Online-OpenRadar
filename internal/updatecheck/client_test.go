package updatecheck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("owner/repo")
	c.BaseURL = srv.URL
	return c
}

func TestFetchLatest_DecodesTagAndURL(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: "1.2.3", HTMLURL: "https://github.com/owner/repo/releases/tag/1.2.3"})
	})

	release, err := client.FetchLatest()
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if release.TagName != "1.2.3" {
		t.Errorf("TagName = %q, want %q", release.TagName, "1.2.3")
	}
	if release.HTMLURL != "https://github.com/owner/repo/releases/tag/1.2.3" {
		t.Errorf("HTMLURL = %q", release.HTMLURL)
	}
}

func TestFetchLatest_RequestsExpectedPath(t *testing.T) {
	var gotPath string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(Release{})
	})

	if _, err := client.FetchLatest(); err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if gotPath != "/repos/owner/repo/releases/latest" {
		t.Errorf("path = %q", gotPath)
	}
}

// @verified: GitHub's API returns 403 Forbidden for requests with no User-Agent header - this
// is a hard requirement of the real API, not an optional nicety like the ADP client's headers.
func TestFetchLatest_SendsUserAgent(t *testing.T) {
	var gotUA string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_ = json.NewEncoder(w).Encode(Release{})
	})

	if _, err := client.FetchLatest(); err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if gotUA == "" {
		t.Error("expected a non-empty User-Agent header")
	}
}

func TestFetchLatest_NonOKStatusIsAnError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := client.FetchLatest(); err == nil {
		t.Error("expected an error for a 404 response")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name, current, latest string
		want                  bool
	}{
		{"newer patch", "1.0.2", "1.0.3", true},
		{"newer minor", "1.0.2", "1.1.0", true},
		{"newer major", "1.0.2", "2.0.0", true},
		{"same version", "1.0.2", "1.0.2", false},
		{"older version", "1.0.2", "1.0.1", false},
		{"current is dev", "dev", "1.0.2", false},
		{"current is empty", "", "1.0.2", false},
		{"latest is empty", "1.0.2", "", false},
		{"tolerates a leading v on both sides", "v1.0.2", "v1.0.3", true},
		{"tolerates a -suffix on the latest tag", "1.0.2", "1.0.3-beta", true},
		{"malformed latest treated as 0.0.0, never newer", "1.0.2", "not-a-version", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNewer(tt.current, tt.latest); got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
