#!/usr/bin/env bash
set -euo pipefail

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
echo

# Run the Go proxy
exec go run ./main.go
