export type TaskStatus =
  | 'pending'
  | 'active'
  | 'paused'
  | 'complete'
  | 'error'
  | 'cancelled'

export interface Progress {
  TotalBytes: number
  DownloadedBytes: number
  SpeedBytesPerS: number
}

export interface TaskRecord {
  id: string
  engine: string
  priority: number
  added_at: string
  status: TaskStatus
  progress: Progress
  error?: string
  warning?: string
}

export interface ApiError {
  error: string
}

export interface ServerSettings {
  allow_torrents: boolean
  require_proxy_for_torrents: boolean
}
