import { useEffect, useRef, useState } from 'react'
import { eventsURL, listTasks } from '../lib/api'
import type { TaskRecord } from '../lib/types'

interface UseTasksResult {
  tasks: TaskRecord[]
  loading: boolean
  error: string | null
  connected: boolean
  refresh: () => void
}

// Loads the task list once, then keeps it live via the WebSocket events
// feed (core/internal/api/websocket.go pushes the full list once a second).
// No polling — matches TODO.md Phase 7's "Live updates via WebSocket" item.
export function useTasks(): UseTasksResult {
  const [tasks, setTasks] = useState<TaskRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [connected, setConnected] = useState(false)
  const [refreshNonce, setRefreshNonce] = useState(0)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listTasks()
      .then((result) => {
        if (!cancelled) {
          setTasks(result)
          setError(null)
        }
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [refreshNonce])

  useEffect(() => {
    const ws = new WebSocket(eventsURL())
    wsRef.current = ws

    ws.onopen = () => setConnected(true)
    ws.onclose = () => setConnected(false)
    ws.onerror = () => setConnected(false)
    ws.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data) as TaskRecord[]
        setTasks(parsed)
      } catch {
        // ignore a malformed frame — the next one a second later will correct it
      }
    }

    return () => {
      ws.close()
      wsRef.current = null
    }
  }, [])

  return {
    tasks,
    loading,
    error,
    connected,
    refresh: () => setRefreshNonce((n) => n + 1),
  }
}
