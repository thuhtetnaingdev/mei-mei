#!/usr/bin/env bash
set -euo pipefail

REPO="${MEIMEI_REPO:-thuhtetnaingdev/mei-mei}"
VERSION="${MEIMEI_VERSION:-latest}"
INSTALL_DIR="${MEIMEI_PANEL_DIR:-/opt/meimei-panel}"
PORT="${MEIMEI_PANEL_PORT:-8080}"
SERVICE_NAME="${MEIMEI_PANEL_SERVICE:-meimei-panel}"

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

github_release_api() {
  if [[ "$VERSION" == "latest" ]]; then
    echo "https://api.github.com/repos/${REPO}/releases/latest"
  else
    echo "https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
  fi
}

asset_url() {
  curl -fsSL "$(github_release_api)" | grep '"browser_download_url"' | grep "${asset_name}" | head -n1 | sed -E 's/.*"([^"]+)".*/\1/'
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

download_url="$(asset_url)"
if [[ -z "$download_url" ]]; then
  echo "failed to find release asset ${asset_name} for ${REPO}@${VERSION}" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

curl -fsSL "$download_url" -o "$tmp_dir/panel_backend.tar.gz"

sudo mkdir -p "$INSTALL_DIR"
tar -xzf "$tmp_dir/panel_backend.tar.gz" -C "$tmp_dir"
sudo install -m 0755 "$tmp_dir/panel_backend-linux-${asset_arch}/panel_backend" "$INSTALL_DIR/panel_backend"
sudo install -m 0644 "$tmp_dir/panel_backend-linux-${asset_arch}/.env.example" "$INSTALL_DIR/.env.example"

env_file="$INSTALL_DIR/.env"
if [[ ! -f "$env_file" ]]; then
  admin_username="${MEIMEI_ADMIN_USERNAME:-admin}"
  admin_password="${MEIMEI_ADMIN_PASSWORD:-$(random_hex | cut -c1-16)}"
  jwt_secret="${MEIMEI_JWT_SECRET:-$(random_hex)}"
  node_shared_token="${MEIMEI_NODE_SHARED_TOKEN:-$(random_hex)}"
  public_host="${MEIMEI_PUBLIC_URL:-http://$(public_host_default):${PORT}}"

  sudo tee "$env_file" >/dev/null <<EOF
APP_ENV=production
PORT=${PORT}
DATABASE_PATH=${INSTALL_DIR}/panel.sqlite3
JWT_SECRET=${jwt_secret}
ADMIN_USERNAME=${admin_username}
ADMIN_PASSWORD=${admin_password}
BASE_SUBSCRIPTION_URL=${public_host}/subscription
BASE_PUBLIC_URL=${public_host}
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

sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" >/dev/null <<EOF
[Unit]
Description=Meimei Panel Backend
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

sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"

echo
echo "panel installed successfully"
echo "service: ${SERVICE_NAME}"
echo "env: ${env_file}"
echo "status: sudo systemctl status ${SERVICE_NAME} --no-pager"
