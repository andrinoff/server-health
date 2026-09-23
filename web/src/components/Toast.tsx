import { useEffect, useState } from 'react'

export type ToastTone = 'error' | 'ok'

type Listener = (message: string, tone: ToastTone) => void
const listeners = new Set<Listener>()

// Tiny pub/sub so api.ts can surface failures without context plumbing.
export function toast(message: string, tone: ToastTone = 'ok') {
  listeners.forEach((l) => l(message, tone))
}

interface ToastItem {
  id: number
  message: string
  tone: ToastTone
}

let nextId = 1

export function Toaster() {
  const [items, setItems] = useState<ToastItem[]>([])

  useEffect(() => {
    const listener: Listener = (message, tone) => {
      const id = nextId++
      setItems((prev) => [...prev, { id, message, tone }])
      setTimeout(() => setItems((prev) => prev.filter((t) => t.id !== id)), 5000)
    }
    listeners.add(listener)
    return () => {
      listeners.delete(listener)
    }
  }, [])

  return (
    <div className="notes" role="status" aria-live="polite">
      {items.map((item) => (
        <button
          key={item.id}
          className={`note ${item.tone}`}
          onClick={() => setItems((prev) => prev.filter((t) => t.id !== item.id))}
        >
          {item.message}
        </button>
      ))}
    </div>
  )
}
