#!/usr/bin/env bash
set -euo pipefail

# Set defaults for proxy configuration
export LISTEN_ADDR="${LISTEN_ADDR:-0.0.0.0:3000}"
export TARGET_HOST="${TARGET_HOST:-212.95.41.118}"
export TARGET_PORT="${TARGET_PORT:-48560}"
export TARGET_SCHEME="${TARGET_SCHEME:-http}"
export VLESS_PATH="${VLESS_PATH:-/}"
export LINK_NAME="${LINK_NAME:-g2ray-lwq4w11y}"

# Logging helpers
log_info() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] ℹ️  $*"
}

log_success() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] ✓ $*"
}

log_warn() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] ⚠️  $*"
}

# Try to make port 3000 public using GitHub CLI.
# This runs in background because sometimes Codespaces registers the forwarded port
# a few seconds after postStartCommand starts.
make_port_public() {
  if [ -z "${CODESPACE_NAME:-}" ]; then
    log_info "CODESPACE_NAME is empty; skipping Codespaces port visibility setup"
    return 0
  fi

  if ! command -v gh >/dev/null 2>&1; then
    log_warn "gh CLI not found; skipping Codespaces port visibility setup"
    return 0
  fi

  log_info "Attempting to make port 3000 public in Codespaces..."

  for i in 1 2 3 4 5 6 7 8 9 10; do
    log_info "Port visibility attempt $i/10"

    if gh codespace ports visibility 3000:public -c "$CODESPACE_NAME" >/dev/null 2>&1; then
      log_success "Port 3000 is now public"
      return 0
    fi

    sleep 3
  done

  log_warn "Could not make port 3000 public automatically"
  log_warn "Open the Ports tab and set port 3000 visibility to Public manually"
}

make_port_public &

log_info "Starting g2ray-lite-forwarder..."
log_info "Configuration:"
log_info "  LISTEN_ADDR:       $LISTEN_ADDR"
log_info "  TARGET_HOST:       $TARGET_HOST"
log_info "  TARGET_PORT:       $TARGET_PORT"
log_info "  TARGET_SCHEME:     $TARGET_SCHEME"
log_info "  VLESS_PATH:        $VLESS_PATH"
log_info "  LINK_NAME:         $LINK_NAME"

if [ -n "${CLIENT_ADDRESS_OVERRIDE:-}" ]; then
  log_info "  CLIENT_OVERRIDE:   $CLIENT_ADDRESS_OVERRIDE"
fi

if [ -z "${VLESS_UUID:-}" ]; then
  log_warn "VLESS_UUID is empty; final VLESS link may not be printed"
else
  log_success "VLESS_UUID is loaded from environment"
fi

echo

# Run the Go proxy
exec go run ./main.go