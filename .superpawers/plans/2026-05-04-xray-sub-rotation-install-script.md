# Install Script & Release Workflow Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpawers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a shell install script (`install.sh`) that auto-detects OS/arch, verifies dependencies, and downloads/releases a binary from GitHub, plus a proper release workflow to generate those binaries.

**Architecture:** The existing `.github/workflows/build.yml` already handles multi-platform builds and release creation, but has issues (duplicate release job, no version injection, no checksums). I'll rewrite it cleanly, then create `install.sh` that downloads from the latest release (with version pinning support) and runs the binary.

**Tech Stack:** Bash (POSIX-compatible), GitHub Actions (softprops/action-gh-release), shellcheck for linting.

---

## Questions & Tradeoffs

1. **Where to install?** I'll default to `~/.local/bin/xray-sub-rotation` (standard user-local bin), with `--dry-run` to just print what it would do. Users can override with `-d <path>`.

2. **Run after install vs. just install?** I'll install *and* run, because the user asked for a script that "runs the binary." There's a `--no-run` flag to skip execution.

3. **Checksum verification?** Yes — I'll compute SHA-256 of the downloaded tarball and verify it. This is important for supply-chain safety.

4. **Version pinning?** Default to `latest`, but accept `--version v1.2.3` to pin.

5. **`curl` vs `wget`?** Prefer `curl`, fall back to `wget`. Check for either.

---

## Files to Create / Modify

| File | Action |
|------|--------|
| `install.sh` | **Create** — the install/run script |
| `.github/workflows/build.yml` | **Rewrite** — clean up existing broken workflow |
| `.gitignore` | **Modify** — add `dist/` |
| `README.md` | **Modify** — add install script docs |

---

### Task 1: Rewrite `.github/workflows/build.yml`

**Problem with current file:**
- Line 62-68: Second `Create Release (manual)` job duplicates the first one — both use `softprops/action-gh-release` and will conflict
- Line 42: `-X main.version=${VERSION}` references `main.version` but `main.go` has no `version` variable
- No checksums in release assets
- No artifact retention policy

**Action:** Rewrite the file.

```yaml
# .github/workflows/build.yml
name: Build & Release

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:
    inputs:
      version:
        description: 'Version tag (e.g., v1.0.0)'
        required: true
        type: string

permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          check-latest: true

      - name: Run tests
        run: go test -race -count=1 ./...

      - name: Build binaries
        run: |
          VERSION=${{ github.event.inputs.version || github.ref_name }}
          mkdir -p dist
          for OS_ARCH in "linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64" "windows/amd64"; do
            OS="${OS_ARCH%%/*}"
            ARCH="${OS_ARCH##*/}"
            NAME="xray-sub-rotation-${VERSION}-${OS}-${ARCH}"
            [ "$OS" = "windows" ] && NAME="${NAME}.exe"
            GOOS="$OS" GOARCH="$ARCH" CGO_ENABLED=0 go build \
              -ldflags="-s -w -X main.version=${VERSION}" \
              -o "dist/${NAME}" \
              ./cmd/xray-sub-rotation/
            if [ "$OS" = "windows" ]; then
              (cd dist && zip "${NAME}.zip" "${NAME}")
            else
              (cd dist && tar czf ../"${NAME}".tar.gz "${NAME}")
            fi
          done
          ls -lh dist/

      - name: Generate checksums
        run: |
          VERSION=${{ github.event.inputs.version || github.ref_name }}
          cd dist
          sha256sum * > "checksums-${VERSION}.txt"
          cat "checksums-${VERSION}.txt"

      - name: Create Release
        uses: softprops/action-gh-release@v2
        if: startsWith(github.ref, 'refs/tags/')
        with:
          files: dist/*
          generate_release_notes: true
          token: ${{ secrets.GITHUB_TOKEN }}

      - name: Create Release (manual)
        uses: softprops/action-gh-release@v2
        if: github.event.inputs.version
        with:
          tag_name: ${{ github.event.inputs.version }}
          files: dist/*
          generate_release_notes: true
          token: ${{ secrets.GITHUB_TOKEN }}
```

