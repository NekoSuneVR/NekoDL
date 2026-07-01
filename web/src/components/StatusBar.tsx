import type { TaskRecord } from '../lib/types'
import { formatSpeed } from '../lib/format'

interface StatusBarProps {
  tasks: TaskRecord[]
  connected: boolean
}

export function StatusBar({ tasks, connected }: StatusBarProps) {
  const totalDownloadSpeed = tasks.reduce((sum, t) => sum + t.progress.SpeedBytesPerS, 0)
  const activeCount = tasks.filter((t) => t.status === 'active').length

  return (
    <footer className="flex items-center justify-between border-t border-surface-border bg-surface-900 px-4 py-2 text-xs text-text-muted">
      <span>
        {activeCount} active · {tasks.length} total
      </span>
      <div className="flex items-center gap-4">
        <span className="flex items-center gap-1">
          <span aria-hidden="true">⬇</span> {formatSpeed(totalDownloadSpeed)}
        </span>
        <span className={connected ? 'text-brand-400' : 'text-danger-500'}>
          {connected ? '● live' : '● disconnected'}
        </span>
      </div>
    </footer>
  )
}
