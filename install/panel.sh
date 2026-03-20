#!/usr/bin/env bash
set -euo pipefail

REPO="${MEIMEI_REPO:-thuhtetnaingdev/mei-mei}"
VERSION="${MEIMEI_VERSION:-latest}"
INSTALL_DIR="${MEIMEI_PANEL_DIR:-/opt/meimei-panel}"
BACKEND_PORT="${MEIMEI_PANEL_PORT:-8080}"
SERVICE_NAME="${MEIMEI_PANEL_SERVICE:-meimei-panel}"
CLI_PATH="${MEIMEI_CLI_PATH:-/usr/local/bin/mei}"
CLI_CONFIG_DIR="${MEIMEI_CLI_CONFIG_DIR:-/etc/meimei}"
CLI_CONFIG_FILE="${CLI_CONFIG_DIR}/panel-cli.env"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd tar
need_cmd systemctl
need_cmd mktemp
need_cmd sudo

kill_port_processes() {
  local port="$1"
  local pids=""

  if command -v lsof >/dev/null 2>&1; then
    pids="$(sudo lsof -tiTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  elif command -v ss >/dev/null 2>&1; then
    pids="$(sudo ss -ltnp 2>/dev/null | awk -v target=":${port}" '$4 ~ target { if (match($0, /pid=[0-9]+/)) { print substr($0, RSTART + 4, RLENGTH - 4) } }' | sort -u)"
  fi

  if [[ -z "$pids" ]]; then
    return
  fi

  echo "stopping existing listeners on port ${port}: ${pids}"
  while IFS= read -r pid; do
    if [[ -n "$pid" ]]; then
      sudo kill -9 "$pid" 2>/dev/null || true
    fi
  done <<< "$pids"
}

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch="amd64" ;;
  aarch64|arm64) asset_arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

asset_name="panel_backend-linux-${asset_arch}.tar.gz"
frontend_asset_name="panel_frontend.tar.gz"
release_json=""

github_release_api() {
  if [[ "$VERSION" == "latest" ]]; then
    echo "https://api.github.com/repos/${REPO}/releases/latest"
  else
    echo "https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
  fi
}

load_release_json() {
  if [[ -z "$release_json" ]]; then
    release_json="$(curl -fsSL "$(github_release_api)")"
  fi
}

asset_url() {
  load_release_json
  printf '%s' "$release_json" | grep '"browser_download_url"' | grep "${asset_name}" | head -n1 | sed -E 's/.*"([^"]+)".*/\1/'
}

release_tag() {
  if [[ "$VERSION" == "latest" ]]; then
    load_release_json
    printf '%s' "$release_json" | grep '"tag_name"' | head -n1 | sed -E 's/.*"([^"]+)".*/\1/'
  else
    printf '%s' "$VERSION"
  fi
}

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    cat /proc/sys/kernel/random/uuid 2>/dev/null | tr -d '-'
  fi
}

public_host_default() {
  curl -fsSL --max-time 5 https://api.ipify.org 2>/dev/null || echo "127.0.0.1"
}

named_asset_url() {
  local wanted_name="$1"
  load_release_json
  printf '%s' "$release_json" | grep '"browser_download_url"' | grep "${wanted_name}" | head -n1 | sed -E 's/.*"([^"]+)".*/\1/'
}

download_cli_script() {
  local target_path="$1"
  local ref=""
  local url=""

  for ref in "$(release_tag)" main; do
    url="https://raw.githubusercontent.com/${REPO}/${ref}/install/mei.sh"
    if curl -fsSL "$url" -o "$target_path"; then
      return 0
    fi
  done

  return 1
}

backend_download_url="$(asset_url)"
if [[ -z "$backend_download_url" ]]; then
  echo "failed to find release asset ${asset_name} for ${REPO}@${VERSION}" >&2
  exit 1
fi

frontend_download_url="$(named_asset_url "$frontend_asset_name")"
if [[ -z "$frontend_download_url" ]]; then
  echo "failed to find release asset ${frontend_asset_name} for ${REPO}@${VERSION}" >&2
  echo "push a new tagged release after the frontend bundle asset is published" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

