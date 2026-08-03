# Market Prices

How OpenRadar's Market page looks up item prices, and how the radar contributes back to the Hub.

*Last verified against code: 2026-08-03.*

## Detection surface

| Piece | Source | Path |
|---|---|---|
| Item search | `web/ao-bin-dumps/items.min.json` (UniqueName only, no localized display name) | `web/scripts/data/ItemsDatabase.js` (`searchByName`) |
| Price lookup | Public [Albion Online Data Project](https://www.albion-online-data.com/) API, cached by the Hub | `web/scripts/utils/MarketClient.js` -> `internal/server/market_api.go` -> (Hub, if configured) `internal/hub/market_api.go` -> `internal/adp` |
| UI | Search box + per-city price table | `internal/templates/pages/market.gohtml` |
| Live in-game contribution | Photon `AuctionGetOffers`/`AuctionGetRequests` responses (opcodes 81/82), observed passively while the player browses the marketplace | `web/scripts/data/MarketHandler.js` -> `POST /api/market/observations` -> (Hub, if configured) `POST /api/market/prices` |

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
from the original feature request lives (see "Part B: live in-game contribution" below for the
contribution half). The radar client only calls the public API directly as its own fallback when no Hub is
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

`POST /api/market/prices` accepts `{"side": "sell"|"buy"|"both", "entries": [...]}` and upserts
them. `side` matters: a `PriceEntry` only ever has one side populated when it comes from a live
observation (see below), so `"sell"`/`"buy"` update only their own `sell_price_*`/`buy_price_*`
columns (`Store.UpsertSellObservations`/`UpsertBuyObservations`), leaving whatever was already
known about the *other* side untouched. A bare full-row overwrite (`side` omitted or `"both"`,
`Store.UpsertPrices`) would otherwise wipe out one side every time the other gets updated - the
GET handler's own public-API-fallback path uses `"both"` internally, since a public API response
always has complete data for every entry.

## Part B: live in-game contribution

Shipped 2026-08-03. The radar client passively observes `AuctionGetOffers`/`AuctionGetRequests`
responses (opcodes 81/82) whenever the *player themselves* browses the in-game marketplace -
this project never sends its own market queries, only reads what the game client already asked
for. See `docs/technical/PROTOCOL18_PARAM_LAYOUTS.md`'s "Marketplace operations" section for the
full decoded wire format (confirmed via a live capture).

**`web/scripts/data/MarketHandler.js`** (wired into `EventRouter.onResponse`):
1. Parses `Parameters[0]` - an array of JSON-**encoded strings**, one per listing (each needs
   its own `JSON.parse`, they aren't nested objects).
2. Drops `IsFinished:true` listings (not currently tradeable) and groups the rest by
   `(ItemTypeId, QualityLevel)`.
3. For each group, computes `min`/`max` of `UnitPriceSilver` - this project's own interpretation
   of "current sell/buy price" (the public API's exact aggregation methodology is undocumented
   upstream), matching the field names literally: the range of currently-listed offers, not a
   historical-sales average.
4. Resolves the **city** from the radar's own already-tracked current zone
   (`zonesDatabase.getZoneName(window.currentMapId)`), substring-matched against the 7 known
   royal city names - see the protocol doc for why the wire format itself never carries a city.
   **Black Market observations are silently skipped**: it doesn't appear under that name
   anywhere in `zones.json`, so there's no zone-name heuristic that reaches it; a real fix would
   need to identify whichever zone id the Black Market building actually uses.
5. POSTs the aggregated entries to the radar backend's `POST /api/market/observations`
   (`{side: "sell"|"buy", entries}`), best-effort/fire-and-forget (same `try`/`Promise.catch`
   guard as `ZoneGraph.reportTransition`).

**`internal/server/market_api.go`**'s `handlePostObservations`: relays the payload to the
configured Hub's `POST /api/market/prices` unchanged. Without a Hub configured, there's nowhere
for an observation to go (the public API is read-only) - the endpoint still responds `204`
either way, same "a contribution failing must never surface as a browser error" philosophy as
`RoadsAPI.forwardToHub`.

**Known limitations (v1)**:
- Black Market prices aren't attributed (see above).
- The min/max-of-currently-listed-offers computation is this project's own choice, not a
  verified match for the public API's own historical/transaction-based methodology - the two
  numbers answer a similar but not necessarily identical question.
- No de-duplication/rate-limiting on submissions beyond what naturally happens from the player
  not spamming the marketplace UI - a very active session could submit somewhat often.
