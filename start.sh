#!/usr/bin/env bash
set -euo pipefail

export LISTEN_ADDR="${LISTEN_ADDR:-0.0.0.0:3000}"

export TARGET_HOST="${TARGET_HOST:-212.95.41.118}"
export TARGET_PORT="${TARGET_PORT:-48560}"
export TARGET_SCHEME="${TARGET_SCHEME:-http}"

export VLESS_PATH="${VLESS_PATH:-/}"
export LINK_NAME="${LINK_NAME:-g2ray-lite}"

echo
echo "Starting g2ray-lite-forwarder-go..."
echo "Target: ${TARGET_SCHEME}://${TARGET_HOST}:${TARGET_PORT}"
echo

go run ./main.go