curl -fsSL "$backend_download_url" -o "$tmp_dir/panel_backend.tar.gz"
curl -fsSL "$frontend_download_url" -o "$tmp_dir/panel_frontend.tar.gz"

sudo mkdir -p "$INSTALL_DIR"
tar -xzf "$tmp_dir/panel_backend.tar.gz" -C "$tmp_dir"
sudo install -m 0755 "$tmp_dir/panel_backend-linux-${asset_arch}/panel_backend" "$INSTALL_DIR/panel_backend"
sudo install -m 0644 "$tmp_dir/panel_backend-linux-${asset_arch}/.env.example" "$INSTALL_DIR/.env.example"

frontend_dir="${INSTALL_DIR}/frontend"
sudo rm -rf "$frontend_dir"
sudo mkdir -p "$frontend_dir"
tar -xzf "$tmp_dir/panel_frontend.tar.gz" -C "$tmp_dir"
sudo cp -R "$tmp_dir/panel_frontend/." "$frontend_dir/"

env_file="$INSTALL_DIR/.env"
if [[ ! -f "$env_file" ]]; then
  admin_username="${MEIMEI_ADMIN_USERNAME:-admin}"
  admin_password="${MEIMEI_ADMIN_PASSWORD:-$(random_hex | cut -c1-16)}"
  jwt_secret="${MEIMEI_JWT_SECRET:-$(random_hex)}"
  node_shared_token="${MEIMEI_NODE_SHARED_TOKEN:-$(random_hex)}"
  server_ip="$(public_host_default)"
  backend_public_url="${MEIMEI_PUBLIC_URL:-http://${server_ip}:${BACKEND_PORT}}"

  sudo tee "$env_file" >/dev/null <<EOF
APP_ENV=production
PORT=${BACKEND_PORT}
DATABASE_PATH=${INSTALL_DIR}/panel.sqlite3
JWT_SECRET=${jwt_secret}
ADMIN_USERNAME=${admin_username}
ADMIN_PASSWORD=${admin_password}
BASE_SUBSCRIPTION_URL=${backend_public_url}/subscription
BASE_PUBLIC_URL=${backend_public_url}
ALLOWED_ORIGINS=${backend_public_url},http://localhost:5173,http://127.0.0.1:5173
FRONTEND_DIST_DIR=${frontend_dir}
NODE_SHARED_TOKEN=${node_shared_token}
SYNC_TIMEOUT_SECONDS=10
EOF

  echo "created ${env_file}"
  echo "admin username: ${admin_username}"
  echo "admin password: ${admin_password}"
  echo "node shared token: ${node_shared_token}"
else
  echo "keeping existing ${env_file}"
fi

backend_public_url="$(grep '^BASE_PUBLIC_URL=' "$env_file" | head -n1 | cut -d= -f2-)"

sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" >/dev/null <<EOF
[Unit]
Description=Meimei Panel
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${env_file}
ExecStart=${INSTALL_DIR}/panel_backend
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

if ! download_cli_script "$tmp_dir/mei"; then
  echo "failed to download mei CLI installer helper" >&2
  exit 1
fi

sudo mkdir -p "$CLI_CONFIG_DIR"
sudo tee "$CLI_CONFIG_FILE" >/dev/null <<EOF
INSTALL_DIR=${INSTALL_DIR}
SERVICE_NAME=${SERVICE_NAME}
ENV_FILE=${env_file}
CLI_PATH=${CLI_PATH}
EOF
sudo install -m 0755 "$tmp_dir/mei" "$CLI_PATH"

kill_port_processes "$BACKEND_PORT"

sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"

echo
echo "panel installed successfully"
echo "service: ${SERVICE_NAME}"
echo "env: ${env_file}"
echo "status: sudo systemctl status ${SERVICE_NAME} --no-pager"
echo "panel url: ${backend_public_url}"
echo "cli: ${CLI_PATH} uninstall"
