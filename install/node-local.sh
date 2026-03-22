#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SSH_HOST=""
SSH_USER="${MEIMEI_SSH_USER:-root}"
SSH_PORT="${MEIMEI_SSH_PORT:-22}"
SSH_KEY="${MEIMEI_SSH_KEY:-}"
TARBALL_PATH="${MEIMEI_NODE_TARBALL:-}"

INSTALL_DIR="${MEIMEI_NODE_DIR:-/opt/meimei-node}"
NODE_NAME="${MEIMEI_NODE_NAME:-}"
NODE_PORT="${MEIMEI_NODE_PORT:-9090}"
PUBLIC_HOST="${MEIMEI_PUBLIC_HOST:-}"
NODE_TOKEN="${MEIMEI_NODE_TOKEN:-}"
CONTROL_PLANE_TOKEN="${MEIMEI_CONTROL_PLANE_TOKEN:-}"
SINGBOX_V2RAY_API_LISTEN="${MEIMEI_SINGBOX_V2RAY_API_LISTEN:-127.0.0.1:10085}"
PANEL_ENV_PATH="${REPO_ROOT}/panel_backend/.env"

usage() {
  cat <<'EOF'
Usage:
  install/node-local.sh [options]

Options:
  --host <host>           Remote SSH host or IP. Prompts if omitted.
  --user <user>           SSH user. Default: root
  --port <port>           SSH port. Default: 22
  --key <path>            SSH private key path
  --tarball <path>        Local node backend tarball to upload
  --install-dir <path>    Remote install dir. Default: /opt/meimei-node
  --node-name <name>      Node name. Default: remote hostname
  --node-port <port>      Node backend port. Default: 9090
  --public-host <host>    Public host for generated config. Default: --host value
  --node-token <token>    Node API token. Default: generated on remote
  --control-plane-token <token>
                          Shared token from panel. Optional for local dev install.
  --v2ray-api-listen <addr>
                          sing-box stats listen addr. Default: 127.0.0.1:10085
  --help                  Show this help

Environment variable equivalents:
  MEIMEI_SSH_USER, MEIMEI_SSH_PORT, MEIMEI_SSH_KEY, MEIMEI_NODE_TARBALL,
  MEIMEI_NODE_DIR, MEIMEI_NODE_NAME, MEIMEI_NODE_PORT, MEIMEI_PUBLIC_HOST,
  MEIMEI_NODE_TOKEN, MEIMEI_CONTROL_PLANE_TOKEN, MEIMEI_SINGBOX_V2RAY_API_LISTEN

Examples:
  install/node-local.sh

  install/node-local.sh \
    --host dev-node.example.com \
    --user ubuntu \
    --tarball dist/node_backend-linux-amd64.tar.gz \
    --control-plane-token shared-token \
    --public-host dev-node.example.com
EOF
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

load_control_plane_token_from_panel_env() {
  if [[ -n "${CONTROL_PLANE_TOKEN}" ]]; then
    return
  fi
  if [[ ! -f "${PANEL_ENV_PATH}" ]]; then
    return
  fi

  local line
  line="$(grep '^NODE_SHARED_TOKEN=' "${PANEL_ENV_PATH}" 2>/dev/null | tail -n1 || true)"
  if [[ -z "${line}" ]]; then
    return
  fi

  CONTROL_PLANE_TOKEN="${line#NODE_SHARED_TOKEN=}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host)
      SSH_HOST="${2:-}"
      shift 2
      ;;
    --user)
      SSH_USER="${2:-}"
      shift 2
      ;;
    --port)
      SSH_PORT="${2:-}"
      shift 2
      ;;
    --key)
      SSH_KEY="${2:-}"
      shift 2
      ;;
    --tarball)
      TARBALL_PATH="${2:-}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    --node-name)
      NODE_NAME="${2:-}"
      shift 2
      ;;
    --node-port)
      NODE_PORT="${2:-}"
      shift 2
      ;;
    --public-host)
      PUBLIC_HOST="${2:-}"
      shift 2
      ;;
    --node-token)
      NODE_TOKEN="${2:-}"
      shift 2
      ;;
    --control-plane-token)
      CONTROL_PLANE_TOKEN="${2:-}"
      shift 2
      ;;
    --v2ray-api-listen)
      SINGBOX_V2RAY_API_LISTEN="${2:-}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "${SSH_HOST}" ]]; then
  read -r -p "Node IP or host: " SSH_HOST
