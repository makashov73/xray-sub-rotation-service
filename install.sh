#!/usr/bin/env bash
set -euo pipefail

# ─── Configuration ──────────────────────────────────────────────────────────────
REPO="makashov73/xray-sub-rotation-service"
BINARY_NAME="xray-sub-rotation"
VERSION="latest"
INSTALL_DIR="$HOME/.local/bin"
DRY_RUN=false
RUN_AFTER=false
WITH_CONFIG=false
CONFIG_PATH=""


# ─── Helpers ────────────────────────────────────────────────────────────────────

die() {
  echo "ERROR: $*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Install xray-sub-rotation from GitHub Releases.
Supports Linux and macOS (amd64/arm64). For Windows, download binaries manually from GitHub Releases.

Options:
  --version VERSION     Pin to a specific version (default: latest)
  -d, --dir DIR         Install directory (default: $HOME/.local/bin)
  --with-config         Download config.yaml template to install directory
  --run                 Run the binary immediately after installation
  --config PATH         Pass --config to binary when using --run
  --dry-run             Print what would be done without doing it
  -h, --help            Show this help message

Examples:
  $(basename "$0")                  # Install latest to ~/.local/bin
  $(basename "$0") --version v1.0   # Install a specific version
  $(basename "$0") --with-config    # Install + download config.yaml
  $(basename "$0") --run            # Install and run the binary
EOF
  exit 0
}

die_on_missing() {
  if [ -z "$1" ]; then
    die "Version '$2' not found. Check https://github.com/${REPO}/releases"
  fi
}

# ─── Argument parsing ───────────────────────────────────────────────────────────

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    -d|--dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --with-config)
      WITH_CONFIG=true
      shift
      ;;
    --run)
      RUN_AFTER=true
      shift
      ;;
    --config)
      CONFIG_PATH="$2"
      shift 2
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

# ─── OS / Arch detection ────────────────────────────────────────────────────────

detect_os() {
  case "$(uname -s)" in
    Linux*)   echo "linux" ;;
    Darwin*)  echo "darwin" ;;
    *)        die "Unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64)  echo "amd64" ;;
    amd64)   echo "amd64" ;;
    aarch64) echo "arm64" ;;
    arm64)   echo "arm64" ;;
    *)       die "Unsupported arch: $(uname -m)" ;;
  esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
ASSET_NAME="${BINARY_NAME}-${VERSION}-${OS}-${ARCH}"
ASSET_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}.tar.gz"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums-${VERSION}.txt"
CONFIG_URL="https://github.com/${REPO}/releases/download/${VERSION}/config.yaml"

# ─── Dependency checks ──────────────────────────────────────────────────────────

check_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "Required command not found: $1 (install it and retry)"
  fi
}

echo "Checking dependencies..."
check_command tar
if command -v curl >/dev/null 2>&1; then
  FETCH_CMD="curl"
elif command -v wget >/dev/null 2>&1; then
  FETCH_CMD="wget"
else
  die "Neither curl nor wget is available. Please install one and retry."
fi

# Prefer sha256sum (linux) over shasum (macOS)
if command -v sha256sum >/dev/null 2>&1; then
  CHECK_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  CHECK_CMD="shasum"
else
  die "Neither sha256sum nor shasum is available. Cannot verify checksums."
fi

echo "Platform: ${OS}/${ARCH}  Fetcher: ${FETCH_CMD}  Checksum: ${CHECK_CMD}"

# ─── Version resolution ─────────────────────────────────────────────────────────

resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    echo "Resolving latest version..."
    case "$FETCH_CMD" in
      curl)
        VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
          | grep '"tag_name":' \
          | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
        ;;
      wget)
        VERSION="$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" \
          | grep '"tag_name":' \
          | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
        ;;
    esac
    die_on_missing "$VERSION" "latest"
  fi
}

resolve_version

# ─── Build final paths ─────────────────────────────────────────────────────────

ASSET_NAME="${BINARY_NAME}-${VERSION}-${OS}-${ARCH}"
ASSET_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}.tar.gz"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums-${VERSION}.txt"
CONFIG_URL="https://github.com/${REPO}/releases/download/${VERSION}/config.yaml"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# ─── Download & install ─────────────────────────────────────────────────────────

if [ "$DRY_RUN" = true ]; then
  echo "[dry-run] Would download: ${ASSET_URL}"
  echo "[dry-run] Would verify checksums from: ${CHECKSUMS_URL}"
  echo "[dry-run] Would extract to: ${TMPDIR}"
  echo "[dry-run] Would install to: ${INSTALL_DIR}"
  if [ "$WITH_CONFIG" = true ]; then
    echo "[dry-run] Would download config.yaml to: ${INSTALL_DIR}"
  fi
  if [ "$RUN_AFTER" = true ]; then
    echo "[dry-run] Would run: ${INSTALL_DIR}/${BINARY_NAME} ${CONFIG_PATH:+--config $CONFIG_PATH}"
  fi
  exit 0
fi

# Download tarball
echo "Downloading ${ASSET_NAME}.tar.gz..."
case "$FETCH_CMD" in
  curl)
    curl -fSL -o "${TMPDIR}/${ASSET_NAME}.tar.gz" "$ASSET_URL"
    ;;
  wget)
    wget -O "${TMPDIR}/${ASSET_NAME}.tar.gz" "$ASSET_URL"
    ;;
esac

# Download and verify checksums
echo "Verifying checksums..."
case "$FETCH_CMD" in
  curl)
    curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL"
    ;;
  wget)
    wget -qO "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL"
    ;;
esac

# Verify checksum
CHECKSUM_LINE=$(grep "${ASSET_NAME}.tar.gz" "${TMPDIR}/checksums.txt") || \
  die "Checksum for ${ASSET_NAME}.tar.gz not found in checksums file"

cd "$TMPDIR"
if [ "$CHECK_CMD" = "shasum" ]; then
  echo "$CHECKSUM_LINE" | shasum -a 256 -c -
else
  echo "$CHECKSUM_LINE" | sha256sum -c -
fi
cd - >/dev/null

# Extract
echo "Extracting..."
tar -xzf "${TMPDIR}/${ASSET_NAME}.tar.gz" -C "$TMPDIR"

# Ensure install directory exists
mkdir -p "$INSTALL_DIR"

# Copy binary
cp "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
echo "Installed to ${INSTALL_DIR}/${BINARY_NAME}"

# Download config if requested
if [ "$WITH_CONFIG" = true ]; then
  echo "Downloading config.yaml..."
  case "$FETCH_CMD" in
    curl)
      curl -fsSL -o "${INSTALL_DIR}/config.yaml" "$CONFIG_URL"
      ;;
    wget)
      wget -qO "${INSTALL_DIR}/config.yaml" "$CONFIG_URL"
      ;;
  esac
  echo "Config written to ${INSTALL_DIR}/config.yaml"
fi

# Run if requested
if [ "$RUN_AFTER" = true ]; then
  echo "Starting ${BINARY_NAME}..."
  if [ -n "$CONFIG_PATH" ]; then
    exec "${INSTALL_DIR}/${BINARY_NAME}" --config "$CONFIG_PATH"
  else
    exec "${INSTALL_DIR}/${BINARY_NAME}"
  fi
fi

echo "Done."
