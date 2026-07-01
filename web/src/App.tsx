import { useState } from 'react'
import { useToast } from './components/Toast'
import { Modal } from './components/Modal'

// Placeholder dashboard shell. Real task list / add-task flow lands once the
// core API (Phase 1) has more than a /health endpoint to talk to — see TODO.md.
function App() {
  const { showToast } = useToast()
  const [confirmOpen, setConfirmOpen] = useState(false)

  return (
    <div className="min-h-screen bg-surface-950">
      <header className="flex items-center justify-between border-b border-surface-border px-6 py-4">
        <h1 className="text-xl font-semibold text-brand-400">NekoDL</h1>
        <span className="text-sm text-text-muted">no tasks yet</span>
      </header>

      <main className="p-6">
        <div className="rounded-xl border border-dashed border-surface-border p-10 text-center text-text-muted">
          Task queue will render here once the core API is wired up.
        </div>

        {/* Temporary component smoke test — remove once real task actions exist. */}
        <div className="mt-6 flex gap-3">
          <button
            type="button"
            onClick={() => showToast('success', 'Download completed successfully.')}
            className="rounded-lg bg-brand-600 px-4 py-2 text-sm font-medium text-surface-950 hover:bg-brand-500"
          >
            Test success toast
          </button>
          <button
            type="button"
            onClick={() => showToast('error', 'Proxy connection dropped — torrent paused.')}
            className="rounded-lg border border-surface-border px-4 py-2 text-sm font-medium text-text-primary hover:bg-surface-700"
          >
            Test error toast
          </button>
          <button
            type="button"
            onClick={() => setConfirmOpen(true)}
            className="rounded-lg border border-surface-border px-4 py-2 text-sm font-medium text-text-primary hover:bg-surface-700"
          >
            Test confirm modal
          </button>
        </div>
      </main>

      <Modal open={confirmOpen} title="Remove task" onClose={() => setConfirmOpen(false)}>
        <p className="mb-6 text-sm text-text-muted">
          This is a placeholder confirmation dialog, standing in for real task actions.
        </p>
        <div className="flex justify-end gap-3">
          <button
            type="button"
            onClick={() => setConfirmOpen(false)}
            className="rounded-lg border border-surface-border px-4 py-2 text-sm text-text-primary hover:bg-surface-700"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => {
              setConfirmOpen(false)
              showToast('info', 'Task removed.')
            }}
            className="rounded-lg bg-danger-500 px-4 py-2 text-sm font-medium text-surface-950 hover:opacity-90"
          >
            Remove
          </button>
        </div>
      </Modal>
    </div>
  )
}

export default App
