package hub

import (
	"testing"
	"time"

	"github.com/nospy/albion-openradar/internal/adp"
)

func sampleEntry(itemID, city string, quality int) adp.PriceEntry {
	return adp.PriceEntry{
		ItemID: itemID, City: city, Quality: quality,
		SellPriceMin: 100, SellPriceMinDate: "2026-08-03T13:10:00",
		SellPriceMax: 200, SellPriceMaxDate: "2026-08-03T13:10:00",
		BuyPriceMin: 10, BuyPriceMinDate: "2026-08-02T22:10:00",
		BuyPriceMax: 50, BuyPriceMaxDate: "2026-08-02T22:10:00",
	}
}

func TestMarketStore_GetPricesOnEmptyStoreReportsEverythingMissing(t *testing.T) {
	s := newTestStore(t)

	found, missing, err := s.GetPrices([]string{"T4_BAG"}, []string{"Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("found = %+v, want empty", found)
	}
	if len(missing) != 1 || missing[0] != (PriceKey{ItemID: "T4_BAG", City: "Caerleon", Quality: 1}) {
		t.Errorf("missing = %+v, want one key", missing)
	}
}

func TestMarketStore_UpsertThenGetReturnsFreshRow(t *testing.T) {
	s := newTestStore(t)
	entry := sampleEntry("T4_BAG", "Caerleon", 1)
	if err := s.UpsertPrices([]adp.PriceEntry{entry}); err != nil {
		t.Fatalf("UpsertPrices: %v", err)
	}

	found, missing, err := s.GetPrices([]string{"T4_BAG"}, []string{"Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %+v, want none", missing)
	}
	if len(found) != 1 || found[0] != entry {
		t.Errorf("found = %+v, want %+v", found, entry)
	}
}

func TestMarketStore_UpsertOverwritesExistingRow(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertPrices([]adp.PriceEntry{sampleEntry("T4_BAG", "Caerleon", 1)}); err != nil {
		t.Fatalf("UpsertPrices #1: %v", err)
	}
	updated := sampleEntry("T4_BAG", "Caerleon", 1)
	updated.SellPriceMin = 999
	if err := s.UpsertPrices([]adp.PriceEntry{updated}); err != nil {
		t.Fatalf("UpsertPrices #2: %v", err)
	}

	found, _, err := s.GetPrices([]string{"T4_BAG"}, []string{"Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(found) != 1 || found[0].SellPriceMin != 999 {
		t.Errorf("found = %+v, want refreshed sell_price_min=999", found)
	}
}

func TestMarketStore_DistinctItemCityQualityRowsCoexist(t *testing.T) {
	s := newTestStore(t)
	entries := []adp.PriceEntry{
		sampleEntry("T4_BAG", "Caerleon", 1),
		sampleEntry("T4_BAG", "Caerleon", 2),
		sampleEntry("T4_BAG", "Martlock", 1),
		sampleEntry("T5_BAG", "Caerleon", 1),
	}
	if err := s.UpsertPrices(entries); err != nil {
		t.Fatalf("UpsertPrices: %v", err)
	}

	found, missing, err := s.GetPrices(
		[]string{"T4_BAG", "T5_BAG"},
		[]string{"Caerleon", "Martlock"},
		[]int{1, 2},
	)
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(missing) != 4 { // T4_BAG/Martlock/2, T5_BAG/Caerleon/2, T5_BAG/Martlock/1, T5_BAG/Martlock/2
		t.Errorf("missing = %+v, want 4 keys", missing)
	}
	if len(found) != 4 {
		t.Errorf("found = %+v, want the 4 seeded rows", found)
	}
}

// @verified 2026-08-03: a row cached longer ago than marketStaleAfter must be reported as
// missing (needs a refresh), not returned as trustworthy - manipulates cached_at directly
// since UpsertPrices always stamps "now" and there's no public way to backdate it.
func TestMarketStore_StaleRowIsReportedAsMissing(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertPrices([]adp.PriceEntry{sampleEntry("T4_BAG", "Caerleon", 1)}); err != nil {
		t.Fatalf("UpsertPrices: %v", err)
	}
	staleTime := time.Now().UTC().Add(-marketStaleAfter - time.Minute)
	if _, err := s.db.Exec(
		`UPDATE market_prices SET cached_at = ? WHERE item_id = ? AND city = ? AND quality = ?`,
		staleTime, "T4_BAG", "Caerleon", 1,
	); err != nil {
		t.Fatalf("backdate cached_at: %v", err)
	}

	found, missing, err := s.GetPrices([]string{"T4_BAG"}, []string{"Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("found = %+v, want none (stale)", found)
	}
	if len(missing) != 1 {
		t.Errorf("missing = %+v, want the stale key", missing)
	}
}

func TestMarketStore_FreshRowJustUnderStaleThresholdIsReturned(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertPrices([]adp.PriceEntry{sampleEntry("T4_BAG", "Caerleon", 1)}); err != nil {
		t.Fatalf("UpsertPrices: %v", err)
	}
	recentTime := time.Now().UTC().Add(-marketStaleAfter + time.Minute)
	if _, err := s.db.Exec(
		`UPDATE market_prices SET cached_at = ? WHERE item_id = ? AND city = ? AND quality = ?`,
		recentTime, "T4_BAG", "Caerleon", 1,
	); err != nil {
		t.Fatalf("set cached_at: %v", err)
	}

	found, missing, err := s.GetPrices([]string{"T4_BAG"}, []string{"Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %+v, want none (still fresh)", missing)
	}
	if len(found) != 1 {
		t.Errorf("found = %+v, want the fresh row", found)
	}
}

func TestMarketStore_UpsertEmptySliceIsNoOp(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertPrices(nil); err != nil {
		t.Fatalf("UpsertPrices(nil): %v", err)
	}
}

func TestMarketStore_GetPricesWithEmptyDimensionReturnsNothing(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertPrices([]adp.PriceEntry{sampleEntry("T4_BAG", "Caerleon", 1)}); err != nil {
		t.Fatalf("UpsertPrices: %v", err)
	}

	found, missing, err := s.GetPrices(nil, []string{"Caerleon"}, []int{1})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(found) != 0 || len(missing) != 0 {
		t.Errorf("expected no results for an empty item list, got found=%+v missing=%+v", found, missing)
	}
}
