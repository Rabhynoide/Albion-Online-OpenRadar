# Native Settings Client

A native, browser-free configuration app (`cmd/radar-settings`, Fyne) covering everything the web
app's Players/Resources/Enemies/Chests/Ignore List/Settings pages did, plus a button to launch the
native map overlay (`docs/technical/NATIVE_OVERLAY_CLIENT.md`) alongside it.

*Last verified against code: 2026-08-07.*

## What this is (and isn't)

Follows directly from OVERLAY-1: once the map had a native click-through window, the natural next
question was "why does configuring it still need a browser?". This client answers that - it's the
new primary way to run OpenRadar day to day, not an add-on. Market stays on the web app for now
(deferred, see "Known limitations"); everything else moved here.

## Launching it

```
go run ./cmd/radar-settings -dev
```

or, from a build (see `make build-windows`):

```
OpenRadarSettings-windows-amd64.exe
```

`radar.exe`/`OpenRadar-windows-amd64.exe` must sit **next to** the settings binary - the "Lancer
l'overlay" button looks for it there first, falling back to the working directory (so `go run`,
whose own binary lives in a throwaway build-cache path, still finds a `radar.exe` built into the
project root). See `overlay_control.go`'s `radarExecutablePath`.

## The finding that shaped this whole client: Ebiten and Fyne can't share a binary

The original plan (written before any of this was tried) called for a single `cmd/radar` binary
with `-overlay` and `-settings` flags choosing between Ebiten and Fyne at runtime, same pattern as
the existing `-overlay`/TUI split. That doesn't work, and the failure mode is worth recording
because it's not obvious from either library's docs:

**Merely linking both Ebiten and Fyne into the same Windows executable breaks Fyne's window
creation - even if Ebiten is never actually used at runtime.** A minimal repro nailed it down: a
binary with `app.NewWithID(...)` + `NewWindow(...)` + `ShowAndRun()` and nothing else opens fine;
the exact same three lines, plus a single unused `import _ "github.com/hajimehoshi/ebiten/v2"`,
fails every time with `PlatformError: Win32: Failed to register window class: La classe existe
déjà` and no window ever appears (confirmed via `EnumWindows`/`MainWindowTitle` - the process
looked alive and "responding" the whole time, which made this look at first like a transient/
timing issue rather than a hard architectural conflict). The two libraries' Win32 windowing layers
collide at the `RegisterClassEx` level as soon as both are linked in, regardless of which one's
code path actually executes.

Consequence: `cmd/radar-settings` must never import `internal/overlay` (or anything that imports
it), full stop - not "don't run them at the same time," but "don't link them together at all."
That's why the map overlay is launched as a **separate OS process** (a real, different `.exe`),
not a mode switch within one process.

## Architecture

```
cmd/radar-settings                       (new binary - Fyne, no Ebiten import anywhere in its tree)
    │
    ├─ internal/radarapp.App              (capture + HTTP/WS server, shared bootstrap - see below)
    │        go app.StartServers()        → still serves Market's page + WS (web-only for now)
    │
    ├─ internal/settingsui (Fyne)         → main-thread event loop (blocking, like overlay.Run)
    │        Players/Resources/Enemies/Chests/Ignore List/Settings, all writing straight to
    │        internal/syncsettings / capture.Config - no HTTP loopback, same process
    │
    └─ "Lancer l'overlay" button → exec.Command("radar.exe", "-overlay", "-no-server")
             (a DIFFERENT binary/process - cmd/radar, which does import internal/overlay/Ebiten;
             -no-server skips its own HTTP server since this process already runs one; its own
             capture.Manager runs independently - a cheap, accepted duplicate passive pcap listener)
```

## internal/radarapp (new)

Extracted from what used to be inline in `cmd/radar/main.go`: network interface enumeration/
resolution, `capture.Manager`/HTTP server construction, Photon dispatch, stats, graceful shutdown.
Deliberately has **no dependency on any GUI toolkit** - no `bubbletea`, no `internal/overlay`, no
`internal/settingsui` - specifically so both `cmd/radar` and `cmd/radar-settings` can import it
without either dragging the other's windowing library along (see the finding above).

