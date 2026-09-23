#!/usr/bin/env bash
# Let the dashboard's service user start, stop and restart specific systemd
# units, so the buttons on the page work.
#
# Usage: sudo ./deploy/service-control.sh home.service caddy.service
#        sudo ./deploy/service-control.sh --remove
#
# Reading a unit's state needs no permission, so this is only about the
# buttons. Permission is granted through polkit rather than sudo: one rule that
# allows the `server-health` user to manage exactly the listed units and
# nothing else. No password, no privilege escalation, and no change to the
# unit's own hardening.
set -euo pipefail

RULES_FILE="/etc/polkit-1/rules.d/50-server-health.rules"
SERVICE_USER="${SERVER_HEALTH_USER:-server-health}"

if [[ $EUID -ne 0 ]]; then
  echo "error: run as root (sudo ./deploy/service-control.sh ...)" >&2
  exit 1
fi

if [[ "${1:-}" == "--remove" ]]; then
  rm -f "$RULES_FILE"
  echo "Removed $RULES_FILE: the dashboard can no longer start or stop units."
  exit 0
fi

if [[ $# -eq 0 ]]; then
  echo "usage: sudo ./deploy/service-control.sh <unit> [unit...]" >&2
  echo "example: sudo ./deploy/service-control.sh home.service caddy.service" >&2
  exit 1
fi

if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  echo "error: user '$SERVICE_USER' does not exist (install the service first)" >&2
  exit 1
fi

units=()
for arg in "$@"; do
  unit="${arg%.service}.service"
  # Unit names go into a polkit JavaScript rule below, so keep them to the
  # characters systemd itself allows.
  if [[ ! "$unit" =~ ^[A-Za-z0-9@:._-]+\.service$ ]]; then
    echo "error: '$arg' is not a valid unit name" >&2
    exit 1
  fi
  units+=("$unit")
done

echo "==> Writing $RULES_FILE"
{
  cat <<EOF
// Managed by server-health (deploy/service-control.sh). Grants the
// '$SERVICE_USER' user permission to manage these units and no others:
EOF
  printf '//   %s\n' "${units[@]}"
  cat <<EOF

polkit.addRule(function (action, subject) {
    if (action.id !== "org.freedesktop.systemd1.manage-units") {
        return polkit.Result.NOT_HANDLED;
    }
    if (subject.user !== "$SERVICE_USER") {
        return polkit.Result.NOT_HANDLED;
    }
    var units = [
EOF
  printf '        "%s",\n' "${units[@]}" | sed '$ s/,$//'
  cat <<EOF
    ];
    if (units.indexOf(action.lookup("unit")) === -1) {
        return polkit.Result.NOT_HANDLED;
    }
    return polkit.Result.YES;
});
EOF
} > "$RULES_FILE"
chmod 0644 "$RULES_FILE"
chown root:root "$RULES_FILE"

echo "==> Reloading polkit"
systemctl restart polkit 2>/dev/null || systemctl restart polkitd 2>/dev/null || true

echo
echo "OK: the dashboard may now start, stop and restart:"
printf '  %s\n' "${units[@]}"
echo
echo "If a button still reports that the action is not allowed, check the rule loaded:"
echo "  journalctl -u polkit -n 20 --no-pager"
echo "Undo at any time with: sudo ./deploy/service-control.sh --remove"
