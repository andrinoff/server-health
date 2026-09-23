#!/usr/bin/env bash
# One-shot installer for the server-health dashboard on Ubuntu.
# Usage: sudo ./deploy/install.sh   (build the binary first with `make build`)
set -euo pipefail

BIN="server-health"
BIN_DIR="/opt/server-health"
USER="server-health"
SERVICE="server-health.service"

# Port to listen on. 8080 is often taken (by another app on the same box), so
# override it with: sudo SERVER_HEALTH_ADDR=127.0.0.1:8081 ./deploy/install.sh ...
ADDR="${SERVER_HEALTH_ADDR:-127.0.0.1:8080}"
PORT="${ADDR##*:}"

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ $EUID -ne 0 ]]; then
  echo "error: run as root (sudo ./deploy/install.sh)" >&2
  exit 1
fi

if [[ ! -f "$REPO_DIR/$BIN" ]]; then
  echo "error: '$BIN' not found in $REPO_DIR. Run 'make build' first." >&2
  exit 1
fi

# Units to watch and control, e.g. ./deploy/install.sh home.service caddy.service
services=""
for arg in "$@"; do
  unit="${arg%.service}.service"
  if [[ ! "$unit" =~ ^[A-Za-z0-9@:._-]+\.service$ ]]; then
    echo "error: '$arg' is not a valid unit name" >&2
    exit 1
  fi
  services="${services:+$services,}$unit"
done

echo "==> Creating service user"
if ! id -u "$USER" >/dev/null 2>&1; then
  useradd --system --home-dir "$BIN_DIR" --shell /usr/sbin/nologin "$USER"
fi

echo "==> Installing binary"
install -d -m 0755 "$BIN_DIR"
install -m 0755 "$REPO_DIR/$BIN" "$BIN_DIR/$BIN"

echo "==> Installing systemd unit"
install -m 0644 "$REPO_DIR/deploy/$SERVICE" "/etc/systemd/system/$SERVICE"
sed -i "s|-addr [^ ]*|-addr $ADDR|" "/etc/systemd/system/$SERVICE"
if [[ -n "$services" ]]; then
  sed -i "s|-services [^ ]*|-services $services|" "/etc/systemd/system/$SERVICE"
fi
systemctl daemon-reload

echo "==> Starting service"
systemctl enable --now "$SERVICE"
sleep 1

if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then
  echo "OK: the dashboard is up on $ADDR"
else
  echo "Started, but the health check failed. Check:"
  echo "  systemctl status $SERVICE --no-pager"
  echo "  journalctl -u $SERVICE -e --no-pager"
fi

echo
echo "Running: $(grep -m1 '^ExecStart=' "/etc/systemd/system/$SERVICE" | cut -d= -f2-)"
echo
if [[ -n "$services" ]]; then
  echo "Watching: $services"
else
  echo "Next: choose the units to watch by editing ExecStart in /etc/systemd/system/$SERVICE, then:"
  echo "  sudo systemctl daemon-reload && sudo systemctl restart $SERVICE"
fi
echo
echo "Allow the start/stop/restart buttons (same units as above):"
echo "  sudo ./deploy/service-control.sh $services"
