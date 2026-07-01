interface ProgressBarProps {
  total: number
  downloaded: number
}

export function ProgressBar({ total, downloaded }: ProgressBarProps) {
  const pct = total > 0 ? Math.min(100, (downloaded / total) * 100) : 0
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-surface-700">
      <div
        className="h-full rounded-full bg-brand-500 transition-all"
        style={{ width: `${pct}%` }}
        role="progressbar"
        aria-valuenow={Math.round(pct)}
        aria-valuemin={0}
        aria-valuemax={100}
      />
    </div>
  )
}
