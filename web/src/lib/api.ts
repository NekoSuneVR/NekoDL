import type { ServerSettings, TaskRecord } from './types'

// Same-origin in production (the Go server serves this SPA and the API
// together — see core/internal/api/static.go). VITE_API_BASE_URL lets
// `npm run dev` point at a NekoDL core running on a different port.
const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? ''

const TOKEN_STORAGE_KEY = 'nekodl_api_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_STORAGE_KEY) ?? ''
}

export function setToken(token: string): void {
  if (token) {
    localStorage.setItem(TOKEN_STORAGE_KEY, token)
  } else {
    localStorage.removeItem(TOKEN_STORAGE_KEY)
  }
}

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken()
  const headers = new Headers(init?.headers)
  headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const resp = await fetch(`${BASE_URL}${path}`, { ...init, headers })

  if (!resp.ok) {
    let message = `request failed with status ${resp.status}`
    try {
      const body = (await resp.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // response wasn't JSON — keep the generic message
    }
    throw new ApiError(message, resp.status)
  }

  if (resp.status === 204) return undefined as T
  return (await resp.json()) as T
}

export function listTasks(): Promise<TaskRecord[]> {
  return request<TaskRecord[]>('/api/v1/tasks')
}

export function getTask(id: string): Promise<TaskRecord> {
  return request<TaskRecord>(`/api/v1/tasks/${id}`)
}

export interface AddTaskInput {
  url: string
  max_connections?: number
  priority?: number
  checksum?: { algo: string; expected: string }
}

export function addTask(input: AddTaskInput): Promise<{ id: string; warning?: string }> {
  return request('/api/v1/tasks', { method: 'POST', body: JSON.stringify(input) })
}

export interface AddTorrentInput {
  magnet_uri?: string
  torrent_file_base64?: string
  proxy_addr?: string
  max_download_bps?: number
  max_upload_bps?: number
  seed?: boolean
  priority?: number
}

export function addTorrent(input: AddTorrentInput): Promise<{ id: string; warning?: string }> {
  return request('/api/v1/torrents', { method: 'POST', body: JSON.stringify(input) })
}

export interface AddYtdlpInput {
  url: string
  format?: string
  no_playlist?: boolean
  subtitles?: boolean
  output_template?: string
  proxy_addr?: string
  cookies_file_base64?: string
  priority?: number
}

export function addYtdlp(input: AddYtdlpInput): Promise<{ id: string; warning?: string }> {
  return request('/api/v1/ytdlp', { method: 'POST', body: JSON.stringify(input) })
}

export function pauseTask(id: string): Promise<void> {
  return request(`/api/v1/tasks/${id}/pause`, { method: 'POST' })
}

export function resumeTask(id: string): Promise<void> {
  return request(`/api/v1/tasks/${id}/resume`, { method: 'POST' })
}

export function cancelTask(id: string): Promise<void> {
  return request(`/api/v1/tasks/${id}/cancel`, { method: 'POST' })
}

export function removeTask(id: string): Promise<void> {
  return request(`/api/v1/tasks/${id}`, { method: 'DELETE' })
}

export function getSettings(): Promise<ServerSettings> {
  return request<ServerSettings>('/api/v1/settings')
}

export function putSettings(next: ServerSettings): Promise<ServerSettings> {
  return request<ServerSettings>('/api/v1/settings', { method: 'PUT', body: JSON.stringify(next) })
}

// eventsURL builds the live-updates WebSocket URL. The token travels as a
// query param here specifically because browsers' native WebSocket API
// can't set an Authorization header on the handshake request — the server
// accepts this fallback only on this one endpoint (see auth.go).
export function eventsURL(): string {
  const base = BASE_URL || window.location.origin
  const url = new URL('/api/v1/events', base)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = getToken()
  if (token) url.searchParams.set('token', token)
  return url.toString()
}
