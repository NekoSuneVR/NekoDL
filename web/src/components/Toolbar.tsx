import type { ReactNode } from 'react'
import { Input } from './ui/Input'

interface ToolbarProps {
  onAdd: () => void
  onResumeSelected: () => void
  onPauseSelected: () => void
  onDeleteSelected: () => void
  selectedCount: number
  search: string
  onSearchChange: (value: string) => void
}

export function Toolbar({
  onAdd,
  onResumeSelected,
  onPauseSelected,
  onDeleteSelected,
  selectedCount,
  search,
  onSearchChange,
}: ToolbarProps) {
  const hasSelection = selectedCount > 0

  return (
    <div className="flex items-center justify-between border-b border-surface-border bg-surface-900 px-4 py-2.5">
      <div className="flex items-center gap-1">
        <ToolbarButton onClick={onAdd} title="Add a new download">
          <span aria-hidden="true">＋</span> New
        </ToolbarButton>
        <ToolbarDivider />
        <ToolbarButton onClick={onResumeSelected} disabled={!hasSelection} title="Resume selected">
          ▶
        </ToolbarButton>
        <ToolbarButton onClick={onPauseSelected} disabled={!hasSelection} title="Pause selected">
          ⏸
        </ToolbarButton>
        <ToolbarButton onClick={onDeleteSelected} disabled={!hasSelection} title="Remove selected">
          🗑
        </ToolbarButton>
        {hasSelection && (
          <span className="ml-2 text-xs text-text-muted">{selectedCount} selected</span>
        )}
      </div>

      <div className="w-64">
        <Input
          type="search"
          placeholder="Search"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
        />
      </div>
    </div>
  )
}

function ToolbarButton({
  children,
  onClick,
  disabled,
  title,
}: {
  children: ReactNode
  onClick: () => void
  disabled?: boolean
  title: string
}) {
  return (
    <button
      type="button"
      title={title}
      onClick={onClick}
      disabled={disabled}
      className="rounded-md px-3 py-1.5 text-sm text-text-primary hover:bg-surface-700 disabled:cursor-not-allowed disabled:text-text-muted disabled:hover:bg-transparent"
    >
      {children}
    </button>
  )
}

function ToolbarDivider() {
  return <span className="mx-1 h-5 w-px bg-surface-border" aria-hidden="true" />
}
