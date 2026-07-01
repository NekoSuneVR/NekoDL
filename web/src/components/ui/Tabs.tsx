interface TabsProps {
  tabs: string[]
  active: string
  onChange: (tab: string) => void
}

export function Tabs({ tabs, active, onChange }: TabsProps) {
  return (
    <div className="mb-4 flex gap-4 border-b border-surface-border">
      {tabs.map((tab) => (
        <button
          key={tab}
          type="button"
          onClick={() => onChange(tab)}
          className={`-mb-px border-b-2 px-1 pb-2 text-sm font-medium transition-colors ${
            active === tab
              ? 'border-brand-500 text-brand-400'
              : 'border-transparent text-text-muted hover:text-text-primary'
          }`}
        >
          {tab}
        </button>
      ))}
    </div>
  )
}
