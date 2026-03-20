#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="${MEIMEI_CLI_CONFIG_FILE:-/etc/meimei/panel-cli.env}"
DEFAULT_INSTALL_DIR="${MEIMEI_PANEL_DIR:-/opt/meimei-panel}"
DEFAULT_SERVICE_NAME="${MEIMEI_PANEL_SERVICE:-meimei-panel}"
DEFAULT_CLI_PATH="${MEIMEI_CLI_PATH:-/usr/local/bin/mei}"

print_usage() {
  cat <<'EOF'
Usage:
  mei uninstall [--yes]

Commands:
  uninstall   Remove the installed Meimei panel service, files, and database
  help        Show this help message

Options:
  --yes       Skip the confirmation prompt
EOF
}

read_config_value() {
  local file="$1"
  local key="$2"

  if [[ ! -f "$file" ]]; then
    return 1
  fi

  grep "^${key}=" "$file" | head -n1 | cut -d= -f2-
}

resolve_setting() {
  local env_value="$1"
  local config_key="$2"
  local fallback="$3"
  local config_value=""

  if [[ -n "$env_value" ]]; then
    printf '%s\n' "$env_value"
    return
  fi

  config_value="$(read_config_value "$CONFIG_FILE" "$config_key" 2>/dev/null || true)"
  if [[ -n "$config_value" ]]; then
    printf '%s\n' "$config_value"
    return
  fi

  printf '%s\n' "$fallback"
}

run_as_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    "$@"
    return
  fi

  if ! command -v sudo >/dev/null 2>&1; then
    echo "sudo is required to manage the panel installation" >&2
    exit 1
  fi

  sudo "$@"
}

path_within_base() {
  local target="$1"
  local base="$2"

  case "$target" in
    "$base"|"$base"/*) return 0 ;;
    *) return 1 ;;
  esac
}

remove_path_if_present() {
  local target="$1"

  if [[ -z "$target" ]]; then
    return
  fi

  if [[ -e "$target" || -L "$target" ]]; then
    run_as_root rm -rf "$target"
  fi
}

confirm_uninstall() {
  local install_dir="$1"
  local service_unit="$2"
  local env_file="$3"
  local database_path="$4"

  if [[ ! -t 0 ]]; then
    echo "Refusing to uninstall from a non-interactive shell without --yes." >&2
    exit 1
  fi

  echo "This will permanently remove the Meimei panel installation:"
  echo "  service: ${service_unit}"
  echo "  install dir: ${install_dir}"
  if [[ -n "$env_file" ]]; then
    echo "  env file: ${env_file}"
  fi
  if [[ -n "$database_path" ]]; then
    echo "  database: ${database_path}"
  fi
  printf 'Type "uninstall" to continue: '

  local confirmation=""
  read -r confirmation
  if [[ "$confirmation" != "uninstall" ]]; then
    echo "Uninstall cancelled."
    exit 1
  fi
}

uninstall_panel() {
  local assume_yes="false"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --yes)
        assume_yes="true"
        ;;
      -h|--help)
        print_usage
        exit 0
        ;;
      *)
        echo "Unknown option for uninstall: $1" >&2
        print_usage >&2
        exit 1
        ;;
    esac
    shift
  done

  local install_dir service_name cli_path env_file service_unit database_path frontend_dir config_dir
  install_dir="$(resolve_setting "${MEIMEI_PANEL_DIR:-}" "INSTALL_DIR" "$DEFAULT_INSTALL_DIR")"
  service_name="$(resolve_setting "${MEIMEI_PANEL_SERVICE:-}" "SERVICE_NAME" "$DEFAULT_SERVICE_NAME")"
  cli_path="$(resolve_setting "${MEIMEI_CLI_PATH:-}" "CLI_PATH" "$DEFAULT_CLI_PATH")"
  env_file="$(resolve_setting "${MEIMEI_PANEL_ENV_FILE:-}" "ENV_FILE" "${install_dir}/.env")"
  service_unit="${service_name%.service}.service"
  database_path="$(read_config_value "$env_file" "DATABASE_PATH" 2>/dev/null || true)"
  frontend_dir="$(read_config_value "$env_file" "FRONTEND_DIST_DIR" 2>/dev/null || true)"
  config_dir="$(dirname "$CONFIG_FILE")"

  if [[ "$assume_yes" != "true" ]]; then
    confirm_uninstall "$install_dir" "$service_unit" "$env_file" "$database_path"
  fi

  if command -v systemctl >/dev/null 2>&1; then
    run_as_root systemctl disable --now "$service_unit" >/dev/null 2>&1 || true
  fi
  run_as_root rm -f "/etc/systemd/system/${service_unit}"
  if command -v systemctl >/dev/null 2>&1; then
    run_as_root systemctl daemon-reload >/dev/null 2>&1 || true
    run_as_root systemctl reset-failed >/dev/null 2>&1 || true
  fi

  if [[ -n "$database_path" ]] && ! path_within_base "$database_path" "$install_dir"; then
    remove_path_if_present "$database_path"
  fi
  if [[ -n "$frontend_dir" ]] && ! path_within_base "$frontend_dir" "$install_dir"; then
    remove_path_if_present "$frontend_dir"
  fi
  if [[ -n "$env_file" ]] && ! path_within_base "$env_file" "$install_dir"; then
    remove_path_if_present "$env_file"
  fi

  remove_path_if_present "$install_dir"
  remove_path_if_present "$CONFIG_FILE"
  run_as_root rmdir "$config_dir" >/dev/null 2>&1 || true
  remove_path_if_present "$cli_path"

  echo "Meimei panel uninstalled."
}

main() {
  local command="${1:-help}"

  case "$command" in
    uninstall)
      shift
      uninstall_panel "$@"
      ;;
    help|-h|--help)
      print_usage
      ;;
    *)
      echo "Unknown command: $command" >&2
      print_usage >&2
      exit 1
      ;;
  esac
}

main "$@"
