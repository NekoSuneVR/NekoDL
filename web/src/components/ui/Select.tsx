import type { SelectHTMLAttributes } from 'react'

export function Select({ className = '', ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={`w-full rounded-lg border border-surface-border bg-surface-900 px-3 py-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none ${className}`}
      {...props}
    />
  )
}
