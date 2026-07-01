import { useState } from 'react'
import type { TaskRecord } from '../lib/types'
import { EngineBadge, StatusBadge } from './ui/Badge'
import { ProgressBar } from './ui/ProgressBar'
import { formatBytes, formatETA, formatSpeed } from '../lib/format'

interface TaskListProps {
  tasks: TaskRecord[]
  selected: Set<string>
  onSelectedChange: (selected: Set<string>) => void
}

export function TaskList({ tasks, selected, onSelectedChange }: TaskListProps) {
  const [expandedId, setExpandedId] = useState<string | null>(null)

  if (tasks.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-surface-border p-10 text-center text-text-muted">
        Nothing here — add a download with the "+ New" button above.
      </div>
    )
  }

  function toggleOne(id: string) {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onSelectedChange(next)
  }

  function toggleAll() {
    onSelectedChange(selected.size === tasks.length ? new Set() : new Set(tasks.map((t) => t.id)))
  }

  return (
    <div className="overflow-hidden rounded-xl border border-surface-border">
      <table className="w-full text-left text-sm">
        <thead className="bg-surface-800 text-text-muted">
          <tr>
            <th className="w-10 px-4 py-3">
              <input
                type="checkbox"
                checked={selected.size === tasks.length}
                onChange={toggleAll}
                className="h-4 w-4 rounded border-surface-border bg-surface-900 accent-brand-500"
              />
            </th>
            <th className="px-4 py-3">Task</th>
            <th className="px-4 py-3">Status</th>
            <th className="px-4 py-3">Progress</th>
            <th className="px-4 py-3">Remaining</th>
            <th className="px-4 py-3">Speed</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-surface-border">
          {tasks.map((task) => (
            <TaskRow
              key={task.id}
              task={task}
              checked={selected.has(task.id)}
              onToggle={() => toggleOne(task.id)}
              expanded={expandedId === task.id}
              onToggleExpand={() => setExpandedId(expandedId === task.id ? null : task.id)}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}

function TaskRow({
  task,
  checked,
  onToggle,
  expanded,
  onToggleExpand,
}: {
  task: TaskRecord
  checked: boolean
  onToggle: () => void
  expanded: boolean
  onToggleExpand: () => void
}) {
  return (
    <>
      <tr className="cursor-pointer hover:bg-surface-800/60" onClick={onToggleExpand}>
        <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}>
          <input
            type="checkbox"
            checked={checked}
            onChange={onToggle}
            className="h-4 w-4 rounded border-surface-border bg-surface-900 accent-brand-500"
          />
        </td>
        <td className="px-4 py-3">
          <div className="flex items-center gap-2">
            <EngineBadge engine={task.engine} />
            <span className="text-text-muted">{task.id}</span>
          </div>
          {task.error && <p className="mt-1 text-xs text-danger-500">{task.error}</p>}
          {task.warning && !task.error && (
            <p className="mt-1 text-xs text-warning-500">{task.warning}</p>
          )}
        </td>
        <td className="px-4 py-3">
          <StatusBadge status={task.status} />
        </td>
        <td className="px-4 py-3">
          <div className="w-40">
            <ProgressBar total={task.progress.TotalBytes} downloaded={task.progress.DownloadedBytes} />
            <p className="mt-1 text-xs text-text-muted">
              {formatBytes(task.progress.DownloadedBytes)}
              {task.progress.TotalBytes > 0 && ` / ${formatBytes(task.progress.TotalBytes)}`}
            </p>
          </div>
        </td>
        <td className="px-4 py-3 text-text-muted">
          {formatETA(task.progress.TotalBytes, task.progress.DownloadedBytes, task.progress.SpeedBytesPerS)}
        </td>
        <td className="px-4 py-3 text-text-muted">{formatSpeed(task.progress.SpeedBytesPerS)}</td>
      </tr>
      {expanded && (
        <tr className="bg-surface-800/40">
          <td colSpan={6} className="px-4 py-3">
            <dl className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
              <DetailField label="ID" value={task.id} />
              <DetailField label="Engine" value={task.engine} />
              <DetailField label="Priority" value={String(task.priority)} />
              <DetailField label="Added" value={new Date(task.added_at).toLocaleString()} />
            </dl>
            <p className="mt-2 text-xs text-text-muted">
              Per-file breakdown, torrent peer lists, and engine logs aren't available from the
              API yet — this panel shows everything the backend currently reports for a task.
            </p>
          </td>
        </tr>
      )}
    </>
  )
}

function DetailField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-text-muted">{label}</dt>
      <dd className="text-text-primary">{value}</dd>
    </div>
  )
}
