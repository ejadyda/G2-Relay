#!/usr/bin/env bash
set -euo pipefail

export PORT="${PORT:-3000}"
export TARGET_HOST="${TARGET_HOST:-212.95.41.118}"
export TARGET_PORT="${TARGET_PORT:-48560}"
export TARGET_SCHEME="${TARGET_SCHEME:-http}"
export VLESS_UUID="${VLESS_UUID:-e6c16592-f8bf-4032-9a8d-1dcf9e8a5e94}"
export VLESS_PATH="${VLESS_PATH:-/}"
export LINK_NAME="${LINK_NAME:-g2ray-lite}"

echo
echo "Starting g2ray-lite-forwarder..."
echo "Target: ${TARGET_SCHEME}://${TARGET_HOST}:${TARGET_PORT}"
echo

npm start
