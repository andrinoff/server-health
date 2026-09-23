import type { ReactNode } from 'react'

interface ModalProps {
  title: string
  command: string
  confirmLabel: string
  onConfirm: () => void
  onClose: () => void
  children: ReactNode
}

// Confirmations name the command they will run, so the page and the shell
// never disagree about what a button does.
export function Modal({ title, command, confirmLabel, onConfirm, onClose, children }: ModalProps) {
  return (
    <div className="scrim" onMouseDown={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h2 className="modal-title">{title}</h2>
        {children}
        <p className="modal-command">
          <span className="key">runs</span>
          <code>systemctl {command}</code>
        </p>
        <div className="modal-actions">
          <button className="btn" onClick={onClose}>
            Cancel
          </button>
          <button className="btn solid" onClick={onConfirm} autoFocus>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
