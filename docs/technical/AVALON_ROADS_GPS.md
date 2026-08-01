# GPS / Avalon Roads Detection

How OpenRadar's GPS feature builds a zone-connectivity graph, learns Avalon Road connections
by play, and what it deliberately doesn't know.

*Last verified against code: 2026-07-30.*

## Detection surface

| Piece | Source | Path |
|---|---|---|
| Static open-world adjacency | `cluster/world.json`'s `exits` (`@targettype: "Cluster"` only) | `tools/update-ao-data.ts` (`extractClusterEdges`, `filterRoutableEdges`) -> `web/ao-bin-dumps/zone-graph.json` |
| Discovered Avalon Road edges | Live zone transitions the player actually takes | `web/scripts/core/EventRouter.js` (`applyMapChange`) -> `web/scripts/data/ZoneGraph.js` (`reportTransition`) -> `internal/roads` (`roads.json`) via `internal/server/roads_api.go` |
| Pathfinding + UI | Merges both graphs client-side | `web/scripts/data/ZoneGraph.js` (`getNextHop`) -> GPS card on `internal/templates/pages/radar.gohtml` |

## Static graph: what's routable and what isn't

`cluster/world.json` exits come in two `@targettype` flavors: `"Cluster"` (a fixed, real
neighboring zone) and `"DungeonGroup"` (leads into an instanced/randomized destination - same
problem class as Avalon Roads). Only `"Cluster"` exits are extracted.

That's not sufficient on its own: static dungeons (including Hellgates and corrupted
dungeons - every such `type` string contains `"DUNGEON"`) are *also* `"Cluster"`-type targets
in the raw data, despite not being freely walkable open-world content. Confirmed via a real
bug report: the GPS was routing players *through* a dungeon (`Deepwood Enclave`,
`DUNGEON_YELLOW`) as a waypoint. `isRoutableZoneType()` in `tools/update-ao-data.ts` excludes
by zone `type` substring: `DUNGEON`, `HIDEOUT`, `ISLAND`, `ARENA`, `EXPEDITION`. Regenerating
against real data dropped the graph from 2381 to 1632 edges. `TUNNEL_*` types (Roads of
Avalon, all tiers) are deliberately *not* excluded - they're real walkable roads, just with
zero static connectivity data (see below).

**Zone ids are not always numeric** (e.g. `TNL-001`, `ISLAND-GUILD-0001a`) - don't assume an
integer zone id anywhere in this pipeline.

## Avalon Roads have zero static connectivity data

Confirmed by inspecting the generated graph directly: **no edge in `zone-graph.json` touches
any `TNL-*` zone** - not in, out, or between tunnel zones. This makes sense: the tunnel
network is randomized/reshuffled periodically, so a static dump can't encode "TNL-001 connects
to TNL-004 right now." The entrance from open world into a road, every hop between tunnel
segments, and the exit back to open world are *all* only discoverable by a client that
actually walks through them - there's no way around live discovery for this part.

## Discovered edges: bidirectional-by-assumption, with staleness

`ZoneGraph.reportTransition(from, to, pos)` fires on every real zone change (`EventRouter.js`)
and does two things:
1. Records `from -> to` as a confirmed `"discovered"` edge (persisted server-side via
   `POST /api/roads/edges`).
2. Also synthesizes `to -> from` as an **`"assumed"`** edge (client-side only, never persisted)
   - "we know you got here, we don't know you can get back," motivated by a real user request
   to have the GPS suggest a U-turn when entering an unexplored road. A real observation in
   either direction always overwrites an assumed one; assumed edges never downgrade a
   confirmed one.

`getNextHop()` runs an unweighted BFS twice: first restricted to reliable edges (static +
fresh confirmed discoveries), falling back to also allowing stale/assumed edges only if no
fully-reliable path exists. The returned hop is flagged `stale`/`assumed` so the UI can say so
(`"includes an unconfirmed U-turn"` etc.) rather than presenting a guess as fact.

**Staleness threshold: 24h** (`STALE_MS` in `ZoneGraph.js`), not guessed - confirmed from real
game-data files, see below. An assumed edge's exit position is always `null`: the return
exit's position lives in the *other* zone's local coordinates, which `reportTransition` never
observes (it only knows the pre-transition position, in the zone being left).

## Manual removal (issue #5)

The 24h staleness window is a passive fallback, not a guarantee - a Road of Avalon can reset
to a different connection well before that, and the GPS would keep confidently suggesting a
dead exit in the meantime. The sidebar GPS widget's "Remove this route" button (shown
whenever a live `getNextHop()` result is on screen) calls `ZoneGraph.removeEdge(from, to)` for
exactly that edge:

- Removes the edge from the in-memory adjacency map immediately, so the next `renderResult()`
  recomputes with it gone (or falls through to "no known route").
- Best-effort `DELETE /api/roads/edges` to the backend (mirrors `reportTransition`'s
  best-effort `POST`), which prunes it from `roads.json` and forwards the deletion to the Hub
  if one is configured (see `internal/roads.RemoveEdge`, `internal/hub.Store.DeleteEdge`).
- Also drops the assumed reverse if it was only ever inferred from the edge being removed
  (never touches a separately-confirmed reverse edge).
- No-ops on static (open-world) edges - those aren't persisted through this API and don't
  expire, so removing one in memory would just be undone by the next `load()`. Currently not
  distinguished in the UI (the button doesn't know a hop's source), so clicking it on a static
  hop is a silent no-op rather than a visible error - acceptable since the button only matters
  in practice for Avalon Road hops, which are never static.

