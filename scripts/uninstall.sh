#!/usr/bin/env bash

set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"
LIBEXECDIR="$PREFIX/libexec/focus"
SYSTEMD_USER_DIR="${SYSTEMD_USER_DIR:-$HOME/.config/systemd/user}"

usage() {
  cat <<'EOF'
Usage: scripts/uninstall.sh [options]

Options:
  --prefix <path>      Remove binaries from <path> (default: ~/.local)
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

if command -v systemctl >/dev/null 2>&1; then
  SERVICE_TARGET="$SYSTEMD_USER_DIR/focusd.service"
  if [[ -f "$SERVICE_TARGET" ]]; then
    systemctl --user disable --now focusd.service || true
    rm -f "$SERVICE_TARGET"
    systemctl --user daemon-reload || true
    systemctl --user reset-failed || true
    echo "Removed systemd user unit $SERVICE_TARGET"
  else
    echo "No user service file found at $SERVICE_TARGET"
  fi
fi

rm -f "$BINDIR/focus" "$BINDIR/focusd" "$BINDIR/focus-events"
rm -f "$LIBEXECDIR/focusd" "$LIBEXECDIR/focus-events"
rm -rf "$PREFIX/share/focus/assets"
if [[ -d "$PREFIX/share/focus" ]] && [[ -z "$(ls -A "$PREFIX/share/focus" 2>/dev/null)" ]]; then
  rmdir "$PREFIX/share/focus" || true
fi
echo "Removed binaries from $BINDIR"
rm -rf "$LIBEXECDIR"
