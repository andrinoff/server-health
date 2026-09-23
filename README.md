# server-health

A one-machine dashboard. It shows what your server is doing (CPU, memory,
load, disk) and the state of the systemd units you name, and it can start,
stop and restart those units from the browser.

Built for a private machine: a single Go binary with the frontend embedded,
bound to loopback, reached over [Tailscale](https://tailscale.com) or a local
reverse proxy. No accounts, no public ports.

```
     masthead            hobbes · Ubuntu 24.04 / kernel 6.8.0-41 / amd64
     readings            uptime 12d 04h   load 0.14 0.09 0.06   cores 4
     recorder            cpu ─────╮        4%
                                  ╰──────
                         mem ─────╮       38%
                                  ╰──────
     units               home.service   running   08:12  41 MB  pid 4211
     storage             disk ▓▓▓▓░░░░░░ 3.0 of 7.8 GB
```

## What it does

| Band | Contents |
| --- | --- |
| Masthead | Hostname, OS, kernel, architecture, last reading time |
| Readings | Uptime, load average, core count, memory, disk |
| Recorder | Two stacked plots of CPU and memory over the last five minutes |
| Units | State of each configured systemd unit, with start time, memory and PID |
| Storage | Disk use for the watched filesystem, plus swap when there is any |

Readings come from `/proc` and `statfs`, sampled every 2 seconds into a
five minute ring buffer on the server, so the trace is already there when you
open the page. Hover either plot to read a moment in the past: both pens move
together, the way a two-pen recorder works.

On a machine without `/proc` (a Mac, say) the page says so instead of showing
numbers it does not have.

## Run it

```bash
make build
./server-health -addr 127.0.0.1:8080 -services home.service,caddy.service
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8080` | Listen address. Keep it on loopback. |
| `-services` | `server-health` | Comma-separated units to show and control. |
| `-disk` | `/` | Filesystem to report disk usage for. |
| `-demo` | off | Invent readings and units, to preview the page off Linux. |

## Install on Ubuntu

Name the units you want on the page; they go into the service's `-services`
flag:

```bash
make build
sudo ./deploy/install.sh home.service caddy.service
```

The installer listens on `127.0.0.1:8080` unless you say otherwise. If
something else already holds that port (another dashboard, for instance):

```bash
sudo SERVER_HEALTH_ADDR=127.0.0.1:8081 ./deploy/install.sh home.service caddy.service
```

Whatever you choose, that is the address your reverse proxy should point at.

That creates a `server-health` system user, installs the binary to
`/opt/server-health`, writes and starts the systemd unit, and prints the units
it is watching. To change them later, edit `ExecStart` in
`/etc/systemd/system/server-health.service` and run:

```bash
sudo systemctl daemon-reload && sudo systemctl restart server-health
```

Only the units on that list are shown, and only they can ever be controlled: a
unit name that is not on the list is rejected before anything reaches systemd.
A unit that does not exist still appears, marked as not installed, which is
the useful answer when a service fails to come back.

Upgrading is `make build && sudo ./deploy/install.sh <same units>` again.

### Allowing the buttons

Reading state needs no special permission, so the page works as soon as it
runs. Starting, stopping and restarting do need permission. Grant it with a
polkit rule covering exactly the units you named:

```bash
sudo ./deploy/service-control.sh home.service caddy.service
```

The rule lives at `/etc/polkit-1/rules.d/50-server-health.rules`. It is not
sudo, it does not weaken the unit's own hardening (`NoNewPrivileges`,
`ProtectSystem=strict`), and it is reversible:

```bash
sudo ./deploy/service-control.sh --remove
```

If a button reports that the action is not allowed, the rule did not load:
`journalctl -u polkit -n 20 --no-pager`.

Restarting `server-health` itself restarts the page you are looking at. It
notices the dropped connection, keeps polling, and tells you when the server
answers again.

## Reach it privately

Loopback plus a private network is the whole story; there is nothing to expose.

```bash
# on the server
sudo tailscale up
sudo tailscale serve --bg 127.0.0.1:8080
# then open https://<server>.<tailnet>.ts.net from any device in your tailnet
```

To use your own domain while staying off the public internet, point an A record
at the server's private (Tailscale) IP with the proxy off, give Caddy a DNS-01
certificate and reverse-proxy to the address from the install step:

```
server.andrinoff.com {
	tls {
		dns cloudflare {env.CF_DNS_API_TOKEN}   # or an inline token, as in your Caddyfile
	}
	reverse_proxy 127.0.0.1:8080
}
```

```bash
sudo systemctl reload caddy     # the DNS record must resolve inside the tailnet
```

## Develop

Needs Go ≥ 1.24 and Node ≥ 18.

```bash
make dev-api   # Go API on :8080
make dev-web   # Vite dev server on :5173, /api proxied to :8080
make test      # go vet + go test
make fmt       # tsc --noEmit + gofmt
```

Readings need Linux, so work on the page from a Mac with demo mode. It serves
the same shapes with an invented machine (`demo-machine`, and it says so in the
log), so the recorder moves and the unit buttons work:

```bash
./server-health -demo -services home.service,caddy.service,stale.service
```

Demo mode changes nothing and reads nothing: it is a preview harness, not a
fallback.

## API

```
GET  /api/health                            {"status":"ok"}
GET  /api/host                              machine readings + unit states
GET  /api/host/services                     unit states only
POST /api/host/services/{name}/{action}     action: start | stop | restart
```

## Layout

```
cmd/server-health/  entry point (flags, server wiring)
internal/host/      /proc readings, systemd queries and actions
internal/demo/      invented readings for previewing the page off Linux
internal/api/       routes, JSON handlers, embedded frontend
web/                React + Vite frontend (built into the binary)
deploy/             systemd unit, install.sh, service-control.sh
```

## Notes and limits

- Linux only for readings. `internal/host` reads `/proc/stat`,
  `/proc/meminfo`, `/proc/uptime`, `/proc/loadavg` and `/etc/os-release`.
- Unit control shells out to `systemctl`; nothing escalates privileges, and
  the allowed actions are exactly `start`, `stop` and `restart`.
- No authentication by design. It binds to loopback and has no accounts:
  keep it on your tailnet, or behind a proxy that adds auth.