## Roads of Avalon constants (from `ao-bin-dumps`, not the wire)

The per-instance exact remaining lifetime of a specific road/portal is **not observable** -
extensively checked directly on the wire (see `PROTOCOL18_PARAM_LAYOUTS.md`'s event 325
section) and confirmed absent. It's a value rolled server-side at spawn and never transmitted
to the client in anything captured.

What *is* confirmed, from files this project doesn't currently fetch
(`tools/update-ao-data.ts` only pulls 5 of 400+ files in the `ao-bin-dumps` repo -
`items`/`mobs`/`spells`/`harvestables`/`cluster/world.json`):

- **`tunnelsystem.json`** -> `tunnelexitsettings`: `@lifetimemin="43200"` (12h),
  `@lifetimemax="86400"` (24h), `@shutdownwarningtime="3600"` (1h warning). This is where
  `STALE_MS = 24h` comes from - the confirmed upper bound, replacing an earlier 3-day guess.
- Same file, `tunnelexittypes`: `SOLO` has `@maxcharges="7"` and 300s (5min) per charge -
  matches the in-game "7/7" usage counter and its ~5-minute recharge countdown exactly.
- `tunnelexitpresets`: named presets (`OUT_ROYAL_SAFE`, `OUT_BLACK_HIGH`, `OUT_TUNNEL_ROYAL`,
  ...) with weighted `outgoingConnectionWeighting` - the actual probability-weighted rule set
  for where a road connects next. Not currently used by the GPS (still server-rolled per
  instance, no target zone id given), but could inform a future probabilistic suggestion.
- **`randomdungeons.json`**: `@maxlifetimeseconds="87000"` (~24h10, separate system from
  tunnels), plus `@lifetimeafteremptyseconds` / `@lifetimeafterlastuse` per dungeon entry.

Other unpulled files spotted as promising for *other* future features (not investigated
further): `corrupteddungeons.json`, `helldungeons.json`, `mists.json` /
`miststemplatemetadata.json` (relevant to `MISTS_DETECTION.md`), `outlandsteleportationportals.json`,
`staticdungeons.json`, `worldbosses.json`, `worldsettings.json`.

## Compass direction is screen-relative, not true north

The GPS shows the next exit's direction as `N`/`N-E`/`E`/... (`relativeScreenBearing` +
`bearingToCompassLabel` in `web/scripts/utils/DrawingUtils.js`). This is computed using the
*same* rotation the radar canvas itself uses to render (`transformPoint`'s `SCREEN_ROTATION`)
- it matches what you see on your own radar, but is **not** the game's own compass convention.

That was a deliberate, tested decision: the in-game world-map UI shows exit directions too
(e.g. "Sortie S-O vers Creag Dhor"), and an attempt was made to reverse-engineer that exact
convention from confirmed zone-exit position data (multiple real screenshot + zone-id data
points, cross-checked against 16 different rotation/axis-swap formulas). None matched more
than 2 of 4 confirmed points - the in-game direction is most likely computed from each zone's
position on the regional/continental world map (data this project doesn't have), not from the
exit's local position within its own zone. Don't re-attempt this without new data (e.g. a
source for zone-to-zone regional layout) - it's a dead end via the data available today.

## Local Treasures (`LocalTreasuresUpdate`, event 285)

A related but separate detection, added alongside the GPS work: `web/scripts/handlers/LocalTreasuresHandler.js`
renders buried treasure chests, temporary resource nodes, smuggler piles, and timed world
events found via the same event that motivated the ticks-format investigation above. See
`PROTOCOL18_PARAM_LAYOUTS.md`'s event 285 section for the wire layout and why
`SPECIAL_EVENT_*` labels are excluded (they duplicate the existing mob detection).

## The Hub: shared roads across a group

Everything above (`internal/roads`, `roads.json`, `ZoneGraph.js`) works standalone,
per-user, local-only. On top of that, `cmd/hub` (see
[`cmd/hub/README.md`](../../cmd/hub/README.md)) is a small self-hostable service that lets a
group of players pool discovered road edges into one shared database instead of each person
rediscovering the same roads independently.

**Architecture**: the browser's `GET`/`POST /api/roads/edges` calls never change. What changes
is what the radar's own Go backend does with them (`internal/server/roads_api.go`), when a Hub
is configured (`capture.Config.Hub`, persisted in `network.json`, set via Settings → Hub):
- `POST`: writes to the local `roads.json` as always, then best-effort forwards the same edge
  to the Hub. A Hub failure never fails the browser's request - the local write already
  succeeded.
- `GET`: tries the Hub first; on any failure (unreachable, timeout, non-200) falls back to the
  local `roads.json`, exactly like today.

This keeps the browser out of CORS/secret-management entirely (the Hub URL and shared secret
never reach client-side JS or DevTools) and keeps `roads.json` working as an always-available
offline cache even when the Hub is down.

**The Hub itself** (`internal/hub`) is SQLite-backed (`modernc.org/sqlite`, pure Go, no CGO -
keeps the Docker image tiny and static) and ships as a single container
(`Dockerfile.hub`) - one Hub per friend group, each pointed at from Settings independently.
Auth is a single shared secret (`X-Hub-Secret` header), not per-user accounts - "a group
password," matching the scope of "friends sharing discoveries," not a public service.
