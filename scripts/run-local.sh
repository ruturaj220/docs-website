#!/usr/bin/env bash
# Runs docs-platform on the host for local testing.
#
#   WEBHOOK_SECRET=dev-secret ./scripts/run-local.sh
#
# Needs Go and mkdocs on PATH:  pip install -r requirements.txt
set -euo pipefail

PROJ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export DATA_DIR="${DATA_DIR:-$PROJ/.local-data}"
export LISTEN_ADDR="${LISTEN_ADDR:-:8080}"
export WEBHOOK_SECRET="${WEBHOOK_SECRET:-dev-secret}"
export MKDOCS_BASE_CONFIG="${MKDOCS_BASE_CONFIG:-$PROJ/mkdocs-base.yml}"
export COMMON_INDEX="${COMMON_INDEX:-$PROJ/docs/index.md}"
export PROTECTED_REFS="${PROTECTED_REFS:-main,master,develop}"
export MAX_VERSIONS="${MAX_VERSIONS:-5}"
# Feature branches publish as their own versions; set false to turn that off.
export PUBLISH_FEATURE_BRANCHES="${PUBLISH_FEATURE_BRANCHES:-true}"

if ! command -v mkdocs >/dev/null 2>&1; then
  echo "mkdocs not found on PATH. Run: pip install -r requirements.txt" >&2
  exit 1
fi

mkdir -p "$DATA_DIR"
echo "data dir       : $DATA_DIR"
echo "protected refs : $PROTECTED_REFS"
echo "feature pub    : $PUBLISH_FEATURE_BRANCHES (keep newest $MAX_VERSIONS)"
echo "listening on   : http://localhost${LISTEN_ADDR}"
echo

exec go run ./cmd/server
