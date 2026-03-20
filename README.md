# Sing-box Multi-Node Proxy Management System

This repository contains a scalable control-plane/data-plane proxy management system built around the `sing-box` core.

## Apps

- `panel_backend`: Go control plane for users, nodes, subscriptions, and sync
- `node_backend`: Go node agent for applying `sing-box` config and reporting status
- `panel_frontend`: React + TypeScript admin panel

## Architecture

- Admins use `panel_frontend`
- `panel_frontend` talks to `panel_backend`
- `panel_backend` stores state in SQLite
- `panel_backend` pushes user config to all `node_backend` instances
- Clients connect directly to the registered nodes, not the panel

## Quick Start

1. Configure environment variables using the included `.env.example` files.
   `panel_frontend/.env.example` points the UI at the control plane API.
2. Run `go mod tidy` in `panel_backend` and `node_backend`.
3. Run `npm install` in `panel_frontend`.
4. Start all three services.

## One-Line Install

Panel frontend + backend:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/thuhtetnaingdev/mei-mei/main/install/panel.sh)
```

Node backend:

```bash
MEIMEI_CONTROL_PLANE_TOKEN="<panel NODE_SHARED_TOKEN>" bash <(curl -fsSL https://raw.githubusercontent.com/thuhtetnaingdev/mei-mei/main/install/node.sh)
```

Notes:

- The panel installer now installs both `panel_backend` and a prebuilt `panel_frontend` bundle, then serves them together from one backend service.
- The panel installer creates `/opt/meimei-panel/.env` if it does not exist and prints the generated admin password plus `NODE_SHARED_TOKEN`.
- The panel installer also installs a `mei` CLI. Run `mei uninstall` to remove the panel service, files, and SQLite database. Use `mei uninstall --yes` to skip the confirmation prompt.
- By default the panel is available on `:8080`. You can override it with `MEIMEI_PANEL_PORT`.
- The node installer installs `sing-box`, writes `/opt/meimei-node/.env`, creates TLS files, opens the standard ports when `ufw` is present, and prints the generated `NODE_TOKEN`.
- Release assets are published automatically when you push a tag like `v1.0.0`.

## Notes

- The current scaffold is production-oriented and extensible.
- Usage accounting hooks and bandwidth policy fields are included, but full traffic metering depends on your preferred telemetry pipeline.
- `node_backend` writes a generated `sing-box` JSON file and can execute a reload command without SSH.
- Node registration requires a shared registration token plus a per-node API token.
