package adp

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
	c := NewClient(RegionEurope)
	c.BaseURL = srv.URL
	return c
}

func TestFetchPrices_BuildsExpectedURL(t *testing.T) {
	var gotPath, gotQuery string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]PriceEntry{})
	})

	_, err := client.FetchPrices([]string{"T4_BAG", "T5_BAG"}, []string{"Caerleon", "Martlock"}, []int{1, 2})
	if err != nil {
		t.Fatalf("FetchPrices: %v", err)
	}

	if gotPath != "/api/v2/stats/prices/T4_BAG,T5_BAG.json" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "locations=Caerleon%2CMartlock&qualities=1%2C2" {
		t.Errorf("query = %q", gotQuery)
	}
}

// @verified 2026-08-03: "Black Market" contains a literal space - building the query string
// via raw concatenation instead of url.Values.Encode() corrupts the HTTP request line entirely
// (caught by an httptest.NewServer round-trip, not a recorder, so a real request line is sent).
func TestFetchPrices_CityNameWithSpaceIsProperlyEncoded(t *testing.T) {
	var gotLocations string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotLocations = r.URL.Query().Get("locations")
		_ = json.NewEncoder(w).Encode([]PriceEntry{})
	})

	_, err := client.FetchPrices([]string{"T4_BAG"}, []string{"Black Market", "Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("FetchPrices: %v", err)
	}
	if gotLocations != "Black Market,Caerleon" {
		t.Errorf("locations = %q, want %q", gotLocations, "Black Market,Caerleon")
	}
}

func TestFetchPrices_NoCitiesOrQualitiesOmitsQueryParams(t *testing.T) {
	var gotQuery string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]PriceEntry{})
	})

	_, err := client.FetchPrices([]string{"T4_BAG"}, nil, nil)
	if err != nil {
		t.Fatalf("FetchPrices: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want empty", gotQuery)
	}
}

func TestFetchPrices_EmptyItemIDsReturnsNilWithoutRequest(t *testing.T) {
	called := false
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode([]PriceEntry{})
	})

	entries, err := client.FetchPrices(nil, nil, nil)
	if err != nil {
		t.Fatalf("FetchPrices: %v", err)
	}
	if entries != nil {
		t.Errorf("entries = %+v, want nil", entries)
	}
	if called {
		t.Error("expected no HTTP request for an empty item list")
	}
}

func TestFetchPrices_DecodesResponseFields(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]PriceEntry{
			{
				ItemID: "T4_BAG", City: "Caerleon", Quality: 1,
				SellPriceMin: 8499, SellPriceMinDate: "2026-08-03T13:10:00",
				SellPriceMax: 8500, SellPriceMaxDate: "2026-08-03T13:10:00",
				BuyPriceMin: 8, BuyPriceMinDate: "2026-08-02T22:10:00",
				BuyPriceMax: 4878, BuyPriceMaxDate: "2026-08-02T22:10:00",
			},
		})
	})

	entries, err := client.FetchPrices([]string{"T4_BAG"}, []string{"Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("FetchPrices: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	got := entries[0]
	if got.ItemID != "T4_BAG" || got.City != "Caerleon" || got.Quality != 1 {
		t.Errorf("identity fields wrong: %+v", got)
	}
	if got.SellPriceMin != 8499 || got.SellPriceMinDate != "2026-08-03T13:10:00" {
		t.Errorf("sell price fields wrong: %+v", got)
	}
	if got.BuyPriceMax != 4878 {
		t.Errorf("buy_price_max = %d, want 4878", got.BuyPriceMax)
	}
}

func TestFetchPrices_NonOKStatusReturnsError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := client.FetchPrices([]string{"T4_BAG"}, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestFetchPrices_MalformedJSONReturnsError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	})

	_, err := client.FetchPrices([]string{"T4_BAG"}, nil, nil)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestNewClient_UnknownRegionDefaultsToEurope(t *testing.T) {
	c := NewClient(Region("mars"))
	if c.region != RegionEurope {
		t.Errorf("region = %q, want europe", c.region)
	}
}

func TestIsValidRegion(t *testing.T) {
	cases := map[string]bool{
		"europe":   true,
		"americas": true,
		"asia":     true,
		"mars":     false,
		"":         false,
	}
	for region, want := range cases {
		if got := IsValidRegion(region); got != want {
			t.Errorf("IsValidRegion(%q) = %v, want %v", region, got, want)
		}
	}
}
