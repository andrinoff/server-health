import type { HostService, HostSnapshot, ServiceAction } from './types'
import { toast } from './components/Toast'

interface RequestOptions {
  silent?: boolean
}

async function request<T>(method: string, path: string, body?: unknown, opts: RequestOptions = {}): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`
    try {
      const data = await res.json()
      if (data?.error) msg = data.error
    } catch {
      /* keep the status line */
    }
    if (!opts.silent) toast(msg, 'error')
    throw new Error(msg)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  // Polled, so failures stay silent; the page draws the no-answer state.
  host: (opts?: RequestOptions) => request<HostSnapshot>('GET', '/api/host', undefined, opts),
  services: () => request<HostService[]>('GET', '/api/host/services'),
  control: (unit: string, action: ServiceAction) =>
    request<HostService>('POST', `/api/host/services/${encodeURIComponent(unit)}/${action}`),
}

// --- readout formatting ---

export function formatBytes(bytes: number): string {
  if (!bytes) return '0 MB'
  const units: [number, string][] = [
    [1024 ** 4, 'TB'],
    [1024 ** 3, 'GB'],
    [1024 ** 2, 'MB'],
    [1024, 'KB'],
  ]
  for (const [size, label] of units) {
    if (bytes >= size) {
      const value = bytes / size
      return `${value >= 10 ? Math.round(value) : value.toFixed(1)} ${label}`
    }
  }
  return `${bytes} B`
}

// The two largest units that matter for a home server: "12d 04h", "4h 31m".
export function formatUptime(seconds: number): string {
  if (seconds <= 0) return '—'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${String(hours).padStart(2, '0')}h`
  if (hours > 0) return `${hours}h ${String(minutes).padStart(2, '0')}m`
  if (minutes > 0) return `${minutes}m`
  return 'under a minute'
}

// When a unit entered its current state, keeping the date only if not today.
export function formatSince(iso: string): string {
  if (!iso) return ''
  const then = new Date(iso)
  if (Number.isNaN(then.getTime())) return ''
  const time = then.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  const sameDay = then.toDateString() === new Date().toDateString()
  if (sameDay) return time
  return `${then.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} ${time}`
}

export function formatClock(ms: number): string {
  return new Date(ms).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

// Axis labels count back from now: "-4m", "-30s".
export function formatElapsed(ms: number): string {
  const seconds = Math.round(ms / 1000)
  if (seconds % 60 === 0 && seconds >= 60) return `-${seconds / 60}m`
  if (seconds >= 60) return `-${Math.floor(seconds / 60)}m${seconds % 60}s`
  return `-${seconds}s`
}