fi

if [[ -z "${SSH_HOST}" ]]; then
  echo "node IP or host is required" >&2
  exit 1
fi

if [[ -z "${PUBLIC_HOST}" ]]; then
  PUBLIC_HOST="${SSH_HOST}"
fi

load_control_plane_token_from_panel_env
if [[ -z "${CONTROL_PLANE_TOKEN}" ]]; then
  echo "control plane shared token is required" >&2
  echo "Set MEIMEI_CONTROL_PLANE_TOKEN or add NODE_SHARED_TOKEN to ${PANEL_ENV_PATH}" >&2
  exit 1
fi

need_cmd ssh
need_cmd scp
need_cmd tar

if [[ -z "${SSH_USER}" ]]; then
  SSH_USER="root"
fi

read -r -p "SSH username [${SSH_USER}]: " input_user
if [[ -n "${input_user}" ]]; then
  SSH_USER="${input_user}"
fi

ssh_opts=()
scp_opts=()
if [[ -n "${SSH_PORT}" ]]; then
  ssh_opts+=(-p "${SSH_PORT}")
  scp_opts+=(-P "${SSH_PORT}")
fi
if [[ -n "${SSH_KEY}" ]]; then
  ssh_opts+=(-i "${SSH_KEY}")
  scp_opts+=(-i "${SSH_KEY}")
fi
ssh_opts+=(-o StrictHostKeyChecking=accept-new)
scp_opts+=(-o StrictHostKeyChecking=accept-new)

control_socket="${HOME}/.ssh/meimei-node-local-%r@%h:%p"
ssh_opts+=(-o ControlMaster=auto -o ControlPersist=5m -o "ControlPath=${control_socket}")
scp_opts+=(-o ControlMaster=auto -o ControlPersist=5m -o "ControlPath=${control_socket}")

remote_target="${SSH_USER}@${SSH_HOST}"

ssh_cmd=(ssh "${ssh_opts[@]}")
scp_cmd=(scp "${scp_opts[@]}")

echo "connecting to ${remote_target}"
if [[ -z "${SSH_KEY}" ]]; then
  echo "ssh/scp will prompt for the remote password if needed."
fi

remote_arch="$("${ssh_cmd[@]}" "${remote_target}" 'uname -m')"
case "${remote_arch}" in
  x86_64|amd64) asset_arch="amd64" ;;
  aarch64|arm64) asset_arch="arm64" ;;
  *)
    echo "unsupported remote architecture: ${remote_arch}" >&2
    exit 1
    ;;
esac

if [[ -z "${TARBALL_PATH}" ]]; then
  TARBALL_PATH="${REPO_ROOT}/dist/node_backend-linux-${asset_arch}.tar.gz"
fi

if [[ ! -f "${TARBALL_PATH}" ]]; then
  echo "node backend tarball not found: ${TARBALL_PATH}" >&2
  exit 1
fi

TARBALL_PATH="$(cd "$(dirname "${TARBALL_PATH}")" && pwd)/$(basename "${TARBALL_PATH}")"
if ! tar -tzf "${TARBALL_PATH}" >/dev/null 2>&1; then
  echo "invalid tarball: ${TARBALL_PATH}" >&2
  exit 1
fi

remote_tmp_dir="$("${ssh_cmd[@]}" "${remote_target}" 'mktemp -d')"
remote_tarball="${remote_tmp_dir}/$(basename "${TARBALL_PATH}")"

cleanup_remote() {
  "${ssh_cmd[@]}" "${remote_target}" "rm -rf '${remote_tmp_dir}'" >/dev/null 2>&1 || true
  "${ssh_cmd[@]}" -O exit "${remote_target}" >/dev/null 2>&1 || true
}
trap cleanup_remote EXIT

echo "uploading ${TARBALL_PATH} to ${remote_target}:${remote_tarball}"
"${scp_cmd[@]}" "${TARBALL_PATH}" "${remote_target}:${remote_tarball}"

