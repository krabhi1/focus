#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"
LIBEXECDIR="$PREFIX/libexec/focus"
SYSTEMD_USER_DIR="${SYSTEMD_USER_DIR:-$HOME/.config/systemd/user}"

NO_BUILD=0
NO_SYSTEMD=0

usage() {
  cat <<'EOF'
Usage: scripts/install-local.sh [options]

Options:
  --prefix <path>      Install binaries under <path> (default: ~/.local)
  --no-build           Skip 'make build'
  --no-systemd         Do not install/enable user systemd service
  -h, --help           Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)
      PREFIX="$2"
      BINDIR="$PREFIX/bin"
      shift 2
      ;;
    --no-build)
      NO_BUILD=1
      shift
      ;;
    --no-systemd)
      NO_SYSTEMD=1
      shift
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

if [[ "$NO_BUILD" -eq 0 ]]; then
  echo "Building binaries..."
  make -C "$ROOT_DIR" build
fi

mkdir -p "$BINDIR"
install -m 0755 "$ROOT_DIR/dist/focus" "$BINDIR/focus"
rm -f "$BINDIR/focusd" "$BINDIR/focus-events"
mkdir -p "$LIBEXECDIR"
install -m 0755 "$ROOT_DIR/dist/focusd" "$LIBEXECDIR/focusd"
install -m 0755 "$ROOT_DIR/dist/focus-events" "$LIBEXECDIR/focus-events"
mkdir -p "$PREFIX/share/focus/assets"
cp -r "$ROOT_DIR/assets/." "$PREFIX/share/focus/assets/"

echo "Installed binaries to $BINDIR"
echo "Installed private runtime files to $LIBEXECDIR"

if [[ "$NO_SYSTEMD" -eq 1 ]]; then
  echo "Skipping systemd user service setup (--no-systemd)"
  exit 0
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl not found; skipping systemd user service setup"
  exit 0
fi

mkdir -p "$SYSTEMD_USER_DIR"
SERVICE_TEMPLATE="$ROOT_DIR/packaging/systemd/focusd.service"
SERVICE_TARGET="$SYSTEMD_USER_DIR/focusd.service"
sed "s|@LIBEXECDIR@|$LIBEXECDIR|g" "$SERVICE_TEMPLATE" > "$SERVICE_TARGET"

echo "Installed systemd user unit to $SERVICE_TARGET"

if systemctl --user daemon-reload && systemctl --user enable --now focusd.service; then
  echo "focusd.service enabled and started (user service)"
else
  cat <<EOF
Failed to enable/start systemd user service automatically.
Run manually:
  systemctl --user daemon-reload
  systemctl --user enable --now focusd.service
EOF
fi
