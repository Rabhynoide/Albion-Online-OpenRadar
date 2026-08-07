# Native Overlay Client

A native, click-through radar window (`cmd/radar -overlay`) that sits on top of the game like a
picture-in-picture, alongside the existing web app - not a replacement for it.

*Last verified against code: 2026-08-07.*

## What this is (and isn't)

Started from a simple ask: make the radar window click-through once positioned over the game.
After evaluating options (an external click-through utility, Electron/Tauri, a Go+WebView2
overlay), the client's own architecture and existing web frontend made a fully native,
dependency-light path attractive - and mid-implementation, the scope was deliberately narrowed
back down: **the native window (`internal/overlay`) is a purely visual, click-through
overlay of the radar map. It is not a replacement UI.** Players/Chests/Market/Settings/
Resources/Enemies/Ignore List all stay on the existing web app (`internal/server` +
`internal/templates`), which keeps running exactly as before, in parallel, for configuration.
The overlay window only *reads* settings (`settings-sync.json`) to decide what to draw - it
never writes to them itself. (An earlier iteration added in-overlay keyboard toggles for a
handful of settings; these were removed once the actual requirement became clear - see git
history around commit `5392dd5c` if that's ever worth resurrecting.)

## Launching it

```
go run ./cmd/radar -overlay [-dev]
```

Same binary, same capture backend, same web server - `-overlay` only changes what runs on the
main thread in place of the bubbletea TUI dashboard (`internal/ui`): an `ebiten.Game` instead of
a `tea.Program`. `app.startCaptureStatePoll()` (which only exists to feed the TUI) is skipped;
everything else (`startServers`, `updateStats`, `startUpdateCheck`) runs unchanged.

## Architecture

```
cmd/radar -overlay
    │
    ├─ capture.Manager → photon.PhotonParser        (unchanged)
    │        │
    │        ├─ app.wsHandler.BroadcastEvent/...     (unchanged - still feeds the web app,
    │        │                                        e.g. Market's passive price observation)
    │        │
    │        └─ app.radarRouter.Handle{Event,Request,Response}   (new: a second, in-process
    │                 │                                           subscriber of the same
    │                 │                                           already-decoded Photon data)
    │                 ├─ internal/radarstate  (entity state + GPS pathfinding)
    │                 └─ internal/gamedata    (Items/Mobs/Harvestables/Zones, read directly
    │                                          from web/ao-bin-dumps/, no HTTP)
    │
    └─ internal/overlay (Ebiten)   (new: undecorated, transparent, always-on-top, click-through)
```

## internal/gamedata

Straight Go ports of `web/scripts/data/*Database.js`'s parsing logic, reading the exact same
`web/ao-bin-dumps/*.json(.gz)` files already committed for the web client - `LoadItems`,
`LoadMobs`, `LoadHarvestables`, `LoadZones`, `LoadZoneGraph`. No behavior changes from the JS
versions; see each file's doc comments for the specific parsing rules mirrored (e.g. `MobsDatabase.js`'s
`mobOffset` anchor, `_normalizeResourceType`'s prefix matching).

## internal/radarstate

A Go port of `web/scripts/handlers/*.js` and `web/scripts/data/{PartyRoster,ZoneGraph}.js` - one
file per handler, same method names and parameter indices, deliberately mechanical
(`internal/radarstate`'s package doc explains why: the JS is already capture-verified, this
translates known-correct logic rather than re-deriving wire format). Debug-only logging present
in the JS (the large majority of `HarvestablesHandler.js`/`MobsHandler.js`'s line count) was not
ported - it has no behavioral effect and Go's testing conventions don't need it duplicated.

**`entityList[K,V]`** (`entitylist.go`) is a small generic thread-safe store shared by every
simple handler (Chests, WispCage, MistsDungeon, LocalTreasures, Fishing, Dungeons, Players,
Harvestables, Mobs) - replaces the nearly-identical add/touch/remove/clear/cleanup/enforceMaxSize
methods each JS handler class hand-rolled.

**`Router`** (`router.go`) is the Go equivalent of `EventRouter.js`'s `onEvent`/`onRequest`/
`onResponse` switch statements - the single dispatch point wired into `cmd/radar/main.go`'s
`onPhotonEvent`/`onPhotonRequest`/`onPhotonResponse` as a second subscriber alongside the
existing WebSocket broadcast.

**Real bug caught during this port, worth remembering**: the dispatch key is
**`Parameters[252]`/`Parameters[253]`**, not `EventData.Code`/`OperationRequest.OperationCode`.
Those Go struct fields are `byte` (capped at 255), but this project's own `eventcodes`/
`operationcodes` packages define Albion event/operation IDs well past 255 (e.g.
`NewLootChest = 393`). `internal/photon`'s `PostProcessEvent`/`PostProcessRequest` only fall back
to the raw envelope byte when the wire dictionary omits 252/253 - which it normally doesn't.
`EventRouter.js` has always read `Parameters[252]` for exactly this reason
(`const eventCode = Parameters[252]`). The Go compiler caught this immediately (`untyped int
constant 393 overflows byte`) once real event codes were plugged into a `switch` on the wrong
field - `TestRouter_HandleEvent_UsesParameters252NotRawCode` guards the regression.

**Deliberate simplification**: `Session` (`session.go`) tracks the local player's own zone and
position (needed for GPS/pvp-type resolution) but not `EventRouter.js`'s fuller Mist-portal-chain
PvP-inheritance tracking (which sanctuary a Mist was entered from, across multiple hops,
`setMistOverride`/`restoreMistOverrideFromSession` and friends). A real gap if precise Mist PvP
coloring ever matters here - not yet ported.

## internal/overlay

The `ebiten.Game` implementation.

**Click-through** (`clickthrough_windows.go`): `ebiten.SetWindowMousePassthrough(bool)`, toggled
by **F9**. The interesting finding from prototyping this: once passthrough is active, the window
can *never* regain focus through a click (that's the whole point), so a normal focus-based key
listener (including Ebiten's own `inpututil`) goes deaf exactly when the user needs the toggle
back. Fixed by polling the key **globally** via `user32.dll`'s `GetAsyncKeyState`, independent of
window focus - a `syscall.NewLazyDLL` call, no prior precedent for direct Win32 syscalls in this
repo. Windows-only for now (`clickthrough_other.go` is a no-op stub so the package still builds
on Linux).

**Rendering** (`game.go`, `drawings.go`, `geometry.go`): flat colors/shapes per entity type
rather than a pixel-perfect port of `DrawingUtils.js`'s canvas gradients/rounded-rect badges -
that's UI polish this environment can't iterate on blind, so it was skipped rather than guessed
at. Every tracked `internal/radarstate` entity type is drawn: harvestables (colored squares by
resource type), mobs (colored circles by `EnemyType`), chests, dungeons (ringed by
`DungeonType`), fishing, local treasures, MistsDungeon portals, WispCages, and the local player
(fixed screen-center dot, matching `CanvasManager.js`'s own "world moves, player doesn't"
convention). **Deliberately out of scope, not a gap**: resource clustering and health bars -
confirmed not needed for this client (`DrawingUtils.js`'s `detectClusters`/`drawHealthBar` were
never ported at all, not merely left unwired). Distance indicators remain a possible follow-up
if ever wanted; `CalculateDistance`/`MetersToGameUnits`/`ConvertGameUnitsToMeters` in
`geometry.go` are kept as general-purpose primitives (not clustering-specific) in case they're
useful for that later.

**Map background** (`mapbackground.go`): the one part of `DrawingUtils.js`/`MapsDrawing.js` that
needed porting despite the "skip pixel-perfect polish" rule, since without it the radar has no
spatial context at all. `MapsDrawing.js`'s canvas transform stack
(`scale(1,-1) → translate → rotate(-0.785398) → translate → drawImage`) uses a **real**
trigonometric rotation (`ctx.rotate`), unlike `DrawingUtils.js`'s `transformPoint` (used for every
entity), which - a pre-existing quirk in the shipped JS, kept as-is rather than "fixed" - treats
the same numeric constant as a **plain scalar multiplier**, not an actual rotation. Both behaviors
are reproduced exactly as they ship today (`TransformPoint` vs `MapScreenTransform`), not
reconciled to be mathematically consistent with each other. Getting the map's own transform right
required care on a second front: **Ebiten's `GeoM` composes operations in call order** (first
`.Scale()`/`.Rotate()`/`.Translate()` call applies to the point first), which is the *opposite* of
HTML Canvas's `ctx.transform()` stack (last call applies first) - confirmed by tracing
`GeoM.Scale`/`GeoM.Translate`'s source directly rather than assuming. `MapScreenTransform` solves
the canvas math analytically in Go (verified by hand-deriving the composed matrix) rather than
attempting to mirror the canvas call sequence 1:1 in `GeoM` calls, precisely because getting that
order backward silently produces a mirrored/misrotated result with no compiler help to catch it.
Map tiles decode via `golang.org/x/image/webp` (no existing WebP decoder in this codebase) and are
cached per zone by `mapImageCache`.

**Settings** (`settings.go`): read-only. `settingsPanel.refresh()` re-reads `settings-sync.json`
once per `Update()` tick (the same file `web/scripts/utils/SettingsSync.js` write-throughs to
from the web app's Resources/Enemies/Settings pages); `Draw()` calls `isOn(settingID)` to filter
what's rendered (currently: per-`EnemyType` mob visibility via
`radarstate.SettingNameForEnemyType`). A setting never set defaults to `true` (visible), matching
the web pages' own default-checked checkboxes. The overlay never calls `syncsettings.Set` -
configuration is a one-way read from whatever the web app last wrote.

## Known limitations

- Click-through toggle (F9) is Windows-only.
- Mist-portal-chain PvP inheritance isn't tracked (see `Session`'s doc comment).
- No zoom/pan controls in the overlay window yet (`Draw()`'s `zoom` is hardcoded to `1.0`).
- The overlay only *reads* `settings-sync.json`; there's no way to set a setting from inside the
  overlay itself - by design, the web app remains the only editor (see "What this is (and
  isn't)" above).
- Resource clustering and health bars are intentionally not implemented (confirmed not needed).
