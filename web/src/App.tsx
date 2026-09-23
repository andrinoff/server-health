import { useCallback, useEffect, useRef, useState } from 'react'
import { api, formatBytes, formatClock, formatElapsed, formatSince, formatUptime } from './api'
import type { HostService, HostSnapshot, ServiceAction } from './types'
import { Plot } from './components/Plot'
import { Modal } from './components/Modal'
import { toast } from './components/Toast'

const POLL_MS = 2500
const AXIS_TICKS = 6

type State = 'running' | 'stopped' | 'failed' | 'changing' | 'missing' | 'unavailable'

const ACTING: Record<ServiceAction, string> = { start: 'starting', stop: 'stopping', restart: 'restarting' }

function stateOf(service: HostService): State {
  if (service.error) return 'unavailable'
  if (service.loadState === 'not-found' || service.loadState === 'masked') return 'missing'
  switch (service.activeState) {
    case 'active':
      return 'running'
    case 'failed':
      return 'failed'
    case 'activating':
    case 'deactivating':
    case 'reloading':
      return 'changing'
    default:
      return 'stopped'
  }
}

// The mid column of a unit row: when it started, how much memory it holds,
// which process it is. Empty for units that are down.
function unitMeta(service: HostService, state: State): string {
  if (state === 'unavailable' || state === 'missing') return service.error || 'no such unit'
  if (state !== 'running') {
    return service.enabled === 'enabled' ? 'starts on boot' : 'does not start on boot'
  }
  const facts = [formatSince(service.since) && `since ${formatSince(service.since)}`]
  if (service.memory) facts.push(formatBytes(service.memory))
  if (service.mainPid) facts.push(`pid ${service.mainPid}`)
  return facts.filter(Boolean).join('   ')
}