echo "installing node backend on ${remote_target}"
remote_env=(
  "LOCAL_TARBALL=$(printf '%q' "${remote_tarball}")"
  "INSTALL_DIR=$(printf '%q' "${INSTALL_DIR}")"
  "NODE_NAME=$(printf '%q' "${NODE_NAME}")"
  "NODE_PORT=$(printf '%q' "${NODE_PORT}")"
  "PUBLIC_HOST=$(printf '%q' "${PUBLIC_HOST}")"
  "NODE_TOKEN=$(printf '%q' "${NODE_TOKEN}")"
  "CONTROL_PLANE_TOKEN=$(printf '%q' "${CONTROL_PLANE_TOKEN}")"
  "SINGBOX_V2RAY_API_LISTEN=$(printf '%q' "${SINGBOX_V2RAY_API_LISTEN}")"
)
"${ssh_cmd[@]}" "${remote_target}" "${remote_env[*]} bash -s" <<'REMOTE_SCRIPT'
set -euo pipefail

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd tar
need_cmd systemctl
need_cmd mktemp
need_cmd openssl
need_cmd curl

existing_env_path="${INSTALL_DIR}/.env"

read_existing_env() {
  local key="$1"
  if [[ ! -f "${existing_env_path}" ]]; then
    return
  fi

  local line
  line="$(sudo grep "^${key}=" "${existing_env_path}" 2>/dev/null | tail -n1 || true)"
  if [[ -z "${line}" ]]; then
    return
  fi

  printf '%s' "${line#*=}"
}

if [[ ! -f "${LOCAL_TARBALL}" ]]; then
  echo "uploaded tarball not found: ${LOCAL_TARBALL}" >&2
  exit 1
fi

if [[ -z "${NODE_NAME}" ]]; then
  NODE_NAME="$(read_existing_env NODE_NAME)"
fi
if [[ -z "${NODE_NAME}" ]]; then
  NODE_NAME="$(hostname)"
fi

if [[ -z "${PUBLIC_HOST}" ]]; then
  PUBLIC_HOST="$(read_existing_env PUBLIC_HOST)"
fi
if [[ -z "${PUBLIC_HOST}" ]]; then
  PUBLIC_HOST="$(curl -fsSL --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
fi

if [[ -z "${NODE_TOKEN}" ]]; then
  NODE_TOKEN="$(read_existing_env NODE_TOKEN)"
fi
if [[ -z "${NODE_TOKEN}" ]]; then
  NODE_TOKEN="$(openssl rand -hex 16)"
fi

existing_control_plane_token="$(read_existing_env CONTROL_PLANE_SHARED_TOKEN)"
if [[ -z "${CONTROL_PLANE_TOKEN}" && -n "${existing_control_plane_token}" ]]; then
  CONTROL_PLANE_TOKEN="${existing_control_plane_token}"
fi

kill_port_processes() {
  local port="$1"
  local pids=""

  if command -v lsof >/dev/null 2>&1; then
    pids="$(sudo lsof -tiTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  elif command -v ss >/dev/null 2>&1; then
    pids="$(sudo ss -ltnp 2>/dev/null | awk -v target=":${port}" '$4 ~ target { if (match($0, /pid=[0-9]+/)) { print substr($0, RSTART + 4, RLENGTH - 4) } }' | sort -u)"
  fi

  if [[ -z "${pids}" ]]; then
    return
  fi

  echo "stopping existing listeners on port ${port}: ${pids}"
  while IFS= read -r pid; do
    if [[ -n "${pid}" ]]; then
      sudo kill -9 "${pid}" 2>/dev/null || true
    fi
  done <<< "${pids}"
}

install_official_singbox() {
  echo "installing official sing-box package"
  curl -fsSL https://sing-box.app/install.sh | sudo sh
}

singbox_supports_v2ray_api() {
  if ! command -v sing-box >/dev/null 2>&1; then
    return 1
  fi

  local check_file
  check_file="$(mktemp)"
  cat >"${check_file}" <<'EOF'
{
  "log": { "level": "warn" },
  "inbounds": [],
  "outbounds": [{ "type": "direct", "tag": "direct" }],
  "route": { "final": "direct" },
  "experimental": {
    "v2ray_api": {
      "listen": "127.0.0.1:10085",
      "stats": {
        "enabled": true,
        "users": ["healthcheck@example.com"]
      }
    }
  }
}
EOF

  if sing-box check -c "${check_file}" >/dev/null 2>&1; then
    rm -f "${check_file}"
    return 0
  fi

  rm -f "${check_file}"
  return 1
}

ensure_compatible_singbox() {
  if ! command -v sing-box >/dev/null 2>&1; then
    install_official_singbox
  elif ! singbox_supports_v2ray_api; then
    echo "existing sing-box build lacks v2ray_api support, reinstalling official package"
    install_official_singbox
  fi

  if ! singbox_supports_v2ray_api; then
    echo "installed sing-box still lacks v2ray_api support" >&2
    exit 1
  fi
}