**Note on `main.version`:** The `-X main.version=${VERSION}` ldflag requires a `var Version string` variable in `main.go`. If that variable doesn't exist, remove the `-X` part from the ldflags. I'll include both options in the plan.

---

### Task 2: Create `install.sh`

**Action:** Create the script at the project root.

```bash
#!/usr/bin/env bash
# install.sh — Install and run xray-sub-rotation from GitHub Releases
# Usage: ./install.sh [--version v1.0.0] [--dry-run] [--no-run] [-d <install-dir>]

set -euo pipefail

# --- Defaults ---
VERSION="latest"
DRY_RUN=false
NO_RUN=false
INSTALL_DIR="$HOME/.local/bin"
SCRIPT_NAME="install.sh"

# --- Parse arguments ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --version=*)
      VERSION="${1#*=}"
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --no-run)
      NO_RUN=true
      shift
      ;;
    -d|--dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $SCRIPT_NAME [--version v1.0.0] [--dry-run] [--no-run] [-d <install-dir>]"
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

# --- Helpers ---
die() { echo "Error: $*" >&2; exit 1; }

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) die "Unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) die "Unsupported arch: $(uname -m)" ;;
  esac
}

check_command() {
  command -v "$1" &>/dev/null
}

# --- Detect platform ---
OS=$(detect_os)
ARCH=$(detect_arch)
echo "Detected platform: $OS/$ARCH"

# --- Check dependencies ---
FETCHER=""
if check_command curl; then
  FETCHER="curl"
elif check_command wget; then
  FETCHER="wget"
else
  die "Neither curl nor wget found. Please install one."
fi

# --- Resolve version ---
if [ "$VERSION" = "latest" ]; then
  if [ "$FETCHER" = "curl" ]; then
    VERSION=$(curl -sL https://api.github.com/repos/makashov73/xray-sub-rotation-service/releases/latest | grep '"tag_name":' | sed 's/.*"v\([^"]*\)".*/\1/')
  else
    VERSION=$(wget -qO- https://api.github.com/repos/makashov73/xray-sub-rotation-service/releases/latest | grep '"tag_name":' | sed 's/.*"v\([^"]*\)".*/\1/')
  fi
  [ -z "$VERSION" ] && die "Failed to resolve latest version"
  VERSION="v${VERSION}"
fi

echo "Target version: $VERSION"

# --- Build asset name ---
if [ "$OS" = "windows" ]; then
  ASSET="xray-sub-rotation-${VERSION}-${OS}-${ARCH}.exe"
  ASSET_URL="https://github.com/makashov73/xray-sub-rotation-service/releases/download/${VERSION}/${ASSET}"
else
  ASSET="xray-sub-rotation-${VERSION}-${OS}-${ARCH}.tar.gz"
  ASSET_URL="https://github.com/makashov73/xray-sub-rotation-service/releases/download/${VERSION}/${ASSET}"
fi

# --- Dry run mode ---
if [ "$DRY_RUN" = true ]; then
  echo "Would download: $ASSET_URL"
  echo "Would install to: $INSTALL_DIR"
  [ "$NO_RUN" = false ] && echo "Would run: $INSTALL_DIR/xray-sub-rotation"
  exit 0
fi

# --- Download ---
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading $ASSET..."
if [ "$FETCHER" = "curl" ]; then
  curl -sL -o "$TMPDIR/$ASSET" "$ASSET_URL"
else
  wget -q -O "$TMPDIR/$ASSET" "$ASSET_URL"
fi

[ ! -f "$TMPDIR/$ASSET" ] && die "Download failed: $ASSET_URL"

# --- Verify checksum ---
CHECKSUM_URL="https://github.com/makashov73/xray-sub-rotation-service/releases/download/${VERSION}/checksums-${VERSION}.txt"
if check_command sha256sum; then
  echo "Verifying checksum..."
  (cd "$TMPDIR" && sha256sum --check checksums-${VERSION}.txt) || die "Checksum verification failed"
fi

# --- Install ---
mkdir -p "$INSTALL_DIR"

if [ "$OS" = "windows" ]; then
  cp "$TMPDIR/$ASSET" "$INSTALL_DIR/xray-sub-rotation.exe"
  chmod +x "$INSTALL_DIR/xray-sub-rotation.exe"
else
  tar xzf "$TMPDIR/$ASSET" -C "$TMPDIR"
  cp "$TMPDIR/xray-sub-rotation" "$INSTALL_DIR/xray-sub-rotation"
  chmod +x "$INSTALL_DIR/xray-sub-rotation"
fi

echo "Installed to: $INSTALL_DIR/xray-sub-rotation"

# --- Run ---
if [ "$NO_RUN" = false ]; then
  echo "Running..."
  exec "$INSTALL_DIR/xray-sub-rotation"
fi
```