export function App() {
  const [host, setHost] = useState<HostSnapshot | null>(null)
  const [offline, setOffline] = useState(false)
  const [pending, setPending] = useState<{ unit: string; action: ServiceAction } | null>(null)
  const [confirming, setConfirming] = useState<{ service: HostService; action: ServiceAction } | null>(null)
  const [hover, setHover] = useState<number | null>(null)
  const wasOffline = useRef(false)

  const load = useCallback(async () => {
    try {
      const snapshot = await api.host({ silent: true })
      setHost(snapshot)
      setOffline(false)
      if (wasOffline.current) {
        wasOffline.current = false
        toast('The server answered again.')
      }
    } catch {
      wasOffline.current = true
      setOffline(true)
    }
  }, [])

  useEffect(() => {
    load()
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') load()
    }, POLL_MS)
    const onVisible = () => {
      if (document.visibilityState === 'visible') load()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [load])

  const act = async (service: HostService, action: ServiceAction) => {
    setConfirming(null)
    setPending({ unit: service.unit, action })
    try {
      await api.control(service.unit, action)
      await load()
    } catch (err) {
      // A dropped connection here usually means we just restarted whatever
      // serves this page. Anything else has already been reported.
      if (err instanceof TypeError) {
        wasOffline.current = true
        setOffline(true)
        toast(`${service.unit} is ${ACTING[action]}. Waiting for the server to answer.`, 'ok')
      }
    } finally {
      setPending(null)
    }
  }

  if (!host) return <div className="sheet blank">Reading the machine…</div>

  const services = host.services
  const running = services.filter((s) => stateOf(s) === 'running').length
  const points = host.series
  const marked = hover !== null && hover >= 0 && hover < points.length ? points[hover] : null
  const readAt = points.length > 0 ? points[points.length - 1].t : 0
  const busy = (service: HostService) => (pending && pending.unit === service.unit ? pending.action : null)
  const cpu = marked ? marked.cpu : host.cpu
  const mem = marked ? marked.mem : host.memPct

  return (
    <div className="sheet">
      <header className="masthead">
        <div className="brand">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
            <rect x="2.5" y="4" width="19" height="6.5" />
            <rect x="2.5" y="13.5" width="19" height="6.5" />
            <path d="M6 7.2h.01M6 16.7h.01" />
          </svg>
          server-health
        </div>
        <div className="machine">
          <b>{host.hostname}</b>
          <span>
            {host.os} / {host.supported ? `kernel ${host.kernel} / ` : ''}
            {host.arch}
          </span>
        </div>
        <div className="readout-stamp">
          {offline ? (
            <span className="stamp alert">no answer</span>
          ) : (
            <>
              <span className="stamp">{services.length > 0 && `${running}/${services.length} units up`}</span>
              {readAt > 0 && <span className="hint">read {formatClock(readAt)}</span>}
            </>
          )}
        </div>
      </header>

      {host.supported && (
        <dl className="strip">
          <Reading label="uptime" value={formatUptime(host.uptime)} />
          <Reading label="load" value={host.load.map((l) => l.toFixed(2)).join('  ')} />
          <Reading label="cores" value={String(host.cores)} />
          <Reading label="memory" value={`${formatBytes(host.memUsed)} / ${formatBytes(host.memTotal)}`} />
          {host.disk.total > 0 && <Reading label="disk" value={`${host.disk.pct.toFixed(0)}%`} />}
        </dl>
      )}

      {!host.supported && (
        <p className="notice">
          <b>Live readings need Linux.</b> {host.error || 'CPU, memory and disk are read from /proc.'}
        </p>
      )}

      {host.supported && (
        <section className="band recorder">
          <div className="band-head">
            <h2>Recorder</h2>
            <span className="hint">
              {marked
                ? `reading at ${formatClock(marked.t)}`
                : `sampled every ${(host.intervalMs / 1000).toFixed(0)} s`}
            </span>
          </div>
          {points.length > 1 ? (
            <>
              <Plot
                pen="cpu"
                series={points}
                windowMs={host.windowMs}
                intervalMs={host.intervalMs}
                value={cpu}
                hoverIndex={hover}
                onHover={setHover}
              />
              <Plot
                pen="mem"
                series={points}
                windowMs={host.windowMs}
                intervalMs={host.intervalMs}
                value={mem}
                hoverIndex={hover}
                onHover={setHover}
              />
              <div className="axis">
                <span />
                <div className="axis-ticks">
                  {Array.from({ length: AXIS_TICKS }, (_, i) => {
                    const elapsed = host.windowMs * (1 - i / (AXIS_TICKS - 1))
                    return (
                      <span key={i} className="axis-tick">
                        {i === AXIS_TICKS - 1 ? 'now' : formatElapsed(elapsed)}
                      </span>
                    )
                  })}
                </div>
                <span />
              </div>
            </>
          ) : (
            <p className="hold">Taking the first readings…</p>
          )}
        </section>
      )}

      <section className="band units">
        <div className="band-head">
          <h2>Units</h2>
          <span className="hint">
            {services.length > 0 ? `${running} of ${services.length} running` : 'none configured'}
          </span>
        </div>
        {services.length === 0 ? (
          <p className="notice">
            <b>No units configured.</b> Start the server with{' '}
            <code>-services home.service,caddy.service</code> to watch them here.
          </p>
        ) : (
          <ul className="unit-list">
            {services.map((service) => {
              const state = stateOf(service)
              const action = busy(service)
              const controllable = state !== 'unavailable' && state !== 'missing'
              const up = state === 'running' || state === 'changing'
              return (
                <li key={service.unit} className={`unit ${state}`}>
                  <span className="unit-name">{service.unit}</span>
                  <span className="unit-state">
                    <i className="mark" aria-hidden="true" />
                    {action ? ACTING[action] : state}
                  </span>
                  <span className="unit-meta">{unitMeta(service, state)}</span>
                  {controllable && (
                    <span className="unit-actions">
                      {up ? (
                        <>
                          <button
                            className="btn"
                            disabled={action !== null}
                            onClick={() => setConfirming({ service, action: 'restart' })}
                          >
                            Restart
                          </button>
                          <button
                            className="btn"
                            disabled={action !== null}
                            onClick={() => setConfirming({ service, action: 'stop' })}
                          >
                            Stop
                          </button>
                        </>
                      ) : (
                        <button className="btn solid" disabled={action !== null} onClick={() => act(service, 'start')}>
                          Start
                        </button>
                      )}
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </section>

      {host.supported && (host.disk.total > 0 || host.swapTotal > 0) && (
        <section className="band storage">
          <div className="band-head">
            <h2>Storage</h2>
            <span className="hint">{host.disk.path}</span>
          </div>
          <div className="gauges">
            {host.disk.total > 0 && (
              <Gauge
                label="disk"
                pct={host.disk.pct}
                detail={`${formatBytes(host.disk.used)} of ${formatBytes(host.disk.total)}`}
              />
            )}
            {host.swapTotal > 0 && (
              <Gauge
                label="swap"
                pct={(host.swapUsed / host.swapTotal) * 100}
                detail={`${formatBytes(host.swapUsed)} of ${formatBytes(host.swapTotal)}`}
              />
            )}
          </div>
        </section>
      )}

      <footer className="sheet-foot">
        <span>server-health</span>
        <span>sampled every {host.supported ? (host.intervalMs / 1000).toFixed(0) : '—'} s</span>
        <span>{Math.round(host.windowMs / 60000)} minute window</span>
      </footer>

      {confirming && (
        <Modal
          title={
            confirming.action === 'restart'
              ? `Restart ${confirming.service.unit}?`
              : `Stop ${confirming.service.unit}?`
          }
          command={`${confirming.action} ${confirming.service.unit}`}
          confirmLabel={confirming.action === 'restart' ? 'Restart' : 'Stop'}
          onConfirm={() => act(confirming.service, confirming.action)}
          onClose={() => setConfirming(null)}
        >
          <p>
            {confirming.action === 'restart'
              ? 'Anything it is doing right now is interrupted, then it starts again.'
              : 'It stays down until you start it again.'}
          </p>
        </Modal>
      )}
    </div>
  )
}

function Reading({ label, value }: { label: string; value: string }) {
  return (
    <div className="reading">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}

function Gauge({ label, pct, detail }: { label: string; pct: number; detail: string }) {
  const width = Math.max(0, Math.min(100, pct))
  return (
    <div className="gauge">
      <span className="gauge-label">{label}</span>
      <span className={`gauge-track ${width >= 90 ? 'alert' : ''}`}>
        <span className="gauge-fill" style={{ width: `${width}%` }} />
      </span>
      <span className="gauge-detail">{detail}</span>
      <span className="gauge-pct">{width.toFixed(0)}%</span>
    </div>
  )
}