ensure_compatible_singbox
reality_private_key="$(read_existing_env VLESS_REALITY_PRIVATE_KEY)"
reality_public_key="$(read_existing_env VLESS_REALITY_PUBLIC_KEY)"
if [[ -z "${reality_private_key}" || -z "${reality_public_key}" ]]; then
  pair_output="$(sing-box generate reality-keypair 2>/dev/null || true)"
  reality_private_key="$(printf '%s\n' "${pair_output}" | sed -n 's/^PrivateKey:[[:space:]]*//p')"
  reality_public_key="$(printf '%s\n' "${pair_output}" | sed -n 's/^PublicKey:[[:space:]]*//p')"
  if [[ -z "${reality_private_key}" || -z "${reality_public_key}" ]]; then
    echo "failed to generate sing-box reality keypair" >&2
    exit 1
  fi
fi
reality_short_id="$(read_existing_env VLESS_REALITY_SHORT_ID)"
if [[ -z "${reality_short_id}" ]]; then
  reality_short_id="$(openssl rand -hex 4)"
fi
reality_server_name="$(read_existing_env VLESS_REALITY_SERVER_NAME)"
if [[ -z "${reality_server_name}" ]]; then
  reality_server_name="www.cloudflare.com"
fi
reality_handshake_server="$(read_existing_env VLESS_REALITY_HANDSHAKE_SERVER)"
if [[ -z "${reality_handshake_server}" ]]; then
  reality_handshake_server="www.cloudflare.com"
fi
reality_handshake_port="$(read_existing_env VLESS_REALITY_HANDSHAKE_PORT)"
if [[ -z "${reality_handshake_port}" ]]; then
  reality_handshake_port="443"
fi
tls_server_name="$(read_existing_env TLS_SERVER_NAME)"
if [[ -z "${tls_server_name}" ]]; then
  tls_server_name="${PUBLIC_HOST}"
fi

extract_dir="$(mktemp -d)"
trap 'rm -rf "${extract_dir}"' EXIT

tar -xzf "${LOCAL_TARBALL}" -C "${extract_dir}" 2> >(grep -v "LIBARCHIVE.xattr.com.apple.provenance" >&2 || true)

extracted_binary="$(find "${extract_dir}" -maxdepth 3 -type f -name node_backend | head -n1)"
if [[ -z "${extracted_binary}" || ! -f "${extracted_binary}" ]]; then
  echo "failed to locate node_backend binary in extracted tarball" >&2
  exit 1
fi
tar_root="$(dirname "${extracted_binary}")"

sudo mkdir -p "${INSTALL_DIR}"
sudo install -m 0755 "${tar_root}/node_backend" "${INSTALL_DIR}/node_backend"
if [[ -f "${tar_root}/.env.example" ]]; then
  sudo install -m 0644 "${tar_root}/.env.example" "${INSTALL_DIR}/.env.example"
fi

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
SINGBOX_V2RAY_API_LISTEN=${SINGBOX_V2RAY_API_LISTEN}
SINGBOX_RELOAD_COMMAND=systemctl restart meimei-sing-box.service
NODE_BINARY_PATH=${INSTALL_DIR}/node_backend
NODE_RESTART_COMMAND=systemctl restart meimei-node.service
PUBLIC_HOST=${PUBLIC_HOST}
VLESS_REALITY_PRIVATE_KEY=${reality_private_key}
VLESS_REALITY_PUBLIC_KEY=${reality_public_key}
VLESS_REALITY_SHORT_ID=${reality_short_id}
VLESS_REALITY_SERVER_NAME=${reality_server_name}
VLESS_REALITY_HANDSHAKE_SERVER=${reality_handshake_server}
VLESS_REALITY_HANDSHAKE_PORT=${reality_handshake_port}
TLS_CERTIFICATE_PATH=${INSTALL_DIR}/tls.crt
TLS_KEY_PATH=${INSTALL_DIR}/tls.key
TLS_SERVER_NAME=${tls_server_name}
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

kill_port_processes "${NODE_PORT}"

if command -v ufw >/dev/null 2>&1; then
  sudo ufw allow "${NODE_PORT}/tcp" >/dev/null 2>&1 || true
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
REMOTE_SCRIPT

echo
echo "local deploy completed successfully"
echo "remote host: ${SSH_HOST}"
echo "uploaded tarball: ${TARBALL_PATH}"
