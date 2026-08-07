# Auto Update Check

How OpenRadar notices a newer release exists and tells the user, without ever silently
replacing the running binary.

*Last verified against code: 2026-08-07.*

## Approach: check-and-notify, not self-replace

At launch, the client checks whether a newer version has been published and, if so, shows a
dismissible notice on both surfaces (web sidebar, TUI dashboard header) linking to the GitHub
release. It never downloads or swaps the running executable itself - the user always decides
when and how to update.

## Repo checked

Hardcoded to `internal/updatecheck.DefaultRepo` = `Rabhynoide/Albion-Online-OpenRadar` - the
user's own fork, which is where actual published (non-draft) releases exist today (confirmed via
`gh release list`: v1.0.0, v1.0.1, v1.0.2). **Not** the upstream `Nouuu/Albion-Online-OpenRadar`.
`.github/workflows/release.yml` creates releases as drafts (`gh release create ... --draft`), so
GitHub's `/releases/latest` API only ever sees one once a human publishes it - an unfinished
draft can never trigger a false "update available".

## Where the actual check happens

`cmd/radar/main.go`'s `(app *App) startUpdateCheck(appDir string)`, a one-shot (not ticker)
background goroutine started right after `app.startCaptureStatePoll()`, mirroring that same
`app.wg.Go(...)` pattern:

1. Skipped entirely if `Version` is `""` or `"dev"` (an unversioned dev build - same convention
   `buildID()` already uses in `internal/server/http.go`).
2. Throttled to once per `updateCheckInterval` (1 hour): if the persisted `LastChecked` is more
   recent than that, the goroutine just re-evaluates the already-persisted result instead of
   calling GitHub again - same reasoning as the Hub's `marketStaleAfter` cache TTL, applied here
   to avoid hammering GitHub's API on frequent restarts.
3. Otherwise calls `internal/updatecheck.Client.FetchLatest()` (`GET
   https://api.github.com/repos/{repo}/releases/latest`). Any failure is logged
   (`logger.PrintWarn("UPDATE", ...)`) and swallowed - never blocks startup, never surfaces
   anywhere else, same "an optional external call must never disrupt the app" philosophy as the
   Hub fallback paths.
4. On success, persists `LatestVersion`/`ReleaseURL`/`LastChecked` via `capture.MutateConfig`
   into `network.json`'s new `UpdateCheck` section, and if the new version is actually newer and
   not already dismissed, sends `ui.UpdateAvailableMsg{Version: ...}` to the TUI program.

**Gotcha**: GitHub's REST API returns `403 Forbidden` for requests with no `User-Agent` header -
unlike the Albion Online Data Project API used for market prices, this is a hard requirement, not
an optional nicety. `internal/updatecheck.Client.FetchLatest()` always sets one.

## Version comparison

`internal/updatecheck.IsNewer(current, latest string) bool` - both plain `"X.Y.Z"` strings (an
optional leading `v` and any `-suffix` after the patch number are tolerated/ignored, though this
project's own release tags are bare `"1.0.2"`-style today). `current` being `""` or `"dev"`
always returns `false`. A malformed `latest` is treated as `0.0.0` (never newer) rather than
erroring - a launch-time check must never crash on unexpected input.

Deliberately **not baked into what's persisted**: `network.json` only stores the raw fact
"GitHub's latest tag is X" - "is that actually newer than what's running" is recomputed at read
time (both in the TUI goroutine and in the web API's `handleGet`) against the real running
`Version`. This means a user who updates the binary manually (without ever dismissing anything)
correctly stops seeing the notice on the very next check, with no stale-state cleanup needed.

## Persistence: `internal/capture/network_config.go`

New `Config.UpdateCheck UpdateCheckConfig` section, same shape/atomic-write machinery as
`Config.Hub`/`Config.Market`:

```go
type UpdateCheckConfig struct {
    LatestVersion    string    `json:"latestVersion"`
    ReleaseURL       string    `json:"releaseUrl"`
    LastChecked      time.Time `json:"lastChecked"`
    DismissedVersion string    `json:"dismissedVersion"`
}
```

## Web API: `internal/server/update_settings_api.go`

- `GET /api/settings/update` - reads `network.json`, returns
  `{available, currentVersion, latestVersion, releaseUrl}`. `available` is
  `updatecheck.IsNewer(currentVersion, LatestVersion) && LatestVersion != DismissedVersion`.
  Makes **no** outbound call itself - the one-per-launch GitHub call already happened in
  `cmd/radar/main.go`; this handler only ever reads the cached result, so every page load stays
  cheap regardless of how often the sidebar re-checks.
- `POST /api/settings/update/dismiss` - sets `DismissedVersion = LatestVersion` via
  `capture.MutateConfig`, so the same version doesn't reappear on a later page load or restart.
  If a later launch's check finds something newer still, `LatestVersion` moves past the
  dismissed value and the notice reappears - dismissal is per-version, not permanent.

## Frontend: `web/scripts/core/UpdateBadge.js`

`initUpdateBadge()`, called once from `internal/templates/layouts/sidebar.gohtml`'s existing
`onGlobalsReady` boot block (that template is never re-rendered on HTMX navigation, so this is a
true once-per-session init, same as `initGpsWidget()` next to it). Fetches
`/api/settings/update` once; if `available`, injects a small link (`vX.Y.Z disponible`, opens
the release page in a new tab) plus a `×` dismiss button into both sidebar footer containers
(`#sidebarVersionDesktop`/`#sidebarVersionMobile`, next to the existing `v{{.Version}}` text).
Dismissal removes the badge(s) immediately and POSTs best-effort (fire-and-forget, same pattern
as `ZoneGraph.reportTransition`) - a failed POST just means the badge reappears next launch,
never a visible error.

## TUI dashboard: `internal/ui/dashboard.go`

`UpdateAvailableMsg{Version string}` sets `Dashboard.updateAvailable`/`latestVersion`, rendered
by appending `" • update available: vX.Y.Z"` (styled with the existing `LogWarnStyle`) directly
onto the title line inside `renderHeader()`, rather than adding a new line. This was a deliberate
choice: an extra header line would require recalculating `headerHeight`, and this session had
already hit a real `slice bounds out of range` viewport panic from a header/footer height
mismatch - appending inline sidesteps that risk entirely.