`radarapp.App` exposes `OnEvent`/`OnRequest`/`OnResponse` hooks so a caller can observe
already-decoded Photon data as a second subscriber alongside the WebSocket broadcast, without this
package needing to know what's on the other end (`internal/radarstate`, `internal/overlay`,
whatever). `cmd/radar` wires these to its `radarstate.Router` in `-overlay` mode; `cmd/radar-
settings` doesn't wire them at all today (no radarstate consumption - Market's passive price
observation, the one feature that would need it, is deferred, see below).

`New()` returns the resolved target interfaces without starting capture (`app.StartCapture`
does that separately) - `cmd/radar`'s `-overlay` mode needs to register its `OnEvent` hooks
*before* packets start flowing, so capture can't auto-start inside `New()`.

`cmd/radar`'s own `App` struct now embeds `*radarapp.App` and layers TUI/overlay-specific state
(`program *tea.Program`, `radarRouter`, `overlayState`) on top - see `cmd/radar/main.go`.

## internal/settingsui (Fyne)

One file per page, same pattern as `internal/overlay`'s one-file-per-concern layout:

- **`store.go`** - `Store`, a Go equivalent of `web/scripts/utils/SettingsSync.js`'s typed
  accessors (`GetBool/SetBool/GetNumber/SetNumber/GetString/SetString/GetJSON/SetJSON`) over
  `internal/syncsettings` (already existing, same file the web app and the overlay's read-only
  `settingsPanel` use). `Clear()` is a genuine improvement over the web Settings page's own
  "Reset Settings" button, which only ever cleared browser `localStorage`, never
  `settings-sync.json` itself - a latent gap noted while porting, fixed here since this client
  has direct file access instead of a browser sandbox.
- **`players.go`/`chests.go`/`ignorelist.go`** - straightforward 1:1 ports of their `.gohtml`
  pages' checkbox/list bindings.
- **`resources.go`** - the tier(1-8)×enchant(E0-E4) grid per resource type (`ResourcesHelper.js`
  port), E1-E4 only exposing tiers 4-8 like the web version. Also see the `ShouldRender` fix
  below - this grid was silently not filtering anything in the overlay until this phase.
- **`enemies.go`** - toggles plus the `all`/`bosses`/`miniboss`/`clear` presets
  (`applyEnemyPreset` port) and the "All" checkbox's two-way sync with the four classic enemy
  toggles. Preset application uses a **silent** checkbox update (bypassing `OnChanged`) in a
  **fixed** key order, not a map range - an early version drove state changes through the
  checkboxes' own change-cascade in Go map-iteration order, which is randomized per run and could
  nondeterministically clobber a preset's own values (e.g. "Bosses" sometimes leaving
  `settingBossEnemy` reset back to false depending on iteration order that run).
- **`settings.go`** - the biggest page. `syncsettings`-backed sections (Display, Clusters,
  Logging, WS performance, the new Overlay zoom/opacity controls) go through `Store`;
  `capture.Config`-backed sections (Network, Hub, Market region, Debug/pcap) call
  `capture.ReadConfig`/`MutateConfig` and `deps.Capture` directly - no HTTP round-trip to itself,
  since this is all one process now.
- **`overlay_control.go`** - the "Lancer l'overlay" child-process launcher (see architecture
  above).
- **`theme.go`/`nav.go`** - a custom `fyne.Theme` approximating the web app's dark navy/blue
  DaisyUI palette (background/panel/primary-blue color overrides only - Fyne's theme system
  doesn't reach per-widget rounding/shadows/icons the way the web app's Tailwind CSS does) and a
  left sidebar nav (swap-on-click content stack) replacing an earlier top-tabs layout, closer to
  the web app's own left nav. No icon set - Lucide (what the web app uses) isn't available as
  Fyne resources without a separate asset-conversion pass, not done here.