**Key design notes:**
- Uses `trap` to clean up temp dir on exit
- Supports both `--version v1.0.0` and `--version=v1.0.0`
- Checksum verification is best-effort (skipped if sha256sum not found)
- `exec` replaces the shell process so the binary receives signals directly
- Creates `INSTALL_DIR` if it doesn't exist

---

### Task 3: Update `.gitignore`

**Action:** Add `dist/` to `.gitignore`.

Current `.gitignore` ends at line 18. Add before the `# IDE` comment:

```
# Release artifacts
dist/
```

---

### Task 4: Update `README.md`

**Action:** Replace the manual download section (lines 37-48) with install script instructions.

Add a new "Install" section before "Building from source":

```markdown
## Install

Download and run with a single script:

```bash
# Latest version
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/main/install.sh | bash

# Pin a version
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/main/install.sh | bash -s -- --version v1.0.0

# Install only (don't run)
curl -fsSL https://github.com/makashov73/xray-sub-rotation-service/releases/download/v1.0.0/install.sh | bash --no-run

# Install to custom directory
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/main/install.sh | bash -s -- -d /opt/bin
```

Or download directly:

```bash
curl -LO https://github.com/makashov73/xray-sub-rotation-service/releases/latest/download/install.sh
chmod +x install.sh
./install.sh --version v1.0.0
```

The script installs to `~/.local/bin/` by default. Run `./install.sh --dry-run` to preview without downloading.
```

---

## Task Order

1. **Rewrite `.github/workflows/build.yml`** (foundation — other tasks depend on it)
2. **Update `.gitignore`** (trivial, can be done with build.yml)
3. **Create `install.sh`**
4. **Update `README.md`**

## Verification Steps

- `git tag v0.1.0 && git push origin v0.1.0` — triggers the workflow
- Verify the workflow produces 5 tarballs + 1 zip + 1 checksums file in `dist/`
- Verify the GitHub Release is created with all assets
- Run `./install.sh --dry-run` — should print platform, version URL, install path
- Run `./install.sh --version v0.1.0 --no-run` — should download and install without executing
- Run `./install.sh --version v0.1.0` — should download, install, and run (Ctrl+C to stop)

## Open Questions for User

1. **Should I add a `main.version` variable to `main.go`?** The workflow's `-X main.version=...` ldflag won't do anything unless the binary has a `var Version string` declared. I can add it to `main.go` (e.g., print version on `--version` flag), or remove the ldflag from the workflow. Which do you prefer?

2. **Should `install.sh` also download and install `config.yaml` + `sublist.md`?** Right now it only handles the binary. Users would need to copy those files themselves. I can add a `--with-config` flag to also fetch the repo's config template and sublist example.

3. **Should the script accept `--config /path/to/config.yaml` and `--sublist /path/to/sublist.md` to pass directly to the binary?** This would avoid the need for users to place files in the working directory.
