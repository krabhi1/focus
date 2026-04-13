#!/usr/bin/env bash

set -euo pipefail

REPO="${FOCUS_REPO:-krabhi1/focus}"
VERSION="latest"
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"
LIBEXECDIR="$PREFIX/libexec/focus"
NO_SYSTEMD=0

usage() {
  cat <<'EOF'
Usage: install.sh [options]

Options:
  --version <tag>      Install a specific version (example: v0.1.0). Default: latest release
  --prefix <path>      Install binaries under <path> (default: ~/.local)
  --no-systemd         Skip systemd user service install/enable
  -h, --help           Show help

Current release target:
  linux/amd64
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
require_cmd install

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64) arch="amd64" ;;
  *)
    echo "unsupported architecture: $arch_raw (current releases are linux/amd64 only)" >&2
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
  awk -v a="$asset" '$2 == a || $2 == ("./" a)' "$checksums" > "${asset}.sha256"
  if [[ ! -s "${asset}.sha256" ]]; then
    echo "checksum entry for $asset not found in $checksums" >&2
    exit 1
  fi
  sha256sum -c "${asset}.sha256"
)

tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"
extracted_dir="$tmp_dir/focus_${VERSION}_${os}_${arch}"
if [[ ! -d "$extracted_dir" ]]; then
  echo "expected extracted directory not found: $extracted_dir" >&2
  exit 1
fi

mkdir -p "$BINDIR"
install -m 0755 "$extracted_dir/focus" "$BINDIR/focus"
rm -f "$BINDIR/focusd" "$BINDIR/focus-events"
mkdir -p "$LIBEXECDIR"
install -m 0755 "$extracted_dir/libexec/focus/focusd" "$LIBEXECDIR/focusd"
install -m 0755 "$extracted_dir/libexec/focus/focus-events" "$LIBEXECDIR/focus-events"
mkdir -p "$PREFIX/share/focus/assets"
if [[ -d "$extracted_dir/assets" ]]; then
  cp -r "$extracted_dir/assets/." "$PREFIX/share/focus/assets/"
fi
echo "Installed binaries to $BINDIR"
echo "Installed private runtime files to $LIBEXECDIR"
echo "Installed assets to $PREFIX/share/focus/assets"

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
StartLimitIntervalSec=60
StartLimitBurst=10

[Service]
Type=simple
ExecStart=$LIBEXECDIR/focusd
WorkingDirectory=%h
Restart=on-failure
RestartSec=2
NoNewPrivileges=true

[Install]
WantedBy=graphical-session.target
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
