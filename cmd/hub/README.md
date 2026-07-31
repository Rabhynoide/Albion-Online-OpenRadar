# OpenRadar Hub (planned)

This directory reserves the location for the future **OpenRadar Hub** — a shared backend
that multiple radar clients submit community discoveries to and fetch an aggregated,
crowd-sourced database from. **Not implemented yet** — no working code lives here yet.

## Why

Today, Avalon Road connections discovered while playing are persisted locally per user
(`roads.json`, written by `internal/roads`, served to the browser via
`internal/server/roads_api.go`, consumed by `web/scripts/data/ZoneGraph.js`). That works for
a single player but doesn't let a group of friends pool what each of them has discovered.
The Hub will let many radar instances submit their discoveries to one shared service and pull
back everyone else's — first for Avalon Roads, later (planned, not started) for a
market-price database following the same submit/aggregate/serve shape.

## Where future code will live

- `cmd/hub/main.go` — entry point, once it exists.
- `internal/hub/...` — Hub-specific packages (API handlers, storage, aggregation logic).

Both live in the **same Go module** as `cmd/radar` (`github.com/nospy/albion-openradar`) —
no separate `go.mod`/workspace. That keeps things simple (one dependency graph, one `go.sum`)
and lets Hub code freely import existing `internal/...` packages where it makes sense (e.g.
the `roads.Edge` shape as a starting point for the shared schema), without affecting the
radar client binary: Go only links what a given `main` package actually imports.

## Explicitly not decided yet

- Datastore (SQLite vs. Postgres vs. something else)
- Authentication / abuse prevention for a public-ish endpoint
- Deployment target (where this actually runs)
- CI/release pipeline integration (`Makefile` targets, Docker image, GitHub Actions job)

These get designed together when the roads-sharing feature itself is planned — this
directory just marks where that work will land.
