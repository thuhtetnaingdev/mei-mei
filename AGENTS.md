# meimei — Agent Guide

Sing-box multi-node proxy management system with three apps: `panel_backend` (Go/Gin control plane + SQLite), `node_backend` (Go/Gin node agent), `panel_frontend` (React/TypeScript + Vite).

## Commands

```
make panel-backend          # go run panel_backend/cmd/server/main.go  (:8080)
make node-backend           # go run node_backend/cmd/server/main.go   (:9090)
make frontend               # npm run dev  (:5173)
make stop-all               # kills listeners on :8080 :9090 :5173
make build-panel/node       # go build ./...  (no output path)
make release-{app}          # builds CGO_ENABLED=0 tarball to dist/
make release-all            # all linux amd64+arm64 tarballs + frontend
```

Test (no Makefile target):
```sh
go test ./...               # in panel_backend/ or node_backend/
```

## Entrypoints & structure

| App | Entrypoint | Dir |
|---|---|---|
| panel_backend | `cmd/server/main.go` | `panel_backend/internal/{api,config,db,models,services,auth,subscription}` |
| node_backend | `cmd/server/main.go` | `node_backend/internal/{api,config,services,singbox,auth}` |
| panel_frontend | `src/main.tsx` → `App.tsx` | `src/{api,components,pages,layouts,types,utils,web3}` |

Go sidecar commands (built into the same binary):
- `panel_backend reset-accounting --yes` — reset bandwidth/miner/mint state
- `go run cmd/migrate_sqlite/main.go` — Postgres → SQLite migration (requires `SOURCE_DATABASE_URL` + `TARGET_DATABASE_PATH` env)

Frontend build: `npm run build` runs `tsc -b && vite build`.

## Config quirks

- Both backends load `.env` via godotenv. `panel_backend` requires `JWT_SECRET` and `NODE_SHARED_TOKEN` (fatals if missing). `node_backend` requires `NODE_NAME`, `NODE_TOKEN`, `CONTROL_PLANE_SHARED_TOKEN`.
- Panel bandwidth collector polls nodes every **30s** (hardcoded in main.go). Node bandwidth tracker monitors sing-box V2Ray gRPC API every **60s**.
- User classification scheduler runs every **24h**. Reality key verification every **6h** (env `REALITY_KEY_VERIFICATION_INTERVAL_HOURS`, auto-fix via `REALITY_KEY_AUTO_FIX_ENABLED`).
- panel_backend can serve the frontend bundle when `FRONTEND_DIST_DIR` is set.
- Node config apply is **debounced** (rapid changes coalesced).

## Framework / toolchain details

- **panel_backend**: Go 1.25, Gin, GORM, SQLite (via `github.com/glebarez/sqlite`), JWT auth
- **node_backend**: Go 1.24, Gin, xtls/xray-core (sing-box stats gRPC client)
- **panel_frontend**: React 19, react-router-dom v7, Vite 6, Tailwind 3, Axios
  - Axios interceptor auto-prefixes `/api/` to relative requests and attaches `Bearer` JWT from `localStorage`.
  - 401 responses (non-auth) trigger session-expired notification.
- Install/deploy scripts in `install/` (panel.sh, node.sh, node-local.sh). CLI tool at `install/mei.sh`.
- Release CI (`.github/workflows/release.yml`) on `v*` tags. Builds sing-box 1.13.3 with build tags: `with_gvisor with_quic with_dhcp with_wireguard with_utls with_acme with_clash_api with_tailscale with_ccm with_ocm with_v2ray_api`. Downloads sing-box source and cross-compiles separately.

## Architecture notes

- panel_backend is the control plane — manages users/nodes/miners/subscriptions, pushes config to node_backend instances via HTTP.
- Clients connect **directly** to node_backend instances (not through the panel).
- Node registration requires `NODE_SHARED_TOKEN` (shared secret) plus a per-node `NODE_TOKEN`.
- Bandwidth collection is **pull-based**: panel polls nodes at `/bandwidth-usage` (authenticated with `X-Control-Plane-Token`).
- node_backend reports bandwidth usage to panel at `/api/nodes/bandwidth-report` (push-based, authenticated with Bearer node token + `X-Control-Plane-Token`).
- Protocol settings changes auto-sync to all nodes (best-effort).
