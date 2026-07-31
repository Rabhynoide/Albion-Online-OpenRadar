# OpenRadar — Project Context for Claude

This file is committed and shared by every contributor (see `.gitignore`: only
`CLAUDE.local.md` is personal/uncommitted). It's the entry point — keep it short and link out
to `docs/` for depth rather than duplicating detail here.

## What this repo is

A monorepo with two components sharing one Go module (`github.com/nospy/albion-openradar`):

- **`cmd/radar/`** — the shipped OpenRadar client: passively captures Albion Online's Photon
  network traffic (UDP 5056) and renders a live web radar. Go backend + vanilla-JS frontend
  embedded into the binary. This is the only part that's actually released today.
- **`cmd/hub/`** — **planned, not implemented yet.** A future shared backend multiple radar
  clients submit discoveries to (Avalon Road connections first, market prices later). See
  [`cmd/hub/README.md`](cmd/hub/README.md) before starting any work there.

Start with [`docs/README.md`](docs/README.md) — the full documentation index. Two docs worth
knowing about specifically before touching protocol or GPS/roads code:

- [`docs/technical/PROTOCOL18_PARAM_LAYOUTS.md`](docs/technical/PROTOCOL18_PARAM_LAYOUTS.md) —
  wire-format findings per event code, including the .NET-ticks timestamp encoding and the
  `LocalTreasuresUpdate`/`NewRandomDungeonExit` parameter layouts (reverse-engineered from live
  captures, not guessed — re-derive nothing here without checking first).
- [`docs/technical/AVALON_ROADS_GPS.md`](docs/technical/AVALON_ROADS_GPS.md) — how the GPS
  feature's zone graph, road-discovery, and staleness logic work today, and what the planned
  Hub changes about it.

## Conventions

- Tests: Go (`go test ./...`, table-driven, `_test.go` beside the code) and Vitest
  (`npm run test`, `web/scripts/**/_*.test.js` — the leading underscore matters, it's how
  `embed_prod.go` excludes test files from the shipped binary; see
  `docs/dev/DEV_GUIDE.md` "Asset embedding").
- Lint: `npx eslint web/scripts/ internal/templates/ tools/` and `golangci-lint` (Go).
- Full dev setup, build system, Makefile targets: `docs/dev/DEV_GUIDE.md`.
- Roadmap / open issues / what's currently working: `docs/project/TODO.md`.

## Keeping this useful

When a session turns up a real, hard-won finding (a wire-format detail, a game-data constant,
an architectural decision and why) — write it into the relevant `docs/technical/*.md` file (or
add a new one following the existing ones' style), not just into your own memory. Personal
Claude memory doesn't cross machines or contributors; these docs do. That's the whole point of
this file existing and being committed.
