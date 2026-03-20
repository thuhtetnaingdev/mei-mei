#!/usr/bin/env bash
set -euo pipefail

REPO="${MEIMEI_REPO:-thuhtetnaingdev/mei-mei}"
VERSION="${MEIMEI_VERSION:-latest}"
INSTALL_DIR="${MEIMEI_PANEL_DIR:-/opt/meimei-panel}"
BACKEND_PORT="${MEIMEI_PANEL_PORT:-8080}"
FRONTEND_PORT="${MEIMEI_FRONTEND_PORT:-5173}"
SERVICE_NAME="${MEIMEI_PANEL_SERVICE:-meimei-panel}"
FRONTEND_SERVICE_NAME="${MEIMEI_PANEL_FRONTEND_SERVICE:-meimei-panel-frontend}"

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

repo_archive_url() {
  local tag
  tag="$(release_tag)"
  echo "https://github.com/${REPO}/archive/refs/tags/${tag}.tar.gz"
}

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    cat /proc/sys/kernel/random/uuid 2>/dev/null | tr -d '-'
  fi
}

origin_with_port() {
  local source_url="$1"
  local port="$2"
  local scheme host

  scheme="$(printf '%s' "$source_url" | sed -E 's#^(https?)://.*#\1#')"
  host="$(printf '%s' "$source_url" | sed -E 's#^https?://([^/:]+).*$#\1#')"

  if [[ -z "$scheme" || -z "$host" || "$scheme" == "$source_url" || "$host" == "$source_url" ]]; then
    return 1
  fi

  printf '%s://%s:%s\n' "$scheme" "$host" "$port"
}

public_host_default() {
  curl -fsSL --max-time 5 https://api.ipify.org 2>/dev/null || echo "127.0.0.1"
}

install_nodejs_if_missing() {
  if command -v npm >/dev/null 2>&1 && command -v node >/dev/null 2>&1; then
    return
  fi

  if ! command -v apt-get >/dev/null 2>&1; then
    echo "missing node/npm and no apt-get available to install them automatically" >&2
    exit 1
  fi

  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
  sudo apt-get install -y nodejs
}

download_url="$(asset_url)"
if [[ -z "$download_url" ]]; then
  echo "failed to find release asset ${asset_name} for ${REPO}@${VERSION}" >&2
  exit 1
fi

repo_download_url="$(repo_archive_url)"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

curl -fsSL "$download_url" -o "$tmp_dir/panel_backend.tar.gz"
curl -fsSL "$repo_download_url" -o "$tmp_dir/repo.tar.gz"

sudo mkdir -p "$INSTALL_DIR"
tar -xzf "$tmp_dir/panel_backend.tar.gz" -C "$tmp_dir"
sudo install -m 0755 "$tmp_dir/panel_backend-linux-${asset_arch}/panel_backend" "$INSTALL_DIR/panel_backend"
sudo install -m 0644 "$tmp_dir/panel_backend-linux-${asset_arch}/.env.example" "$INSTALL_DIR/.env.example"

tar -xzf "$tmp_dir/repo.tar.gz" -C "$tmp_dir"
repo_dir="$(find "$tmp_dir" -maxdepth 1 -type d -name 'mei-mei-*' | head -n1)"
if [[ -z "$repo_dir" ]]; then
  echo "failed to unpack repo archive" >&2
  exit 1
fi

install_nodejs_if_missing

frontend_dir="${INSTALL_DIR}/frontend"
sudo rm -rf "$frontend_dir"
sudo mkdir -p "$frontend_dir"
sudo cp -R "$repo_dir/panel_frontend/." "$frontend_dir/"
sudo chown -R "$(id -u)":"$(id -g)" "$frontend_dir"

env_file="$INSTALL_DIR/.env"
if [[ ! -f "$env_file" ]]; then
  admin_username="${MEIMEI_ADMIN_USERNAME:-admin}"
  admin_password="${MEIMEI_ADMIN_PASSWORD:-$(random_hex | cut -c1-16)}"
  jwt_secret="${MEIMEI_JWT_SECRET:-$(random_hex)}"
  node_shared_token="${MEIMEI_NODE_SHARED_TOKEN:-$(random_hex)}"
  server_ip="$(public_host_default)"
  backend_public_url="${MEIMEI_PUBLIC_URL:-http://${server_ip}:${BACKEND_PORT}}"
  frontend_public_url="${MEIMEI_FRONTEND_PUBLIC_URL:-http://${server_ip}:${FRONTEND_PORT}}"

  sudo tee "$env_file" >/dev/null <<EOF
APP_ENV=production
PORT=${BACKEND_PORT}
DATABASE_PATH=${INSTALL_DIR}/panel.sqlite3
JWT_SECRET=${jwt_secret}
ADMIN_USERNAME=${admin_username}
ADMIN_PASSWORD=${admin_password}
BASE_SUBSCRIPTION_URL=${backend_public_url}/subscription
BASE_PUBLIC_URL=${backend_public_url}
ALLOWED_ORIGINS=${frontend_public_url},http://localhost:5173,http://127.0.0.1:5173
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
frontend_public_url="${MEIMEI_FRONTEND_PUBLIC_URL:-}"
if [[ -z "$frontend_public_url" ]]; then
  frontend_public_url="$(origin_with_port "$backend_public_url" "$FRONTEND_PORT" || true)"
fi
if [[ -z "$frontend_public_url" ]]; then
  frontend_public_url="http://$(public_host_default):${FRONTEND_PORT}"
fi

cat > "$frontend_dir/.env.production" <<EOF
VITE_API_URL=${backend_public_url}
EOF

cd "$frontend_dir"
npm install
npm run build

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

sudo tee "/etc/systemd/system/${FRONTEND_SERVICE_NAME}.service" >/dev/null <<EOF
[Unit]
Description=Meimei Panel Frontend
After=network.target ${SERVICE_NAME}.service

[Service]
Type=simple
WorkingDirectory=${frontend_dir}
ExecStart=/usr/bin/npm run preview -- --host 0.0.0.0 --port ${FRONTEND_PORT}
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"
sudo systemctl enable --now "${FRONTEND_SERVICE_NAME}.service"

echo
echo "panel installed successfully"
echo "backend service: ${SERVICE_NAME}"
echo "frontend service: ${FRONTEND_SERVICE_NAME}"
echo "env: ${env_file}"
echo "status: sudo systemctl status ${SERVICE_NAME} --no-pager"
echo "frontend status: sudo systemctl status ${FRONTEND_SERVICE_NAME} --no-pager"
echo "frontend url: ${frontend_public_url}"
echo "backend url: ${backend_public_url}"
