#!/usr/bin/env bash
set -euo pipefail

REPO="${MEIMEI_REPO:-thuhtetnaingdev/mei-mei}"
VERSION="${MEIMEI_VERSION:-latest}"
INSTALL_DIR="${MEIMEI_NODE_DIR:-/opt/meimei-node}"
NODE_NAME="${MEIMEI_NODE_NAME:-$(hostname)}"
NODE_PORT="${MEIMEI_NODE_PORT:-9090}"
VLESS_PORT="${MEIMEI_VLESS_PORT:-443}"
TUIC_PORT="${MEIMEI_TUIC_PORT:-8443}"
HYSTERIA2_PORT="${MEIMEI_HYSTERIA2_PORT:-9443}"
PUBLIC_HOST="${MEIMEI_PUBLIC_HOST:-}"
NODE_TOKEN="${MEIMEI_NODE_TOKEN:-}"
CONTROL_PLANE_TOKEN="${MEIMEI_CONTROL_PLANE_TOKEN:-}"

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
need_cmd openssl

if [[ -z "$CONTROL_PLANE_TOKEN" ]]; then
  echo "MEIMEI_CONTROL_PLANE_TOKEN is required" >&2
  exit 1
fi

if [[ -z "$PUBLIC_HOST" ]]; then
  PUBLIC_HOST="$(curl -fsSL --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
fi

if [[ -z "$NODE_TOKEN" ]]; then
  NODE_TOKEN="$(openssl rand -hex 16)"
fi

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_arch="amd64" ;;
  aarch64|arm64) asset_arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

asset_name="node_backend-linux-${asset_arch}.tar.gz"

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

download_url="$(asset_url)"
if [[ -z "$download_url" ]]; then
  echo "failed to find release asset ${asset_name} for ${REPO}@${VERSION}" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

curl -fsSL "$download_url" -o "$tmp_dir/node_backend.tar.gz"

if ! command -v sing-box >/dev/null 2>&1; then
  curl -fsSL https://sing-box.app/install.sh | sudo sh
fi

pair_output="$(sing-box generate reality-keypair 2>/dev/null || true)"
reality_private_key="$(printf '%s\n' "$pair_output" | sed -n 's/^PrivateKey:[[:space:]]*//p')"
reality_public_key="$(printf '%s\n' "$pair_output" | sed -n 's/^PublicKey:[[:space:]]*//p')"
if [[ -z "$reality_private_key" || -z "$reality_public_key" ]]; then
  echo "failed to generate sing-box reality keypair" >&2
  exit 1
fi
reality_short_id="$(openssl rand -hex 4)"

sudo mkdir -p "$INSTALL_DIR"
tar -xzf "$tmp_dir/node_backend.tar.gz" -C "$tmp_dir"
sudo install -m 0755 "$tmp_dir/node_backend-linux-${asset_arch}/node_backend" "$INSTALL_DIR/node_backend"
sudo install -m 0644 "$tmp_dir/node_backend-linux-${asset_arch}/.env.example" "$INSTALL_DIR/.env.example"

if [[ ! -f "${INSTALL_DIR}/tls.key" || ! -f "${INSTALL_DIR}/tls.crt" ]]; then
  sudo openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "${INSTALL_DIR}/tls.key" \
    -out "${INSTALL_DIR}/tls.crt" \
    -days 3650 \
    -subj "/CN=${PUBLIC_HOST}" >/dev/null 2>&1
  sudo chmod 600 "${INSTALL_DIR}/tls.key"
fi

if [[ ! -f "${INSTALL_DIR}/sing-box.generated.json" ]]; then
  sudo tee "${INSTALL_DIR}/sing-box.generated.json" >/dev/null <<EOF
{
  "log": { "level": "info" },
  "inbounds": [],
  "outbounds": [{ "tag": "direct", "type": "direct" }],
  "route": {
    "auto_detect_interface": true,
    "final": "direct"
  }
}
EOF
fi

sudo tee "${INSTALL_DIR}/.env" >/dev/null <<EOF
PORT=${NODE_PORT}
NODE_NAME=${NODE_NAME}
NODE_TOKEN=${NODE_TOKEN}
CONTROL_PLANE_SHARED_TOKEN=${CONTROL_PLANE_TOKEN}
SINGBOX_CONFIG_PATH=${INSTALL_DIR}/sing-box.generated.json
SINGBOX_RELOAD_COMMAND=systemctl restart meimei-sing-box.service
NODE_BINARY_PATH=${INSTALL_DIR}/node_backend
NODE_RESTART_COMMAND=systemctl restart meimei-node.service
PUBLIC_HOST=${PUBLIC_HOST}
VLESS_PORT=${VLESS_PORT}
TUIC_PORT=${TUIC_PORT}
HYSTERIA2_PORT=${HYSTERIA2_PORT}
VLESS_REALITY_PRIVATE_KEY=${reality_private_key}
VLESS_REALITY_PUBLIC_KEY=${reality_public_key}
VLESS_REALITY_SHORT_ID=${reality_short_id}
VLESS_REALITY_SERVER_NAME=www.cloudflare.com
VLESS_REALITY_HANDSHAKE_SERVER=www.cloudflare.com
VLESS_REALITY_HANDSHAKE_PORT=443
TLS_CERTIFICATE_PATH=${INSTALL_DIR}/tls.crt
TLS_KEY_PATH=${INSTALL_DIR}/tls.key
TLS_SERVER_NAME=${PUBLIC_HOST}
EOF

sudo tee /etc/systemd/system/meimei-node.service >/dev/null <<EOF
[Unit]
Description=Meimei Node Backend
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${INSTALL_DIR}/.env
ExecStart=${INSTALL_DIR}/node_backend
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

sudo tee /etc/systemd/system/meimei-sing-box.service >/dev/null <<EOF
[Unit]
Description=Meimei Sing-box
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/sing-box run -c ${INSTALL_DIR}/sing-box.generated.json
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

if command -v ufw >/dev/null 2>&1; then
  sudo ufw allow "${NODE_PORT}/tcp" >/dev/null 2>&1 || true
  sudo ufw allow "${VLESS_PORT}/tcp" >/dev/null 2>&1 || true
  sudo ufw allow "${TUIC_PORT}/udp" >/dev/null 2>&1 || true
  sudo ufw allow "${HYSTERIA2_PORT}/udp" >/dev/null 2>&1 || true
fi

sudo systemctl daemon-reload
sudo systemctl enable meimei-sing-box.service
sudo systemctl enable --now meimei-node.service
sudo systemctl restart meimei-sing-box.service

echo
echo "node installed successfully"
echo "node name: ${NODE_NAME}"
echo "public host: ${PUBLIC_HOST}"
echo "node token: ${NODE_TOKEN}"
echo "control plane token: ${CONTROL_PLANE_TOKEN}"
echo "status: sudo systemctl status meimei-node meimei-sing-box --no-pager"
