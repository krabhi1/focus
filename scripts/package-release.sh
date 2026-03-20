#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/release-artifacts}"
VERSION=""
TARGETS="${TARGETS:-linux/amd64}"

usage() {
  cat <<'EOF'
Usage: scripts/package-release.sh --version <tag> [options]

Options:
  --version <tag>      Release tag (example: v0.1.0) [required]
  --targets <list>     Comma-separated GOOS/GOARCH targets (default: linux/amd64)
  --out <dir>          Output directory (default: ./release-artifacts)
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --targets)
      TARGETS="$2"
      shift 2
      ;;
    --out)
      OUT_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  echo "--version is required" >&2
  usage
  exit 1
fi

if [[ ! "$VERSION" =~ ^v[0-9]+(\.[0-9]+){1,2}([-.][0-9A-Za-z.-]+)?$ ]]; then
  echo "version must look like v0.1.0 (got: $VERSION)" >&2
  exit 1
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

HOST_GOOS="$(go env GOOS)"
HOST_GOARCH="$(go env GOARCH)"

IFS=',' read -r -a target_array <<< "$TARGETS"
for target in "${target_array[@]}"; do
  goos="${target%%/*}"
  goarch="${target##*/}"

  if [[ -z "$goos" || -z "$goarch" || "$goos" == "$goarch" ]]; then
    echo "invalid target '$target' (expected GOOS/GOARCH)" >&2
    exit 1
  fi

  stage_dir="$(mktemp -d)"
  pkg_dir="$stage_dir/focus_${VERSION}_${goos}_${goarch}"
  mkdir -p "$pkg_dir"

  echo "Building focus and focusd for $goos/$goarch..."
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$pkg_dir/focus" "$ROOT_DIR/cmd/client"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" -o "$pkg_dir/focusd" "$ROOT_DIR/cmd/daemon"

  if [[ "$goos" != "$HOST_GOOS" || "$goarch" != "$HOST_GOARCH" ]]; then
    echo "focus-events native helper currently requires native gcc/libs; cross-build unsupported for $goos/$goarch" >&2
    echo "supported host target for this runner: $HOST_GOOS/$HOST_GOARCH" >&2
    rm -rf "$stage_dir"
    exit 1
  fi

  echo "Building focus-events for $goos/$goarch..."
  native_flags="$(pkg-config --cflags --libs libsystemd x11 xscrnsaver)"
  gcc -Wall -Wextra -O2 "$ROOT_DIR/native/session_event_listener.c" -o "$pkg_dir/focus-events" $native_flags

  cp -r "$ROOT_DIR/assets" "$pkg_dir/assets"
  cp "$ROOT_DIR/README.md" "$pkg_dir/README.md"

  tarball="focus_${VERSION}_${goos}_${goarch}.tar.gz"
  tar -C "$stage_dir" -czf "$OUT_DIR/$tarball" "$(basename "$pkg_dir")"
  rm -rf "$stage_dir"
done

(
  cd "$OUT_DIR"
  sha256sum ./*.tar.gz > "checksums_${VERSION}.txt"
)

echo "Release artifacts created in $OUT_DIR"
