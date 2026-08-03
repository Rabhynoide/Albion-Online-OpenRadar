// Package adp is a small client for the public Albion Online Data Project API
// (https://www.albion-online-data.com/), used as the default/baseline source of market
// price data - both by internal/hub (which caches results server-side) and directly by
// the radar client (internal/server) when no Hub is configured.
package adp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Region selects which of the game's three data-center clusters to query. The public API
// hosts one subdomain per region; there is no single "global" endpoint.
type Region string

const (
	RegionEurope   Region = "europe"
	RegionAmericas Region = "americas"
	RegionAsia     Region = "asia"
)

// regionHosts maps a Region to its actual API subdomain - "americas" reports as "west" and
// "asia" as "east" in the Data Project's own hostnames, confirmed against the live API.
var regionHosts = map[Region]string{
	RegionEurope:   "europe",
	RegionAmericas: "west",
	RegionAsia:     "east",
}

// IsValidRegion reports whether r is one of the three known regions.
func IsValidRegion(r string) bool {
	_, ok := regionHosts[Region(r)]
	return ok
}

// PriceEntry mirrors the Data Project API's own JSON field names exactly (confirmed
// 2026-08-03 against a live https://europe.albion-online-data.com/api/v2/stats/prices/...
// call), so it can be used to decode the API's response directly and, unmodified, as the
// wire shape returned by the Hub's own /api/market/prices endpoint. The *_date fields are
// kept as opaque strings (e.g. "2026-08-03T13:10:00", no timezone) rather than time.Time -
// nothing in this codebase needs to compute with them, only display them, and the API's own
// zero-value sentinel ("0001-01-01T00:00:00") for "no data" round-trips as a plain string
// without needing a custom (un)marshaler.
type PriceEntry struct {
	ItemID           string `json:"item_id"`
	City             string `json:"city"`
	Quality          int    `json:"quality"`
	SellPriceMin     int    `json:"sell_price_min"`
	SellPriceMinDate string `json:"sell_price_min_date"`
	SellPriceMax     int    `json:"sell_price_max"`
	SellPriceMaxDate string `json:"sell_price_max_date"`
	BuyPriceMin      int    `json:"buy_price_min"`
	BuyPriceMinDate  string `json:"buy_price_min_date"`
	BuyPriceMax      int    `json:"buy_price_max"`
	BuyPriceMaxDate  string `json:"buy_price_max_date"`
}

// Client queries the public Albion Online Data Project API for a given region.
type Client struct {
	// BaseURL overrides the computed https://{host}.albion-online-data.com host, for tests
	// (an httptest.NewServer standing in for the public API). Empty means use the real API.
	BaseURL    string
	region     Region
	httpClient *http.Client
}

// NewClient creates a Client for region, defaulting to RegionEurope if region is unknown.
func NewClient(region Region) *Client {
	if _, ok := regionHosts[region]; !ok {
		region = RegionEurope
	}
	return &Client{
		region:     region,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchPrices queries the public API for the given item IDs, optionally narrowed to
// specific cities/qualities (both may be nil/empty to get every combination the API knows).
func (c *Client) FetchPrices(itemIDs, cities []string, qualities []int) ([]PriceEntry, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}

	base := c.BaseURL
	if base == "" {
		base = fmt.Sprintf("https://%s.albion-online-data.com", regionHosts[c.region])
	}

	// City names can contain spaces ("Black Market") and both dimensions are comma-joined
	// lists, so the query values must go through url.Values.Encode() rather than raw string
	// concatenation - an unescaped space corrupts the request line entirely.
	reqURL := fmt.Sprintf("%s/api/v2/stats/prices/%s.json", base, strings.Join(itemIDs, ","))
	q := url.Values{}
	if len(cities) > 0 {
		q.Set("locations", strings.Join(cities, ","))
	}
	if len(qualities) > 0 {
		qs := make([]string, len(qualities))
		for i, quality := range qualities {
			qs[i] = strconv.Itoa(quality)
		}
		q.Set("qualities", strings.Join(qs, ","))
	}
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("fetch prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch prices: unexpected status %d", resp.StatusCode)
	}

	var entries []PriceEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode prices: %w", err)
	}
	return entries, nil
}
