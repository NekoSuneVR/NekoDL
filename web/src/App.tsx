import { useMemo, useState } from 'react'
import { useTasks } from './hooks/useTasks'
import { Sidebar } from './components/Sidebar'
import { Toolbar } from './components/Toolbar'
import { TaskList } from './components/TaskList'
import { StatusBar } from './components/StatusBar'
import { AddTaskModal } from './components/AddTaskModal'
import { AuthPrompt } from './components/AuthPrompt'
import { SettingsModal } from './components/SettingsModal'
import { Modal } from './components/Modal'
import { Button } from './components/ui/Button'
import { useToast } from './components/Toast'
import { cancelTask, pauseTask, removeTask, resumeTask } from './lib/api'
import { countByCategory, filterByCategory, type Category } from './lib/categories'

function App() {
  const { tasks, loading, error, connected, refresh } = useTasks()
  const { showToast } = useToast()

  const [category, setCategory] = useState<Category>('downloading')
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [addOpen, setAddOpen] = useState(false)
  const [authOpen, setAuthOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(false)

  const counts = useMemo(() => countByCategory(tasks), [tasks])
  const visibleTasks = useMemo(() => {
    const inCategory = filterByCategory(tasks, category)
    if (!search.trim()) return inCategory
    const q = search.trim().toLowerCase()
    return inCategory.filter((t) => t.id.toLowerCase().includes(q) || t.engine.toLowerCase().includes(q))
  }, [tasks, category, search])

  async function runOnSelected(action: (id: string) => Promise<void>, label: string) {
    const ids = [...selected]
    const results = await Promise.allSettled(ids.map(action))
    const failures = results.filter((r) => r.status === 'rejected').length
    if (failures > 0) {
      showToast('error', `Failed to ${label} ${failures} of ${ids.length} task(s).`)
    }
    setSelected(new Set())
    refresh()
  }

  return (
    <div className="flex h-screen flex-col overflow-hidden md:flex-row">
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/60 md:hidden"
          onClick={() => setSidebarOpen(false)}
          aria-hidden="true"
        />
      )}
      <div
        className={`fixed inset-y-0 left-0 z-50 transition-transform md:static md:translate-x-0 ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <Sidebar
          category={category}
          onCategoryChange={(c) => {
            setCategory(c)
            setSidebarOpen(false)
          }}
          counts={counts}
          connected={connected}
          onOpenAuth={() => setAuthOpen(true)}
          onOpenSettings={() => setSettingsOpen(true)}
        />
      </div>

      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="flex items-center gap-2 border-b border-surface-border bg-surface-900 px-2 py-1 md:hidden">
          <button
            type="button"
            onClick={() => setSidebarOpen(true)}
            className="rounded-md p-2 text-text-primary hover:bg-surface-700"
            aria-label="Open menu"
          >
            ☰
          </button>
          <span className="text-sm font-semibold text-brand-400">NekoDL</span>
        </div>

        <Toolbar
          onAdd={() => setAddOpen(true)}
          onResumeSelected={() => runOnSelected(resumeTask, 'resume')}
          onPauseSelected={() => runOnSelected(pauseTask, 'pause')}
          onDeleteSelected={() => setConfirmDeleteOpen(true)}
          selectedCount={selected.size}
          search={search}
          onSearchChange={setSearch}
        />

        <main className="flex-1 overflow-y-auto p-4">
          {error ? (
            <div className="rounded-xl border border-danger-500/40 bg-danger-500/10 p-6 text-danger-500">
              Failed to load tasks: {error}
              {error.toLowerCase().includes('token') && (
                <Button className="ml-4" onClick={() => setAuthOpen(true)}>
                  Set API token
                </Button>
              )}
            </div>
          ) : loading ? (
            <p className="text-text-muted">Loading tasks…</p>
          ) : (
            <TaskList tasks={visibleTasks} selected={selected} onSelectedChange={setSelected} />
          )}
        </main>

        <StatusBar tasks={tasks} connected={connected} />
      </div>

      <AddTaskModal open={addOpen} onClose={() => setAddOpen(false)} onAdded={refresh} />
      <SettingsModal open={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <AuthPrompt
        open={authOpen}
        onClose={() => setAuthOpen(false)}
        onSaved={() => {
          setAuthOpen(false)
          window.location.reload() // re-establish the WebSocket with the new token
        }}
      />
      <Modal
        open={confirmDeleteOpen}
        title="Remove selected tasks"
        onClose={() => setConfirmDeleteOpen(false)}
      >
        <p className="mb-6 text-sm text-text-muted">
          This stops {selected.size} task{selected.size === 1 ? '' : 's'} and removes their
          downloaded data. This can't be undone.
        </p>
        <div className="flex justify-end gap-3">
          <Button onClick={() => setConfirmDeleteOpen(false)}>Cancel</Button>
          <Button
            variant="danger"
            onClick={() => {
              setConfirmDeleteOpen(false)
              runOnSelected(async (id) => {
                await cancelTask(id).catch(() => {})
                await removeTask(id)
              }, 'remove')
            }}
          >
            Remove
          </Button>
        </div>
      </Modal>
    </div>
  )
}

export default App
