export interface SeriesPoint {
  t: number
  cpu: number
  mem: number
}

export interface DiskUsage {
  path: string
  total: number
  used: number
  available: number
  pct: number
}

export type ServiceAction = 'start' | 'stop' | 'restart'

export interface HostService {
  name: string
  unit: string
  description: string
  loadState: string
  activeState: string
  subState: string
  enabled: string
  mainPid: number
  memory: number
  since: string
  error?: string
}

export interface HostSnapshot {
  supported: boolean
  hostname: string
  os: string
  kernel: string
  arch: string
  cores: number
  uptime: number
  bootedAt: string
  load: number[]
  cpu: number
  memTotal: number
  memUsed: number
  memPct: number
  swapTotal: number
  swapUsed: number
  disk: DiskUsage
  intervalMs: number
  windowMs: number
  series: SeriesPoint[]
  services: HostService[]
  error?: string
}
