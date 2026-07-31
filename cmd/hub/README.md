# OpenRadar Hub

The **OpenRadar Hub** is a small self-hostable service that lets a group of radar clients
pool discovered Avalonian Road edges into one shared, SQLite-backed database instead of each
player only ever seeing their own local discoveries. One Hub instance per friend group; each
radar client points at whichever Hub it wants (or none) via Settings.

## Why

Avalon Road connections discovered while playing are persisted locally per user by default
(`roads.json`, written by `internal/roads`, served to the browser via
`internal/server/roads_api.go`, consumed by `web/scripts/data/ZoneGraph.js`). The Hub adds an
optional shared layer on top: the radar's Go backend relays `GET`/`POST /api/roads/edges` to
the configured Hub when one is enabled, and falls back to the local `roads.json` when the Hub
is disabled, unreachable, or not configured. The browser-facing API never changes. See
[`docs/technical/AVALON_ROADS_GPS.md`](../../docs/technical/AVALON_ROADS_GPS.md#the-hub-shared-roads-across-a-group)
for the full design.

## Code layout

- `cmd/hub/main.go` — entry point. Reads `PORT` (default `8090`), `DB_PATH` (default
  `/data/hub.db`), and requires `HUB_SECRET` (refuses to start without one).
- `internal/hub/store.go` — SQLite-backed edge store (`modernc.org/sqlite`, pure Go, no CGO).
- `internal/hub/auth.go` — shared-secret header check (`X-Hub-Secret`).
- `internal/hub/api.go` — `GET/POST /api/roads/edges` (secret required) and `GET /health`
  (unauthenticated, for container healthchecks).

Both live in the **same Go module** as `cmd/radar` (`github.com/nospy/albion-openradar`) — no
separate `go.mod`/workspace.

## Running the Hub

### Option A: plain `docker run`

```sh
docker build -f Dockerfile.hub -t openradar-hub .
docker run -d \
  -e HUB_SECRET=your-group-secret \
  -v openradar-hub-data:/data \
  -p 8090:8090 \
  openradar-hub
```

### Option B: `docker compose`

[`docker-compose.hub.yml`](../../docker-compose.hub.yml) at the repo root does the same thing
in one command - handier for a long-running deployment (`restart: unless-stopped`, named
volume, single place to edit the port/secret):

```sh
echo "HUB_SECRET=your-group-secret" > .env
docker compose -f docker-compose.hub.yml up -d --build
```

Both options put `hub.db` inside a named Docker volume, so it survives container
restarts/upgrades. Back it up with `docker run --rm -v openradar-hub-data:/data -v "$PWD":/backup alpine tar czf /backup/hub-backup.tar.gz -C /data .` (adjust the volume name to
`<project>_hub-data` if using compose - check with `docker volume ls`); restore by extracting
that tarball back into a fresh volume the same way, mounted at `/data`.

### Option C: Portainer (Stacks)

Reuses the same [`docker-compose.hub.yml`](../../docker-compose.hub.yml):

1. **Stacks → Add stack**, name it (e.g. `openradar-hub`).
2. **Build method: Repository**.
3. **Repository URL**: this repo's git URL. If private, add credentials (a GitHub PAT) under
   "Authentication".
4. **Repository reference**: the branch to deploy (a stable branch, not a work-in-progress one).
5. **Compose path**: `docker-compose.hub.yml`.
6. **Environment variables** (the stack form has a dedicated section - no committed `.env`
   needed): set `HUB_SECRET` to your group secret.
7. Optionally enable GitOps updates/webhook to auto-redeploy on push to that branch.
8. **Deploy the stack**.

Portainer clones the repo, so `Dockerfile.hub`'s `context: .` resolves to the repo root as
expected. Check **Containers → hub → Logs** for `OpenRadar Hub listening on :8090`.

If you'd rather not have Portainer build the image itself (slower, or a private repo you don't
want to hand credentials to), build and push the image to a registry yourself (Docker Hub,
GHCR) and replace `build:` with `image: <your-registry>/openradar-hub:latest` in the stack -
Portainer then just pulls it.

### Exposing it over HTTPS

The Hub itself only speaks plain HTTP - always put a reverse proxy in front if it's reachable
over the internet rather than just your group's LAN/VPN. Two common options, both terminating
TLS and forwarding to `127.0.0.1:8090`:

**Caddy** (`Caddyfile`, automatic Let's Encrypt certs):
```
hub.yourdomain.com {
    reverse_proxy 127.0.0.1:8090
}
```

**nginx** (assumes a cert already provisioned, e.g. via `certbot`):
```nginx
server {
    listen 443 ssl;
    server_name hub.yourdomain.com;

    ssl_certificate     /etc/letsencrypt/live/hub.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/hub.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
    }
}
```

Point radar clients at `https://hub.yourdomain.com` instead of the bare `http://host:8090` once
this is in place. The shared secret still travels in a request header either way, so serving
over plain HTTP on a trusted LAN/VPN (no reverse proxy at all) is a reasonable choice too if the
Hub never needs to be internet-reachable.

Then, on each radar client, go to **Settings → Hub (Shared Roads)**, enable it, and set the
Hub URL and the same shared secret.

## Explicitly out of scope (v1)

- Per-user accounts/attribution - one shared secret per group, not individual logins.
- A market-price database (mentioned as a possible later, separate effort - not started).
- Conflict resolution smarter than last-write-wins upsert.
