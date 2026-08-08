# OpenRadar Roadmap

**Version**: 2.2.0
**Last update**: 2026-08-08

## Detection systems status

| System | Status | Notes |
|---|---|---|
| Resources | working | database-driven, cleanup, filtering, T1-T8 with enchantments, render-time gate (#82), sound alert on a filter-matching spawn (#22) |
| Mobs | working | OFFSET=16 confirmed (#93), 9 classifications, color-coded threat |
| Players | working | faction detection, zone-aware alerts (unmapped zones no longer suppress hostile alerts, #65), ignore list actually respected by alerts/radar/threat-border (#36), Mist instance pvpType inherits parent (#103), party members auto-excluded from alerts/radar/threat-border (#3) |
| Zones | working | PvP type detection, threat logic |
| Mists | working | portals, feu follets, wisp cages (see `docs/technical/MISTS_DETECTION.md`) |
| Dungeons | working | per-type filters Solo, Group (Duo), Corrupted, Hellgate validated end to end. Per-enchant filters E0-E4 work across every family. Five group families unblocked by #78 (T6_MORGANA, T6_KEEPER, T6_UNDEAD, T5_PORTAL_ROYAL_SOLO, T6_PORTAL). Avalonian dungeons, per-difficulty filters, and a dungeons database stay open. |
| Chests | basic | rarity persisted on entity (#75); drawing-layer color resolution and rarity source slot still open |
| Fishing | working | issue #25 closed via #73 + #85. Event 61 (end-of-fishing) logged but not visualized. |
| GPS / Avalon Roads | working | static open-world graph from `cluster/world.json`, road discovery by play, 24h staleness confirmed from game data, manual "Remove this route" for roads that reset early (#5) - see `docs/technical/AVALON_ROADS_GPS.md`. Per-instance road duration still unobservable. Optional shared roads database via self-hosted `cmd/hub` (Docker, SQLite, shared-secret auth) - radar backend relays/falls back automatically, see `cmd/hub/README.md`. |
| Local Treasures | working | buried chests, temporary resources, smuggler piles, timed events via `LocalTreasuresUpdate` (event 285, #4); `SPECIAL_EVENT_*` excluded as mob-detection duplicates. Single shared icon in v1, no per-type icons or countdown yet. |
| Market Prices | working | item search + per-city price table, backed by the public Albion Online Data Project API, cached/shared via the Hub when configured; live in-game contribution to the Hub while browsing the marketplace (#23) - see `docs/technical/MARKET_PRICES.md`. Black Market observations not attributed (doesn't appear under that name in `zones.json`). |

## v2.3 backlog

### Detection completion

- [ ] **Dungeons**: create `DungeonsDatabase.js` (types, tiers, difficulties) for Avalonian dungeons and per-difficulty filters. Per-type filters (Solo, Group, Corrupted, Hellgate) and per-enchant filters already work in v2.2.
- [ ] **Chests**: rarity drawing-layer wiring (CHEST-2 stored the value at #75, drawing still does substring matches on `chestName`). Plus identify the real rarity source slot (CHEST-1): current `Parameters[5]` is 4 for Mists treasure, 8 for FactionWarfare, never lands in 0-3. Pcap capture across the four rarity levels needed before fixing.
- [ ] **Fishing**: end-of-fishing state, fishing zones on the radar.
- [ ] **Mists routing**: cases for events 518 (NewMistsImmediateReturnExit), 519 (MistsPlayerJoinedInfo), 520 (NewMistsStaticEntrance), 529 (MistsEntranceDataChanged) reach the frontend but no handler consumes them.
- [ ] **Brecilien ecosystem (#117)**: Mist Of Brecilien wrongly classified Safe (PR #103 inheritance hole when origin is `safe`), Roads of Avalon one-way exit drops player events. Consolidates #112, #108, #102. Blocked on 50K Brecilien Standing for pcap captures.

### Maps

- [ ] Map tile size normalization (fix stretching on small zones).
- [ ] Map centering optimization.

### Stability and performance

- [ ] Memory usage optimization for very long sessions.
- [ ] Black Zone portal transitions sometimes drop the cluster id.

### Other improvements

- [ ] Quality metrics dashboard.

## Closed in v2.3 (in progress)

- **PLAY-1** (#65): a zone missing from `zones.json` no longer gets silently treated as
  `'safe'` for threat alerting. `PlayersHandler.getAlertPvpType()` checks
  `zonesDatabase.getZone()` first and falls back to `'yellow'` (not `'safe'`) only when the
  zone itself is genuinely unrecognized, so `faction=255` still alerts.
- **PLAY-2** (#36): the Ignore List (`internal/templates/pages/ignorelist.gohtml`,
  `settingsSync` key `ignoreList`) is now actually consulted - by every alert path
  (`maybeAlert()`, covering both the faction-change and already-hostile-on-spawn cases),
  by `getFilteredPlayers()` (ignored players no longer show on the radar/player list,
  matching what the page always claimed), and by `getThreatPlayers()` (no pulsing threat
  border either). The dead `alreadyIgnoredPlayers` field (never populated by anything) is
  removed. The page's "Known limitation" banner is removed accordingly.
- **GPS-1** (#5, "Suppression des Ava expirées du GPS"): a discovered road could only ever
  age out via the 24h staleness window, even after the player found it had already reset to
  a different connection. Added a "Remove this route" button on the sidebar GPS widget
  (`ZoneGraph.removeEdge()`, `DELETE /api/roads/edges`, forwarded to the Hub if configured) -
  see `docs/technical/AVALON_ROADS_GPS.md`'s "Manual removal" section.
- **PLAY-3** (#3, "Whitelist joueurs friendly"): party members are now auto-excluded from
  hostile-player alerts, the radar player list, and the pulsing threat border - the same
  exclusion the manual Ignore List already provided (PLAY-2), but detected live instead of
  curated by hand. Decoded the previously-uncaptured Photon Party event family (231/232/235)
  from a real capture - see `docs/technical/PROTOCOL18_PARAM_LAYOUTS.md`'s "Party events"
  section. New `web/scripts/data/PartyRoster.js` (session-only, not persisted like
  `ignoreList` - a party roster shouldn't survive a reload or outlive the party) tracks the
  current roster live off `EventRouter.js`; `PlayersHandler.isExcludedPlayer()` now checks it
  alongside the Ignore List, consulted by `maybeAlert()`, `getFilteredPlayers()`, and
  `getThreatPlayers()` identically to how PLAY-2 wired the Ignore List in.
- **TREASURE-1** (#4, "Détection des trésors enfoui"): this had actually been fully built
  (`LocalTreasuresHandler.js`/`LocalTreasuresDrawing.js`, `EventRouter.js`/`RadarRenderer.js`
  wiring, a settings toggle) on a separate `treasures-detector` branch that was never merged -
  `main` never had it despite this doc's earlier "working" status claiming otherwise. Ported
  the feature onto current `main` (adapting the wiring to everything that landed since the
  branch diverged - GPS, PLAY-1/2/3, the backend/frontend optimization pass). Buried treasure
  chests, temporary rich resource nodes, smuggler piles, and timed special/anniversary events
  now actually render on the radar, gated behind `settingLocalTreasures`.
- **MAP-1** (#15, "Extraction des tuiles de map Zone Noire"): turned out not to be a missing-
  extraction problem - 641/641 real (non-instanced) Black Zone tiles were already downloaded
  and committed. The actual bug: Roads of Avalon passage/tunnel instances have a per-instance
  zone id (`PSG-0039#2`) but only one tile is ever downloaded per base zone (`PSG-0039.webp`,
  named from `zone.file`, not `zone.id` - see `download-and-optimize-map.ts`).
  `MapsDrawing.js`'s `draw()` was looking up the tile by the raw (suffixed) id, so it 404'd for
  every one of these - 138 zones across all pvp tiers, not just Black Zone. Now resolves the
  image name through `zonesDatabase.getZoneFile()`, truncated the same way the downloader
  truncates it, falling back to the raw id when a zone has no `file` (unaffected zones keep
  their exact previous behavior).
- **CFG-1** (#21, "Support d'un fichier de config au-delà de network.json"): the browser-
  localStorage settings (`web/scripts/utils/SettingsSync.js` - feature toggles, the Ignore
  List, etc.) now also persist server-side to `settings-sync.json` (new `internal/syncsettings`
  package, same atomic-write pattern as `network.json`), so they survive a browser data wipe or
  move to another machine. New `GET/POST/DELETE /api/settings/sync`. `localStorage` stays the
  synchronous source of truth for reads and the fallback when the backend is unreachable;
  writes go through best-effort (mirrors `ZoneGraph.reportTransition`'s fetch-guard pattern),
  and a value already present locally is never overwritten by hydration - only genuinely
  missing keys are filled in. Hydration is gated behind the existing `globalsReady` event in
  `base.gohtml`, so every page benefits (not just the main radar page).
- **HARV-5** (#22, "Alerte sonore sur ressource filtrée détectée"): plays a one-shot sound
  (`web/sounds/coin.mp3`) the first time a harvestable's tier/enchant match the player's
  current resource filters - the exact same gate `HarvestablesDrawing.js` uses to decide
  whether to render it (`shouldRenderStaticResource`/`shouldRenderLivingResource` from
  `LivingResourceFilter.js`), so the alert never fires for something that wouldn't actually be
  shown. Checked both at spawn (individual Event 40, enchant known immediately) and at Event
  46's enchant correction (batch Event 38 spawns start at enchant=0), gated behind a new
  `settingResourceSound` toggle on the Resources page. A per-instance `alerted` flag stops it
  from re-firing on every later size update once a spawn has already matched once.
- **MARKET-1** (Market Prices, Part A): new Market page (item search + per-city sell/buy price
  table), backed by the public Albion Online Data Project API by default and cached/shared via
  the existing self-hosted Hub when configured - new `internal/adp` client package, new
  `market_prices` table + `GET/POST /api/market/prices` on the Hub (`internal/hub`), a matching
  Hub-first/direct-fallback proxy on the radar client (`internal/server/market_api.go`), and a
  `Market.Region` setting. See `docs/technical/MARKET_PRICES.md`.
- **MARKET-2** (#23, Market Prices Part B): the radar client now contributes prices it observes
  while the player browses the in-game marketplace. Decoded the previously-uncaptured
  `AuctionGetOffers`/`AuctionGetRequests` operations (81/82) from a real capture - see
  `docs/technical/PROTOCOL18_PARAM_LAYOUTS.md`'s "Marketplace operations" section; the finding
  that the wire format never carries a city (resolved instead from the radar's own zone
  tracking) shaped the whole design. New `web/scripts/data/MarketHandler.js` aggregates
  observed listings and POSTs to a new `/api/market/observations` relay
  (`internal/server/market_api.go`), forwarded to the Hub best-effort. The Hub's own
  `POST /api/market/prices` gained a `side` parameter (`Store.UpsertSellObservations`/
  `UpsertBuyObservations`) so a sell-only or buy-only observation can't wipe out the other side
  of a previously-cached row. Black Market observations aren't attributed (not in `zones.json`
  under that name).
- **UPDATE-1** (#24): check-and-notify auto-update - at launch, `cmd/radar` checks GitHub's
  latest published release on the user's fork (`Rabhynoide/Albion-Online-OpenRadar`, hardcoded,
  throttled to once/hour) and, if newer, shows a dismissible notice with a link to the release
  on both surfaces (sidebar badge on the web UI, header line on the TUI dashboard). No silent
  binary self-replacement. New `internal/updatecheck` client package,
  `Config.UpdateCheck` in `network.json`, `GET/POST /api/settings/update(/dismiss)`
  (`internal/server/update_settings_api.go`), `web/scripts/core/UpdateBadge.js`. See
  `docs/technical/AUTO_UPDATE_CHECK.md`.
- **OVERLAY-1**: native click-through radar overlay (`cmd/radar -overlay`) - an undecorated,
  transparent, always-on-top Ebiten window that renders on top of the game, click-through
  togglable via F9 (global key poll, since window focus is unreachable once passthrough is
  active). Purely a visual companion to the existing web app, not a replacement - Players/
  Chests/Market/Settings/Resources/Enemies/Ignore List all stay on the web app, which keeps
  running unchanged; the overlay only reads `settings-sync.json` to decide what to draw, never
  writes to it. New `internal/gamedata` (Go port of the JS `*Database.js` loaders) and
  `internal/radarstate` (Go port of every `web/scripts/handlers/*.js` handler plus a `Router`
  mirroring `EventRouter.js`'s dispatch - caught a real bug in the process, see
  `docs/technical/NATIVE_OVERLAY_CLIENT.md`). Renders every tracked entity type (harvestables,
  mobs, chests, dungeons, fishing, local treasures, MistsDungeon portals, WispCages), the local
  player, and the zone map background. Resource clustering and health bars are intentionally
  out of scope (confirmed not needed), unlike the web radar's own `DrawingUtils.js`.
- **SETTINGS-1**: native, browser-free configuration client (`cmd/radar-settings`, Fyne) covering
  Players/Resources/Enemies/Chests/Ignore List/Settings - the rest of the web app OVERLAY-1 left
  untouched. A "Lancer l'overlay" button in its header spawns the sibling `cmd/radar -overlay
  -no-server` binary for the map view. **Real, load-bearing finding**: Ebiten (`internal/overlay`)
  and Fyne cannot both be *linked* into one Windows binary - not just "not run together", a bare
  `import _ "ebiten"` alone breaks Fyne's window-class registration (`RegisterClassEx` collision)
  even if the import is never used - so `cmd/radar-settings` is a genuinely separate binary/
  process from `cmd/radar`, never importing `internal/overlay`. Shared bootstrap (capture +
  HTTP/WS server) extracted into a new UI-toolkit-agnostic `internal/radarapp` package both
  binaries import; `cmd/radar`'s own `App` now embeds `*radarapp.App` and layers TUI/overlay-
  specific wiring on top via `OnEvent`/`OnRequest`/`OnResponse` hooks. Dark navy/blue theme +
  left sidebar nav approximating the web app's own look (`internal/settingsui/theme.go`/`nav.go`).
  See `docs/technical/NATIVE_SETTINGS_CLIENT.md`.

  **A full manual-testing audit turned up a long list of pre-existing gaps in the overlay**
  (`internal/overlay`), all fixed in this same phase - most share one root cause: a filter that
  lives in the web app's *drawing* layer (`web/scripts/drawings/*.js`) was never ported when the
  overlay's `Draw()` loop was first written, so the setting existed and did nothing:
  - Resources tier/enchant grid never actually filtered what the overlay drew (only gated the
    sound alert) - new `HarvestablesState.ShouldRender`/`MobsState.ShouldRender` (the latter for
    living/skinnable resources tracked as mobs, a *second*, separate JS gate in
    `MobsDrawing.js`).
  - Chests (rarity), Fishing, Local Treasures, Knightfall Abbey portals, and Wisp Cages all drew
    unconditionally, ignoring their own settings entirely.
  - Wisp spawn signs ("feu follets", pre-portal) weren't wired into the overlay **at all** -
    the underlying `MobsState.MistSnapshot()` data existed but nothing read or drew it.
  - Dungeons had no filter of any kind (`shouldRenderDungeon`, ported from
    `DungeonsHandler.js`'s ingestion-time gate, applied at draw time instead so a toggle takes
    effect on already-tracked entries immediately).
  - The Enemies page's unidentified-mob toggle, minimum-health threshold, Avalonian drones, and
    event enemies were all missing from the overlay's mob filter (only the four main hostile
    types were gated).
  - `settingsPanel.isOn` defaulted an unset setting to **visible**, the opposite of every one of
    these pages' actual default-unchecked checkboxes (`SettingsSync.js`'s own `getBool` default
    is `false`) - silently over-showing hostile enemy types nobody had opted into.
  - **Hostile-player and resource sound/flash/pulsing-border alerts never fired at all**:
    `PlayersState.PendingAlerts()`/`HarvestablesState.PendingAlerts()` existed, fully documented
    as meant for the overlay to consume, but nothing ever called them. Wired up in
    `internal/overlay/audio.go` (Ebiten's `audio`/`audio/mp3`, reusing `web/sounds/{coin,
    player}.mp3`) + `Game.updateAlerts`/`drawAlerts`. Found and fixed a real, latent data race
    in the process: those alert queues were written from the capture goroutine and about to be
    read from the Ebiten goroutine with no synchronization - added dedicated mutexes.
  - **Black Zone hostile detection didn't work AT ALL, in any zone** (not just BZ):
    `PlayersState.SetCurrentZone` was never called by the router on a real zone change (only
    `Session`'s own separate zone field was kept in sync), so `HandleNewCharacter`'s
    `currentZone != ""` alert guard was permanently false and BZ's "every player is a threat"
    rule (`IsPlayerThreat`) could never resolve correctly. Fixed in `router.go`'s
    `handleZoneChangeResponse`.
  - Depleted/already-harvested resource nodes stayed on the radar (missing the same `size <= 0`
    guard `HarvestablesDrawing.js` applies).
  - A separate, unrelated pre-existing bug surfaced during testing and fixed alongside these:
    `internal/server`'s WebSocket batching dropped an entire 10-message batch to the browser
    whenever a single message's Photon parameters happened to decode to `NaN` (which
    `encoding/json` refuses to serialize) - now only the actual offending message(s) are dropped.

  Also added: the overlay window is now click-drag-to-move and corner-drag-to-resize (kept
  square; zoom rescales with window size so the same detection range stays visible at any size),
  persisted to `overlay-window.json`; user-adjustable `settingOverlayZoom`/
  `settingOverlayMapOpacity` on the Settings page; a colored ring around resource nodes
  (static and living) matching Albion's own enchantment-tier colors (green/blue/purple/gold),
  a native-overlay-only addition with no direct web equivalent.

  Web page removal (Radar/Players/Resources/Enemies/Chests/Ignore List/Settings, Market stays
  web-only) is still pending, done once each native page is manually validated - see
  `docs/technical/NATIVE_SETTINGS_CLIENT.md`'s status section for what's left. Embedding a map
  view directly in `cmd/radar-settings` (as an alternative to launching the separate overlay
  window) was scoped and deliberately deferred - real work, not started.
- **SETTINGS-2**: native Settings page parity follow-up found while validating SETTINGS-1 live -
  the "LAN access" list (clickable `http://<lan-ip>:port/` links, `NetworkSettingsHandler.js`)
  had no native equivalent, and the Network section's capture-status banner only refreshed on
  the manual buttons instead of polling like the web page does. Added both to
  `internal/settingsui/settings.go`'s `newNetworkSection` (`capture.LANAddresses()`, a 5s
  `fyne.Do` ticker mirroring the one `run.go`'s top bar already used).

## Closed in v2.2

For history. These were `test.fails` or open register entries that flipped to verified during the v2.2 cycle:

- **HARV-1** (#71): `mobileTypeId === -1` now treated as static.
- **HARV-3** (#74): re-gate reads stored `mobileTypeId` instead of hardcoded `isLiving=false`.
- **HARV-4** (#82): living plus DEAD plus static render-time filter, superseded HARV-3 narrative.
- **FISH-1** (#73): empty-string spawn type accepted, closing #25.
- **CHEST-2** (#75): chest rarity persisted on the entity (drawing-layer wiring still on the v2.3 backlog).
- **WISP-1** (#78): wisp cage parameter indexing corrected to `[0]/[2]/[4]`.
- **MIST-1, MIST-3, MIST-5, MIST-6** (#78): inverted enchant filter, feu follet routing, asset reuse, dungeon enchant source.
- **ROUTER-2..9** (#70): event code drift closed by the upstream sync.
- **TIER-1** (#77, #93): living harvest tier rule, then OFFSET=16 confirmation, retiring the `t-1` shift.
- **#90 Mist pvpType** (#103): Mist instances inherit parent cluster classification.
- **#57 BZ derivation** (#87): `map.isBZ` from `zonesDatabase` instead of broken `Parameters[103]` parse.

## Open observations from PR cycles

- **CHEST-1** (#29): rarity parameter source unidentified. `Parameters[5]` does not match the upstream 0-3 range across families: 4 for Mists treasure, 8 for FactionWarfare. Needs a multi-rarity pcap capture to find the real index.
- **MIST-2** (feu follet rarity location): every pcap fixture sample is Common (`Parameters[33]=0`). Live evidence on a "Peu commun" portal showed the rarity is actually carried; we still need a multi-rarity capture to find the slot.
- **MIST-4** (Mists cluster rarity): zone-level rarity lives in the `ChangeCluster` operation response `Parameters[3]` byte array, last byte. Plumbing it requires a Mists capture with opcode 41 response and a cluster-rarity store.
- **MIST-7** (cluster id routing): events 518, 519, 520, 529 carry the Mists cluster id but no handler consumes them. Follow-up PR to plumb a Mists state surface readable by drawings.
- **HARV-2** (living spawn with E0 off plus event 46 enchant update): #82 moved the gate to render-time, which fixes the user-visible toggle latency, but the underlying recovery from a depleted-then-regenerated state is not addressed. Pinned by `test.fails`.
- **ROUTER-1** (#57): direct hashtable parse of `Parameters[103]` is a follow-up. The user-visible BZ alert symptom was resolved by deriving `map.isBZ` from `zonesDatabase.getPvpType(mapId)` (#87), which is the correct long-term path; the direct parse stays pinned in case a future change needs the raw value.
- **OPS-1..4**: four call sites in `EventRouter.js` hardcode opcodes whose upstream name does not match the local handler semantics (event 590 logs as `key_sync`, request 21 is the pre-Protocol18 Move opcode kept as legacy fallback, response 35 treated as map-change with debounce, response 137 is a probably-dead character-stats branch). Each carries a `FIXME ops-drift` comment. Resolution requires pcap-backed investigation.

## Tech debt

- **`NewHTTPServer` config struct**: signature is at 10 parameters after #91. Refactor to `NewHTTPServer(cfg HTTPServerConfig)` to keep the call site readable as more wiring lands. Estimate: 1h.
- **Aggregate `pcap.Stats` across handles**: the per-30s kernel-drop log line was removed when the multi-interface manager replaced the single capturer (commit `fedb2c4e`, replaced by `// TODO(#91)` in `cmd/radar/main.go:updateStats`). Restore by adding `Manager.Stats() map[string]*pcap.Stats` and logging deltas. Helps in-prod debugging of capture loss. Estimate: 2h.
- **TUI awaiting-state banner**: when all opens fail at boot, the warn-log is the only signal. The settings page banner shows the state, the TUI does not. Estimate: 30m.
- **`window.EnemyType` ESM cleanup**: `RadarRenderer._collectClusterCandidates` and `MobsDrawing.invalidate` still read from `window.EnemyType` instead of the ESM `import {EnemyType}` already in scope. Pre-ESM-migration artefact, low risk. Estimate: 30m.
- **`/api/settings/server-logs` removal**: replaced by `/api/settings/logging` in v2.2 (#107). Old endpoint returns 404. No clients in the wild known to use the old path; no compatibility shim shipped. Note in case of future bug reports.

## Permanent limitations

- **Player live positions**: encrypted via XOR with a KeySync `XorCode` itself wrapped by Photon AES. Out of scope without a MITM proxy. See `docs/technical/PLAYER_POSITIONS_MITM.md`.
- **Event 46 unreliability**: `HarvestableChangeState` occasionally skips size values or fires late depending on server batching. The radar reflects what the wire delivers; intermediate states the server skips are unrecoverable. Detail in `docs/technical/HARVEST_EVENTS.md`.
- **Some Black Zone maps**: tiles missing for zone IDs 4000+. Workaround: disable the map background in settings.
