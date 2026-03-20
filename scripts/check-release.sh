#!/usr/bin/env bash

set -euo pipefail

REPO="${FOCUS_REPO:-krabhi1/focus}"
TAG=""
REQUIRED_ASSET_PATTERN="${REQUIRED_ASSET_PATTERN:-focus_.*_linux_amd64\\.tar\\.gz}"
REQUIRED_CHECKSUM_PATTERN="${REQUIRED_CHECKSUM_PATTERN:-checksums_.*\\.txt}"

usage() {
  cat <<'EOF'
Usage: scripts/check-release.sh --tag <tag> [options]

Options:
  --tag <tag>          Release tag to verify (example: v0.1.0) [required]
  --repo <owner/name>  GitHub repo (default: krabhi1/focus)
  -h, --help           Show help

Environment:
  GITHUB_TOKEN         Recommended (required for private repos and higher API limits)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      TAG="$2"
      shift 2
      ;;
    --repo)
      REPO="$2"
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

if [[ -z "$TAG" ]]; then
  echo "--tag is required" >&2
  usage
  exit 1
fi

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd curl
json_get() {
  local expr="$1"
  local input="$2"

  if command -v jq >/dev/null 2>&1; then
    jq -r "$expr" <<<"$input"
    return
  fi

  if ! command -v python3 >/dev/null 2>&1; then
    echo "missing required command: jq or python3" >&2
    exit 1
  fi

  python3 -c '
import json
import sys

expr = sys.argv[1]
data = json.load(sys.stdin)

if expr == ".assets[].name":
    for a in data.get("assets", []):
        print(a.get("name", ""))
elif expr == ".name // empty":
    print(data.get("name", "") or "")
elif expr == ".draft":
    print(str(bool(data.get("draft", False))).lower())
elif expr == ".prerelease":
    print(str(bool(data.get("prerelease", False))).lower())
else:
    print("")
' "$expr" <<<"$input"
}

auth_header=()
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  auth_header=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

echo "Checking workflow runs for tag $TAG in $REPO..."
runs_json="$(curl -fsSL "${auth_header[@]}" "https://api.github.com/repos/${REPO}/actions/runs?event=push&per_page=50")"
if command -v jq >/dev/null 2>&1; then
  run_status="$(jq -r --arg tag "$TAG" '
    .workflow_runs
    | map(select(.head_branch == $tag))
    | sort_by(.created_at)
    | reverse
    | .[0].status // empty
  ' <<<"$runs_json")"
  run_conclusion="$(jq -r --arg tag "$TAG" '
    .workflow_runs
    | map(select(.head_branch == $tag))
    | sort_by(.created_at)
    | reverse
    | .[0].conclusion // empty
  ' <<<"$runs_json")"
elif command -v python3 >/dev/null 2>&1; then
  run_status="$(python3 -c '
import json
import sys
tag = sys.argv[1]
data = json.load(sys.stdin)
runs = [r for r in data.get("workflow_runs", []) if r.get("head_branch") == tag]
runs.sort(key=lambda r: r.get("created_at", ""), reverse=True)
print(runs[0].get("status", "") if runs else "")
' "$TAG" <<<"$runs_json")"
  run_conclusion="$(python3 -c '
import json
import sys
tag = sys.argv[1]
data = json.load(sys.stdin)
runs = [r for r in data.get("workflow_runs", []) if r.get("head_branch") == tag]
runs.sort(key=lambda r: r.get("created_at", ""), reverse=True)
print(runs[0].get("conclusion", "") if runs else "")
' "$TAG" <<<"$runs_json")"
else
  echo "missing required command: jq or python3" >&2
  exit 1
fi

if [[ -z "$run_status" ]]; then
  echo "No workflow run found yet for tag $TAG"
else
  echo "Latest run status: $run_status"
  if [[ -n "$run_conclusion" ]]; then
    echo "Latest run conclusion: $run_conclusion"
  fi
fi

echo "Checking release metadata..."
release_json="$(curl -fsSL "${auth_header[@]}" "https://api.github.com/repos/${REPO}/releases/tags/${TAG}")"
release_name="$(json_get '.name // empty' "$release_json")"
release_draft="$(json_get '.draft' "$release_json")"
release_prerelease="$(json_get '.prerelease' "$release_json")"
echo "Release name: ${release_name:-<empty>}"
echo "Draft: $release_draft | Pre-release: $release_prerelease"

assets="$(json_get '.assets[].name' "$release_json")"
if [[ -z "$assets" ]]; then
  echo "No release assets found for $TAG" >&2
  exit 1
fi

echo "Assets:"
echo "$assets" | sed 's/^/  - /'

if ! echo "$assets" | grep -Eq "$REQUIRED_ASSET_PATTERN"; then
  echo "Missing required tarball asset (pattern: $REQUIRED_ASSET_PATTERN)" >&2
  exit 1
fi

if ! echo "$assets" | grep -Eq "$REQUIRED_CHECKSUM_PATTERN"; then
  echo "Missing required checksum asset (pattern: $REQUIRED_CHECKSUM_PATTERN)" >&2
  exit 1
fi

echo "Release verification passed for $TAG"
