package hub

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/nospy/albion-openradar/internal/adp"
)

// marketStaleAfter bounds how long a cached price row is trusted before GetPrices reports
// it as needing a refresh from the public API - price data doesn't move fast enough to
// warrant tighter polling, and this bounds how often a popular item re-hits the public API.
const marketStaleAfter = 30 * time.Minute

// PriceKey identifies one (item, city, quality) market_prices row.
type PriceKey struct {
	ItemID  string
	City    string
	Quality int
}

// UpsertPrices inserts or refreshes each entry's cached row, stamping cached_at with now.
func (s *Store) UpsertPrices(entries []adp.PriceEntry) error {
	now := time.Now().UTC()
	for _, e := range entries {
		if _, err := s.db.Exec(`
			INSERT INTO market_prices (
				item_id, city, quality,
				sell_price_min, sell_price_min_date,
				sell_price_max, sell_price_max_date,
				buy_price_min, buy_price_min_date,
				buy_price_max, buy_price_max_date,
				cached_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (item_id, city, quality) DO UPDATE SET
				sell_price_min = excluded.sell_price_min,
				sell_price_min_date = excluded.sell_price_min_date,
				sell_price_max = excluded.sell_price_max,
				sell_price_max_date = excluded.sell_price_max_date,
				buy_price_min = excluded.buy_price_min,
				buy_price_min_date = excluded.buy_price_min_date,
				buy_price_max = excluded.buy_price_max,
				buy_price_max_date = excluded.buy_price_max_date,
				cached_at = excluded.cached_at
		`, e.ItemID, e.City, e.Quality,
			e.SellPriceMin, e.SellPriceMinDate,
			e.SellPriceMax, e.SellPriceMaxDate,
			e.BuyPriceMin, e.BuyPriceMinDate,
			e.BuyPriceMax, e.BuyPriceMaxDate,
			now,
		); err != nil {
			return fmt.Errorf("upsert price %s/%s/%d: %w", e.ItemID, e.City, e.Quality, err)
		}
	}
	return nil
}

// GetPrices returns every cached row still fresh enough to trust (cached within
// marketStaleAfter) for each (itemID, city, quality) combination, and separately lists the
// combinations that are either never-cached or stale, so the caller knows what to re-fetch
// from the public API.
func (s *Store) GetPrices(itemIDs, cities []string, qualities []int) (found []adp.PriceEntry, missing []PriceKey, err error) {
	cutoff := time.Now().UTC().Add(-marketStaleAfter)

	for _, itemID := range itemIDs {
		for _, city := range cities {
			for _, quality := range qualities {
				var e adp.PriceEntry
				var cachedAt time.Time
				scanErr := s.db.QueryRow(`
					SELECT item_id, city, quality,
						sell_price_min, sell_price_min_date,
						sell_price_max, sell_price_max_date,
						buy_price_min, buy_price_min_date,
						buy_price_max, buy_price_max_date,
						cached_at
					FROM market_prices
					WHERE item_id = ? AND city = ? AND quality = ?
				`, itemID, city, quality).Scan(
					&e.ItemID, &e.City, &e.Quality,
					&e.SellPriceMin, &e.SellPriceMinDate,
					&e.SellPriceMax, &e.SellPriceMaxDate,
					&e.BuyPriceMin, &e.BuyPriceMinDate,
					&e.BuyPriceMax, &e.BuyPriceMaxDate,
					&cachedAt,
				)
				if scanErr == sql.ErrNoRows {
					missing = append(missing, PriceKey{ItemID: itemID, City: city, Quality: quality})
					continue
				}
				if scanErr != nil {
					return nil, nil, fmt.Errorf("scan price %s/%s/%d: %w", itemID, city, quality, scanErr)
				}
				if cachedAt.Before(cutoff) {
					missing = append(missing, PriceKey{ItemID: itemID, City: city, Quality: quality})
					continue
				}
				found = append(found, e)
			}
		}
	}
	return found, missing, nil
}

// GetCachedPrices returns whatever is cached for each (itemID, city, quality) combination,
// ignoring staleness entirely - used as a last-resort fallback when the public API is
// unreachable and even out-of-date cached data beats returning nothing.
func (s *Store) GetCachedPrices(itemIDs, cities []string, qualities []int) ([]adp.PriceEntry, error) {
	var found []adp.PriceEntry
	for _, itemID := range itemIDs {
		for _, city := range cities {
			for _, quality := range qualities {
				var e adp.PriceEntry
				var cachedAt time.Time
				scanErr := s.db.QueryRow(`
					SELECT item_id, city, quality,
						sell_price_min, sell_price_min_date,
						sell_price_max, sell_price_max_date,
						buy_price_min, buy_price_min_date,
						buy_price_max, buy_price_max_date,
						cached_at
					FROM market_prices
					WHERE item_id = ? AND city = ? AND quality = ?
				`, itemID, city, quality).Scan(
					&e.ItemID, &e.City, &e.Quality,
					&e.SellPriceMin, &e.SellPriceMinDate,
					&e.SellPriceMax, &e.SellPriceMaxDate,
					&e.BuyPriceMin, &e.BuyPriceMinDate,
					&e.BuyPriceMax, &e.BuyPriceMaxDate,
					&cachedAt,
				)
				if scanErr == sql.ErrNoRows {
					continue
				}
				if scanErr != nil {
					return nil, fmt.Errorf("scan price %s/%s/%d: %w", itemID, city, quality, scanErr)
				}
				found = append(found, e)
			}
		}
	}
	return found, nil
}
