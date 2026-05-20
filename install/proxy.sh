#!/usr/bin/env bash
set -euo pipefail

REPO="${MEIMEI_REPO:-thuhtetnaingdev/mei-mei}"
VERSION="${MEIMEI_VERSION:-latest}"
INSTALL_DIR="${MEIMEI_PROXY_DIR:-/opt/meimei-proxy}"
PROXY_PORT="${MEIMEI_PROXY_PORT:-9091}"
SERVICE_NAME="${MEIMEI_PROXY_SERVICE:-meimei-proxy}"

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

asset_name="proxy-linux-${asset_arch}.tar.gz"
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

if [[ -t 0 ]]; then
  echo "===== Meimei Proxy Installer ====="
  echo ""
  echo "The proxy maps subscription usernames to sing-box profiles"
  echo "by looking them up through the panel's migration-map API."
  echo ""

  read -r -p "Proxy port [${PROXY_PORT}]: " input_port
  PROXY_PORT="${input_port:-$PROXY_PORT}"

  while [[ -z "$PROXY_URL" ]]; do
    read -r -p "Panel URL (e.g. https://panel.meimeivpn.cc): " PROXY_URL
  done

  while [[ -z "$MAP_TOKEN" ]]; do
    read -r -s -p "MAP_TOKEN: " MAP_TOKEN
    echo ""
  done
fi

download_url="$(asset_url)"
if [[ -z "$download_url" ]]; then
  echo "failed to find release asset ${asset_name} for ${REPO}@${VERSION}" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

curl -fsSL "$download_url" -o "$tmp_dir/proxy.tar.gz"

sudo mkdir -p "$INSTALL_DIR"
tar -xzf "$tmp_dir/proxy.tar.gz" -C "$tmp_dir"
sudo install -m 0755 "$tmp_dir/proxy-linux-${asset_arch}/proxy" "$INSTALL_DIR/proxy"
sudo install -m 0644 "$tmp_dir/proxy-linux-${asset_arch}/.env.example" "$INSTALL_DIR/.env.example"

sudo tee "$INSTALL_DIR/.env" >/dev/null <<EOF
PORT=${PROXY_PORT}
PROXY_URL=${PROXY_URL}
MAP_TOKEN=${MAP_TOKEN}
EOF

sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" >/dev/null <<EOF
[Unit]
Description=Meimei Proxy
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${INSTALL_DIR}/.env
ExecStart=${INSTALL_DIR}/proxy
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

kill_port_processes "$PROXY_PORT"

sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"

echo ""
echo "proxy installed successfully"
echo "service: ${SERVICE_NAME}"
echo "env: ${INSTALL_DIR}/.env"
echo "status: sudo systemctl status ${SERVICE_NAME} --no-pager"
echo "endpoint: http://<this-server>:${PROXY_PORT}/sub/<username>?format=json"
