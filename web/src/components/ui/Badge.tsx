import type { TaskStatus } from '../../lib/types'

const STATUS_CLASSES: Record<TaskStatus, string> = {
  pending: 'bg-surface-700 text-text-muted',
  active: 'bg-brand-600/20 text-brand-400',
  paused: 'bg-warning-500/20 text-warning-500',
  complete: 'bg-brand-600/20 text-brand-400',
  error: 'bg-danger-500/20 text-danger-500',
  cancelled: 'bg-surface-700 text-text-muted',
}

export function StatusBadge({ status }: { status: TaskStatus }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium capitalize ${STATUS_CLASSES[status]}`}
    >
      {status}
    </span>
  )
}

export function EngineBadge({ engine }: { engine: string }) {
  const icon = ENGINE_ICONS[engine] ?? '📄'
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-surface-border px-2.5 py-0.5 text-xs text-text-muted">
      <span aria-hidden="true">{icon}</span>
      {engine}
    </span>
  )
}

const ENGINE_ICONS: Record<string, string> = {
  http: '🌐',
  torrent: '🧲',
  mega: '🔒',
}
