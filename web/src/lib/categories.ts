import type { TaskRecord, TaskStatus } from './types'

export type Category = 'downloading' | 'waiting' | 'finished'

const CATEGORY_STATUSES: Record<Category, TaskStatus[]> = {
  downloading: ['active'],
  waiting: ['pending', 'paused'],
  finished: ['complete', 'error', 'cancelled'],
}

export function categoryOf(status: TaskStatus): Category {
  for (const [category, statuses] of Object.entries(CATEGORY_STATUSES) as [Category, TaskStatus[]][]) {
    if (statuses.includes(status)) return category
  }
  return 'finished'
}

export function filterByCategory(tasks: TaskRecord[], category: Category): TaskRecord[] {
  return tasks.filter((t) => categoryOf(t.status) === category)
}

export function countByCategory(tasks: TaskRecord[]): Record<Category, number> {
  const counts: Record<Category, number> = { downloading: 0, waiting: 0, finished: 0 }
  for (const t of tasks) counts[categoryOf(t.status)]++
  return counts
}
