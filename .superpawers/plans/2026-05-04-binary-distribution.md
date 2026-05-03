# Binary Distribution Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpawers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Set up automated binary distribution for the xray-sub-rotation-service Go project using GitHub Actions and GitHub Releases.

**Architecture:** A single GitHub Actions workflow (`build.yml`) triggers on `git tag` pushes (annotated, semantic versioned). It cross-compiles the binary for Linux/macOS/Windows and amd64/arm64 architectures, attaches them to a GitHub Release, and publishes release notes. This keeps CI fast, explicit, and deterministic.

**Tech Stack:** Go 1.24+, GitHub Actions, `cross` (for cross-compilation), `actions/upload-artifact@v4`, `softprops/action-gh-release@v2`

---

## File Structure

| File | Responsibility |
|------|----------------|
| `.github/workflows/build.yml` | CI workflow — build, test, release |
| `Makefile` | Update: add `version` variable and `release` target |

---

## Recommendations

**Chosen approach: GitHub Releases on annotated tags.**

Rationale:
- **GitHub Releases** is the simplest path for a single-binary service. Users download a zip/tarball from releases — no server infrastructure needed.
- **Annotated tags** (`git tag -a v1.0.0`) as the trigger are the standard Go pattern. No merge-to-main publishing (avoids accidental releases).
- **Cross-compilation** for the 4 most common combos: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`. This covers 99% of deployment targets.
- Skip apt/yum/Homebrew — overkill for a private/internal tool with few users.

**What we're NOT doing:**
- No GitHub Packages (adds complexity, users can just download from Releases)
- No Homebrew tap (not enough users yet)
- No self-hosted binary server (GitHub Releases is free and sufficient)
- No `latest` tag (can cause confusion with rollbacks)

---

### Task 1: GitHub Actions Workflow

**Files:**
- Create: `.github/workflows/build.yml`

- [ ] **Step 1: Create the workflow file**

```yaml
# .github/workflows/build.yml
name: Build & Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: linux
            goarch: amd64
            asset_ext: tar.gz
          - goos: linux
            goarch: arm64
            asset_ext: tar.gz
          - goos: darwin
            goarch: amd64
            asset_ext: tar.gz
          - goos: darwin
            goarch: arm64
            asset_ext: tar.gz
          - goos: windows
            goarch: amd64
            asset_ext: zip
      fail-fast: false

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: Run tests
        run: go test ./... -v

      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          name=xray-sub-rotation
          if [ "${{ matrix.goos }}" = "windows" ]; then
            name=xray-sub-rotation.exe
          fi
          go build -o ${name} -ldflags "-s -w" ./cmd/xray-sub-rotation/

      - name: Package
        id: package
        run: |
          asset_name="xray-sub-rotation_${{ matrix.goos }}_${{ matrix.goarch }}.${{ matrix.asset_ext }}"
          if [ "${{ matrix.asset_ext }}" = "tar.gz" ]; then
            tar czf "${asset_name}" xray-sub-rotation
          else
            zip "${asset_name}" xray-sub-rotation.exe
          fi
          echo "asset_name=${asset_name}" >> $GITHUB_OUTPUT

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: ${{ steps.package.outputs.asset_name }}
          path: ${{ steps.package.outputs.asset_name }}

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - name: Download artifacts
        uses: actions/download-artifact@v4

      - name: Create Release
        uses: softprops/action-gh-release@v2
        with:
          files: xray-sub-rotation_*/xray-sub-rotation_*
          generate_release_notes: true
```

- [ ] **Step 2: Create the directory**

Run: `mkdir -p .github/workflows`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/build.yml
git commit -m "ci: add GitHub Actions workflow for cross-platform releases"
```

---

### Task 2: Update README.md

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add download and release sections to README**

Replace the existing README.md content with:

```markdown
# xray-sub-rotation-service

A Go service that routes 3x-ui subscription requests across multiple 3x-ui instances, selecting the best-performing one based on health checks.

## Download

Grab the latest release from the [Releases page](https://github.com/makashov73/xray-sub-rotation-service/releases).

## How It Works

1. Load a list of 3x-ui subscription URLs from `sublist.md`
2. Periodically ping each endpoint to measure latency and availability
3. When a client requests `/subrouter/{subId}`, serve the fastest live endpoint

## Configuration

Edit `config.yaml`:
- `server.host`, `server.port`: Listen address
- `health_check.interval`: How often to ping endpoints
- `health_check.persist_path`: Optional path to persist health state across restarts
- `strategy`: Selection strategy (`fastest`, `random`, `first`)
- `tls.cert_file`, `tls.key_file`: HTTPS support
- `rate_limit`: Optional per-IP rate limiting

## Subscription List

Format in `sublist.md`:
```
subId | URL | Name
```

## Usage

```bash
./xray-sub-rotation
```

## API

- `GET /health` — Health check endpoint
- `GET /subrouter/{subId}` — Fetch the best subscription for a user

## Development

```bash
make test    # Run tests
make build   # Build binary
make lint    # Run linter (requires golangci-lint)
```

## Releases

Binaries are built automatically from tagged commits. To release:

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

This triggers the GitHub Actions workflow which builds binaries for Linux/macOS/Windows (amd64/arm64) and creates a GitHub Release with all artifacts.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add download section and release instructions to README"
```

---

## Summary

Two files changed, two commits total:

1. **`.github/workflows/build.yml`** — Single workflow that triggers on annotated tags, cross-compiles 5 binary variants, and publishes to GitHub Releases via `softprops/action-gh-release@v2`.
2. **`README.md`** — Updated with download links, new config options (TLS, rate limiting, persistence), and release instructions.

## Considerations

- **`-ldflags "-s -w"`** strips debug info, reducing binary size by ~30%.
- **`go.sum` is already committed** (yaml.v3), so no `go mod tidy` needed in CI.
- **Windows gets `.zip`**, Unix gets `.tar.gz` — standard convention.
- **No `latest` tag** is pushed, so users always download a specific version.
- **`permissions: contents: write`** is scoped to the workflow, not the repo — best practice.
- **`golangci-lint` is NOT run in CI** — it's only a local `make lint` target. Adding it would require installing it in the workflow.

## Concerns

1. **No semantic versioning validation.** The workflow fires on any `v*` tag. A `v0.0.0-rc1` or `v1.0.0-beta` would create releases. This is fine for now but could add tag validation later.
2. **No release notes template.** `softprops/action-gh-release@v2` generates auto-notes from commits since the last tag. Consider adding a `RELEASE.md` or release notes template if the commit history gets noisy.
3. **No SHA-suffix in filenames.** The artifacts are named `xray-sub-rotation_linux_amd64.tar.gz` without a commit SHA suffix. Adding one (e.g., `_abc123def.tar.gz`) would make builds reproducible. Worth adding in a follow-up if needed.
