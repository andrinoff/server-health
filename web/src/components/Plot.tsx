import { useEffect, useRef, useState } from 'react'
import type { SeriesPoint } from '../types'

interface PlotProps {
  pen: 'cpu' | 'mem'
  series: SeriesPoint[]
  windowMs: number
  intervalMs: number
  value: number
  hoverIndex: number | null
  onHover: (index: number | null) => void
  height?: number
}

const PAD = { top: 8, bottom: 0, right: 7 }
const GRID = [25, 50, 75]

function splitGaps(series: SeriesPoint[], gapMs: number): SeriesPoint[][] {
  const out: SeriesPoint[][] = []
  let run: SeriesPoint[] = []
  for (const point of series) {
    if (run.length > 0 && point.t - run[run.length - 1].t > gapMs) {
      out.push(run)
      run = []
    }
    run.push(point)
  }
  if (run.length > 0) out.push(run)
  return out
}

// One pen of the recorder: a trace drawn against the paper grid, with its
// current value printed beside the live edge, the way a chart recorder prints
// the pen position.
export function Plot({ pen, series, windowMs, intervalMs, value, hoverIndex, onHover, height = 112 }: PlotProps) {
  const plotRef = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)

  useEffect(() => {
    const el = plotRef.current
    if (!el) return
    setWidth(el.clientWidth)
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const plotW = Math.max(1, width - PAD.right)
  const plotH = height - PAD.top - PAD.bottom
  const last = series.length > 0 ? series[series.length - 1] : null
  const now = last ? last.t : Date.now()
  const gapMs = Math.max(intervalMs, 1000) * 3

  const x = (t: number) => plotW - ((now - t) / windowMs) * plotW
  const y = (percent: number) => PAD.top + (1 - percent / 100) * plotH

  const line = (run: SeriesPoint[]) =>
    run.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.t).toFixed(2)} ${y(p[pen]).toFixed(2)}`).join(' ')

  const area = (run: SeriesPoint[]) => {
    const base = (PAD.top + plotH).toFixed(2)
    return `${line(run)} L${x(run[run.length - 1].t).toFixed(2)} ${base} L${x(run[0].t).toFixed(2)} ${base} Z`
  }

  const runs = series.length > 1 ? splitGaps(series, gapMs) : []
  const marked = hoverIndex !== null && hoverIndex >= 0 && hoverIndex < series.length ? series[hoverIndex] : null

  const onPointerMove = (e: React.PointerEvent<SVGSVGElement>) => {
    if (series.length === 0) return
    const rect = e.currentTarget.getBoundingClientRect()
    const px = ((e.clientX - rect.left) / rect.width) * width
    const target = now - ((plotW - px) / plotW) * windowMs
    let best = -1
    let bestDistance = Infinity
    series.forEach((p, i) => {
      const distance = Math.abs(p.t - target)
      if (distance < bestDistance) {
        bestDistance = distance
        best = i
      }
    })
    onHover(bestDistance > gapMs ? null : best)
  }

  return (
    <div className={`pen ${pen}`}>
      <span className="pen-label">{pen}</span>
      <div className="pen-plot" ref={plotRef}>
        <svg
          width={width}
          height={height}
          role="img"
          aria-label={`${pen} use over the last ${Math.round(windowMs / 60000)} minutes`}
          onPointerMove={onPointerMove}
          onPointerLeave={() => onHover(null)}
        >
          <defs>
            <linearGradient id={`ink-${pen}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={`var(--pen-${pen})`} stopOpacity="0.22" />
              <stop offset="100%" stopColor={`var(--pen-${pen})`} stopOpacity="0" />
            </linearGradient>
          </defs>

          {GRID.map((percent) => (
            <line
              key={percent}
              className="grid-line"
              x1="0"
              x2={plotW + PAD.right}
              y1={y(percent)}
              y2={y(percent)}
              shapeRendering="crispEdges"
            />
          ))}
          <line className="grid-base" x1="0" x2={plotW + PAD.right} y1={y(0)} y2={y(0)} shapeRendering="crispEdges" />

          {runs.map((run, i) => (
            <path key={`fill-${i}`} className="trace-fill" d={area(run)} fill={`url(#ink-${pen})`} />
          ))}
          {runs.map((run, i) => (
            <path key={`line-${i}`} className="trace" d={line(run)} />
          ))}

          {last && (
            <>
              <line className="edge-tick" x1={plotW} x2={plotW + PAD.right} y1={y(last[pen])} y2={y(last[pen])} />
              <circle className="edge-dot" cx={plotW} cy={y(last[pen])} r="3" />
            </>
          )}

          {marked && (
            <>
              <line className="cross" x1={x(marked.t)} x2={x(marked.t)} y1={PAD.top} y2={y(0)} />
              <circle className="cross-dot" cx={x(marked.t)} cy={y(marked[pen])} r="3" />
            </>
          )}
        </svg>
      </div>
      <div className="pen-value">
        <b>{value.toFixed(0)}</b>
        <i>%</i>
      </div>
    </div>
  )
}
