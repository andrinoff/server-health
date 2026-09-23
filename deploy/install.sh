#!/usr/bin/env bash
# One-shot installer for the server-health dashboard on Ubuntu.
# Usage: sudo ./deploy/install.sh   (build the binary first with `make build`)
set -euo pipefail

BIN="server-health"
BIN_DIR="/opt/server-health"
USER="server-health"
SERVICE="server-health.service"

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ $EUID -ne 0 ]]; then
  echo "error: run as root (sudo ./deploy/install.sh)" >&2
  exit 1
fi

if [[ ! -f "$REPO_DIR/$BIN" ]]; then
  echo "error: '$BIN' not found in $REPO_DIR. Run 'make build' first." >&2
  exit 1
fi

echo "==> Creating service user"
if ! id -u "$USER" >/dev/null 2>&1; then
  useradd --system --home-dir "$BIN_DIR" --shell /usr/sbin/nologin "$USER"
fi

echo "==> Installing binary"
install -d -m 0755 "$BIN_DIR"
install -m 0755 "$REPO_DIR/$BIN" "$BIN_DIR/$BIN"

echo "==> Installing systemd unit"
install -m 0644 "$REPO_DIR/deploy/$SERVICE" "/etc/systemd/system/$SERVICE"
systemctl daemon-reload

echo "==> Starting service"
systemctl enable --now "$SERVICE"
sleep 1

if curl -fsS http://127.0.0.1:8080/api/health >/dev/null 2>&1; then
  echo "OK: the dashboard is up at http://127.0.0.1:8080"
else
  echo "Started, but the health check failed. Check:"
  echo "  systemctl status $SERVICE --no-pager"
  echo "  journalctl -u $SERVICE -e --no-pager"
fi

echo
echo "Next steps:"
echo "  1. Choose the units it shows:  edit ExecStart in /etc/systemd/system/$SERVICE"
echo "  2. Allow the buttons:          sudo ./deploy/service-control.sh <unit> [unit...]"
echo "  3. Reach it privately:         sudo tailscale serve --bg 127.0.0.1:8080"