- **`run.go`** - `Deps` (what `cmd/radar-settings/main.go` hands over - mirrors
  `cmd/radar/overlay_setup.go`'s `buildOverlayState` pattern) and `Run`, the blocking entry point.

## Fix found during manual testing: Resources page filter never applied in the overlay

`internal/radarstate/harvestables.go`'s `getEnchantGrid` was wired for exactly one thing:
`maybeAlertResource`'s sound-alert gate, correctly mirroring `HarvestablesHandler.js` (which only
ever gated the sound alert too). What got missed: the web app's **visual** filter is a *separate*
gate, in `HarvestablesDrawing.js`, via `LivingResourceFilter.js`'s `shouldRenderStaticResource`/
`shouldRenderLivingResource` - a different file from the handler entirely. `internal/overlay`'s
`Draw()` never ported that second gate, so it rendered every tracked harvestable unconditionally,
regardless of what was checked on the Resources page. Fixed with a new
`HarvestablesState.ShouldRender(h Harvestable) bool`, reusing the same injected `getEnchantGrid`
function and living/static key-map split `maybeAlertResource` already had, called from
`internal/overlay/game.go`'s `Draw()` before drawing each harvestable. Nothing renders for a
tier/enchant cell nobody has checked - matching the Resources page's own all-unchecked-by-default
grid (a fresh install shows *zero* resources until the user opts in, same as the web app always
did).

## Overlay window: move, resize, zoom, opacity

Added in this phase, living in `internal/overlay` (not this package) since it's overlay-window
chrome, but driven by settings this package's Settings page edits:

- **Move/resize**: the overlay window is undecorated (no OS title bar/border), so drag-to-move and
  corner-drag-to-resize are hand-implemented in `Game.updateDrag` (`internal/overlay/game.go`).
  The move-drag anchor is tracked in **absolute screen coordinates**
  (`ebiten.WindowPosition() + ebiten.CursorPosition()`), not window-relative cursor position alone
  - a window-relative-only delta oscillates as soon as the window itself moves out from under an
  otherwise-stationary cursor, since `CursorPosition()` is relative to the window's *current*
  top-left. Resize is **always kept square** (radar view math assumes one shared `canvasCenter`);
  position/size persist to `overlay-window.json` (a small dedicated file, not `settings-sync.json`
  - this is window chrome nobody else needs to read/write, unlike every other overlay-read
  setting, see `windowstate.go`'s doc comment for why that's not a violation of "the overlay never
  writes shared settings").
- **Zoom scales with window size**: resizing rescales the view instead of cropping it - the same
  maximum detection range stays visible at any window size. `settingOverlayZoom` (Settings page)
  multiplies on top of that, defaulting to 1.0.
- **Map opacity**: `settingOverlayMapOpacity` (0-100, default 100) applied via
  `ebiten.DrawImageOptions.ColorScale.ScaleAlpha` in `drawMapBackground`.

## Known limitations

- **Market stays web-only.** Its passive price-observation feature (`MarketHandler.js`, issue
  #23) isn't ported to Go, and porting it would need `cmd/radar-settings` to consume
  `internal/radarstate` (specifically zone tracking, to resolve which city a listing belongs to) -
  not wired in this phase. `cmd/radar-settings` still runs the HTTP/WS server so the Market page
  keeps working exactly as before in a browser.
- **Web pages not yet removed.** Radar/Players/Resources/Enemies/Chests/Ignore List/Settings all
  still exist in `internal/templates/pages/` and are reachable in the browser, now fully redundant
  with this client. Removal is pending manual validation of each native page (see
  `docs/project/TODO.md`'s SETTINGS-1 entry) - once confirmed, delete the `.gohtml` + route +
  nav entry + now-dead JS per page, leaving only `market.gohtml`.
- **No embedded map view in this client.** Discussed and deliberately deferred: rendering the
  radar map inside `cmd/radar-settings` itself (as an alternative to launching the separate
  overlay window) would need the Ebiten-free parts of `internal/overlay` (state aggregation,
  interpolation, geometry math) extracted into a *third* shared package neither `internal/overlay`
  nor `internal/settingsui` currently has, plus an entirely separate Fyne-based renderer (Fyne has
  no per-frame game loop and no arbitrary image rotation like Ebiten's `GeoM.Rotate`, so the map
  background would need either a non-rotated fallback or hand-rolled pixel rotation). Real, doable
  work - just not started.
- **Linux build not wired up for this binary.** `cmd/radar-settings` needs a C/GL toolchain at
  build time (Fyne's `go-gl`/`glfw` bindings, unlike `cmd/radar`'s ebiten/purego path which needs
  neither) and actual X11/OpenGL runtime libraries at runtime - `Dockerfile.linux`'s `FROM
  scratch` final stage strips those. `make build-windows` builds both binaries natively on
  Windows; Linux packaging is a follow-up.
