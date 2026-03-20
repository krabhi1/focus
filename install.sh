#!/usr/bin/env bash

set -euo pipefail

REPO="${FOCUS_REPO:-krabhi1/focus}"
VERSION="latest"
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"
NO_SYSTEMD=0

usage() {
  cat <<'EOF'
Usage: install.sh [options]

Options:
  --version <tag>      Install a specific version (example: v0.1.0). Default: latest release
  --prefix <path>      Install binaries under <path>/bin (default: ~/.local)
  --no-systemd         Skip systemd user service install/enable
  -h, --help           Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --prefix)
      PREFIX="$2"
      BINDIR="$PREFIX/bin"
      shift 2
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

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd curl
require_cmd tar
require_cmd sha256sum

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch_raw" >&2
    exit 1
    ;;
esac

if [[ "$os" != "linux" ]]; then
  echo "unsupported OS: $os (linux only currently)" >&2
  exit 1
fi

if [[ "$VERSION" == "latest" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  if [[ -z "$VERSION" ]]; then
    echo "failed to resolve latest release version for $REPO" >&2
    exit 1
  fi
fi

asset="focus_${VERSION}_${os}_${arch}.tar.gz"
checksums="checksums_${VERSION}.txt"
base_url="https://github.com/$REPO/releases/download/$VERSION"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "Downloading $asset..."
curl -fL "$base_url/$asset" -o "$tmp_dir/$asset"
curl -fL "$base_url/$checksums" -o "$tmp_dir/$checksums"

(
  cd "$tmp_dir"
  grep " ${asset}$" "$checksums" > "${asset}.sha256"
  sha256sum -c "${asset}.sha256"
)

tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"
extracted_dir="$tmp_dir/focus_${VERSION}_${os}_${arch}"

mkdir -p "$BINDIR"
install -m 0755 "$extracted_dir/focus" "$BINDIR/focus"
install -m 0755 "$extracted_dir/focusd" "$BINDIR/focusd"
install -m 0755 "$extracted_dir/focus-events" "$BINDIR/focus-events"
echo "Installed binaries to $BINDIR"

if [[ "$NO_SYSTEMD" -eq 1 ]]; then
  echo "Skipping systemd setup (--no-systemd)"
  exit 0
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl not found; skipping systemd setup"
  exit 0
fi

systemd_user_dir="${SYSTEMD_USER_DIR:-$HOME/.config/systemd/user}"
mkdir -p "$systemd_user_dir"
service_file="$systemd_user_dir/focusd.service"

cat > "$service_file" <<EOF
[Unit]
Description=Focus daemon
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
ExecStart=$BINDIR/focusd
WorkingDirectory=%h
Environment=PATH=$BINDIR:/usr/local/bin:/usr/bin:/bin
Restart=on-failure
RestartSec=2
NoNewPrivileges=true

[Install]
WantedBy=default.target
EOF

if systemctl --user daemon-reload && systemctl --user enable --now focusd.service; then
  echo "focusd.service enabled and started"
else
  cat <<EOF
Installed service file at $service_file
Enable it manually:
  systemctl --user daemon-reload
  systemctl --user enable --now focusd.service
EOF
fi
