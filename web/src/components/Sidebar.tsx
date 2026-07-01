import type { Category } from '../lib/categories'

interface SidebarProps {
  category: Category
  onCategoryChange: (category: Category) => void
  counts: Record<Category, number>
  connected: boolean
  onOpenAuth: () => void
}

const NAV_ITEMS: { category: Category; label: string; icon: string }[] = [
  { category: 'downloading', label: 'Downloading', icon: '⬇' },
  { category: 'waiting', label: 'Waiting', icon: '⏱' },
  { category: 'finished', label: 'Finished / Stopped', icon: '✓' },
]

export function Sidebar({ category, onCategoryChange, counts, connected, onOpenAuth }: SidebarProps) {
  return (
    <aside className="flex w-60 flex-shrink-0 flex-col border-r border-surface-border bg-surface-900">
      <div className="border-b border-surface-border px-5 py-4">
        <h1 className="text-lg font-semibold text-brand-400">NekoDL</h1>
      </div>

      <nav className="flex-1 overflow-y-auto py-3">
        <p className="px-5 pb-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
          Download
        </p>
        {NAV_ITEMS.map((item) => (
          <button
            key={item.category}
            type="button"
            onClick={() => onCategoryChange(item.category)}
            className={`flex w-full items-center justify-between px-5 py-2 text-sm transition-colors ${
              category === item.category
                ? 'bg-brand-600/15 text-brand-400'
                : 'text-text-primary hover:bg-surface-800'
            }`}
          >
            <span className="flex items-center gap-2">
              <span aria-hidden="true">{item.icon}</span>
              {item.label}
            </span>
            {counts[item.category] > 0 && (
              <span className="rounded-full bg-surface-700 px-2 py-0.5 text-xs text-text-muted">
                {counts[item.category]}
              </span>
            )}
          </button>
        ))}

        <p className="mt-4 px-5 pb-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
          Settings
        </p>
        <button
          type="button"
          onClick={onOpenAuth}
          className="flex w-full items-center gap-2 px-5 py-2 text-sm text-text-primary hover:bg-surface-800"
        >
          <span aria-hidden="true">🔑</span>
          API Token
        </button>
      </nav>

      <div className="border-t border-surface-border px-5 py-3">
        <div className="flex items-center justify-between text-xs text-text-muted">
          <span>NekoDL Core</span>
          <span
            className={`rounded-full px-2 py-0.5 font-medium ${
              connected ? 'bg-brand-600/20 text-brand-400' : 'bg-danger-500/20 text-danger-500'
            }`}
          >
            {connected ? 'Connected' : 'Connecting'}
          </span>
        </div>
      </div>
    </aside>
  )
}
