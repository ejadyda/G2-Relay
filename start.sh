#!/usr/bin/env bash
set -euo pipefail

# Set defaults for proxy configuration
export LISTEN_ADDR="${LISTEN_ADDR:-0.0.0.0:3000}"
export TARGET_HOST="${TARGET_HOST:-212.95.41.118}"
export TARGET_PORT="${TARGET_PORT:-48560}"
export TARGET_SCHEME="${TARGET_SCHEME:-http}"
export VLESS_PATH="${VLESS_PATH:-/}"
export LINK_NAME="${LINK_NAME:-g2ray-lwq4w11y}"

# Logging prefix
log_info() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] ℹ️  $*"
}

log_success() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] ✓ $*"
}

# Try to make port 3000 public using GitHub CLI
if [ -n "${CODESPACE_NAME:-}" ] && command -v gh >/dev/null 2>&1; then
  log_info "Attempting to make port 3000 public in Codespaces..."
  if gh codespace ports visibility 3000:public -c "$CODESPACE_NAME" 2>/dev/null; then
    log_success "Port 3000 is now public"
  else
    log_info "Could not set port visibility (may already be public or not available)"
  fi
  echo
fi

log_info "Starting g2ray-lite-forwarder..."
log_info "Configuration:"
log_info "  LISTEN_ADDR:       $LISTEN_ADDR"
log_info "  TARGET_HOST:       $TARGET_HOST"
log_info "  TARGET_PORT:       $TARGET_PORT"
log_info "  TARGET_SCHEME:     $TARGET_SCHEME"

# Run the Go proxy
exec go run ./main.go

