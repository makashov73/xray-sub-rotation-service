#!/usr/bin/env bash
set -euo pipefail

# ─── Configuration ──────────────────────────────────────────────────────────────
BINARY_NAME="xray-sub-rotation"
INSTALL_DIR="$HOME/.local/bin"
DRY_RUN=false
REMOVE_CONFIG=false

# ─── Helpers ────────────────────────────────────────────────────────────────────

die() {
  echo "ERROR: $*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Uninstall xray-sub-rotation and related files.

Options:
  -d, --dir DIR         Install directory (default: $HOME/.local/bin)
  --with-config         Also remove config.yaml from install directory
  --dry-run             Print what would be removed without doing it
  -h, --help            Show this help message

Examples:
  $(basename "$0")                  # Remove binary from ~/.local/bin
  $(basename "$0") --with-config    # Remove binary and config.yaml
  $(basename "$0") -d /usr/local/bin --with-config
EOF
  exit 0
}

# ─── Argument parsing ───────────────────────────────────────────────────────────

while [ $# -gt 0 ]; do
  case "$1" in
    -d|--dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --with-config)
      REMOVE_CONFIG=true
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    -h|--help)
      usage
      ;;
    *)
      die "Unknown option: $1"
      ;;
  esac
done

# ─── Uninstall ──────────────────────────────────────────────────────────────────

BINARY_PATH="${INSTALL_DIR}/${BINARY_NAME}"
CONFIG_PATH="${INSTALL_DIR}/config.yaml"
REMOVED=0

remove_file() {
  local path="$1"
  local label="$2"
  if [ -f "$path" ]; then
    if [ "$DRY_RUN" = true ]; then
      echo "[dry-run] Would remove ${label}: ${path}"
    else
      rm -f "$path"
      echo "Removed ${label}: ${path}"
    fi
    REMOVED=$((REMOVED + 1))
  fi
}

# Stop running process if any
if command -v pgrep >/dev/null 2>&1; then
  if pgrep -x "$BINARY_NAME" >/dev/null 2>&1; then
    if [ "$DRY_RUN" = true ]; then
      echo "[dry-run] Would stop running ${BINARY_NAME} process"
    else
      echo "Stopping running ${BINARY_NAME}..."
      pkill -x "$BINARY_NAME" 2>/dev/null || true
      sleep 1
    fi
  fi
fi

remove_file "$BINARY_PATH" "binary"

if [ "$REMOVE_CONFIG" = true ]; then
  remove_file "$CONFIG_PATH" "config"
fi

if [ "$REMOVED" -eq 0 ]; then
  echo "Nothing to remove — ${BINARY_NAME} not found in ${INSTALL_DIR}"
else
  if [ "$DRY_RUN" = false ]; then
    echo "Done. ${BINARY_NAME} has been uninstalled."
  fi
fi
