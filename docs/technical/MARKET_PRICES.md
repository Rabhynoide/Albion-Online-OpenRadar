# Market Prices

How OpenRadar's Market page looks up item prices, and what it deliberately doesn't do yet.

*Last verified against code: 2026-08-03.*

## Detection surface

| Piece | Source | Path |
|---|---|---|
| Item search | `web/ao-bin-dumps/items.min.json` (UniqueName only, no localized display name) | `web/scripts/data/ItemsDatabase.js` (`searchByName`) |
| Price lookup | Public [Albion Online Data Project](https://www.albion-online-data.com/) API, cached by the Hub | `web/scripts/utils/MarketClient.js` -> `internal/server/market_api.go` -> (Hub, if configured) `internal/hub/market_api.go` -> `internal/adp` |
| UI | Search box + per-city price table | `internal/templates/pages/market.gohtml` |

## Architecture: Hub-first, direct-fallback

```
Browser --GET /api/market/prices?items=...--> Radar Go backend
                                                       |
                                         Hub.Enabled?  |  no -> query the public API directly
                                                       |        (client's own Region setting,
                                                      yes        60s in-memory cache)
                                                       v
                                         Hub  GET /api/market/prices (X-Hub-Secret)
                                                       |
                                   Hub's SQLite cache fresh for these items/cities/qualities?
                                                       |
                                      no (miss/stale) -+- yes -> return cached rows
                                                       v
                                    Hub queries the public API itself (its own
                                    AODP_REGION env var), caches the result, returns it
```

The Hub is the only place that talks to the public API when a group has one configured - this
is where the "good base data, refined by the group's own client contributions over time" idea
from the original feature request lives (see "Not implemented yet" below for the contribution
half). The radar client only calls the public API directly as its own fallback when no Hub is
configured/reachable, mirroring how `internal/server/roads_api.go` falls back to a local file
when its Hub is unavailable - except there's no meaningful local cache for prices the way there
is for discovered roads, so the fallback is a direct API call (with a short 60s in-memory cache
to avoid hammering the public API on rapid re-renders of the same search).

## The public API's actual response shape

Confirmed live 2026-08-03 against `https://europe.albion-online-data.com/api/v2/stats/prices/T4_BAG.json?locations=Caerleon&qualities=1`:

```json
[{"item_id":"T4_BAG","city":"Caerleon","quality":1,"sell_price_min":8499,"sell_price_min_date":"2026-08-03T13:10:00","sell_price_max":8500,"sell_price_max_date":"2026-08-03T13:10:00","buy_price_min":8,"buy_price_min_date":"2026-08-02T22:10:00","buy_price_max":4878,"buy_price_max_date":"2026-08-02T22:10:00"}]
```

- Omitting `locations`/`qualities` returns the full cross product (all 8 cities including
  `"Black Market"`, all 5 qualities) - `internal/hub/market_api.go` and
  `internal/server/market_api.go` both apply this same default set when the browser omits them.
- No data for a combination isn't an error - it comes back as a real row with `0`-valued
  prices and the Go zero-time sentinel as a string (`"0001-01-01T00:00:00"`), not an omitted
  entry or a 404.
- The `*_date` fields are kept as opaque strings everywhere in this codebase (`internal/adp.PriceEntry`,
  the Hub's `market_prices` table, the browser) - nothing needs to compute with them, only
  display them, so there's no custom time-parsing for this no-timezone format.
- **Gotcha that cost a real bug**: city names can contain a space (`"Black Market"`). Building
  the query string via raw `strings.Join` + concatenation instead of `net/url`'s
  `url.Values.Encode()` corrupts the HTTP request line entirely - both `internal/adp`'s client
  and the radar backend's Hub-proxy path properly URL-encode every query value for this reason.

## Regions

The game has three data-center clusters; the public API hosts one subdomain per region
(`europe`, `west` for Americas, `east` for Asia - `internal/adp.regionHosts` maps the
user-facing name to the real host). Each side picks its own region independently:

- **The radar client**: `Config.Market.Region` in `network.json`, default `europe`, set via
  **Settings → Market** (`GET/POST /api/settings/market`). Only used for the direct-fallback
  path (no Hub, or Hub unreachable).
- **The Hub**: `AODP_REGION` env var on `cmd/hub`, default `europe`. Used whenever its own
  cache is empty/stale for a requested item.

There's no cross-region merging - a Hub configured for `europe` will never see `americas`
prices, matching how the game's own markets are already fully region-isolated.

## Hub schema (`internal/hub/market_store.go`)

One `market_prices` row per `(item_id, city, quality)`, mirroring the public API's own field
names exactly (`internal/adp.PriceEntry`), plus a `cached_at` timestamp the Hub stamps itself
on every upsert. A row is considered fresh for 30 minutes (`marketStaleAfter`) - past that,
`GetPrices` reports it as needing a refresh from the public API, but `GetCachedPrices` (used
when the public API is unreachable) ignores staleness entirely and returns whatever's there,
on the theory that out-of-date prices beat no prices at all.

`POST /api/market/prices` accepts a batch of price observations and upserts them directly -
built now as a generic, already-tested ingestion endpoint, even though nothing calls it yet
(see below).

## Not implemented yet: live in-game contribution

The original feature request asked for the radar client to *also* submit prices it observes
live, whenever the player browses the in-game marketplace - so a group's Hub gets
progressively better than the public API alone, not just a cached mirror of it.

This needs decoding the marketplace's own Photon wire format first. `AuctionGetOffers` (81) and
`AuctionGetRequests` (82) exist as raw operation-code enum entries
(`internal/photon/operationcodes`, `web/scripts/utils/OperationCodes.js`) but have **never been
captured or decoded** - no parameter layout in `docs/technical/PROTOCOL18_PARAM_LAYOUTS.md`,
no request/response postprocessing anywhere in `internal/photon`. Per this project's
capture-first convention (see `CLAUDE.md`), that's a pcap capture of the in-game marketplace UI
away from being started, tracked as a separate, blocked GitHub issue rather than guessed at
here.
