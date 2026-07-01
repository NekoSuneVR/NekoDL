import { useState, type ReactNode } from 'react'
import { Modal } from './Modal'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { Tabs } from './ui/Tabs'
import { addTask, addTorrent } from '../lib/api'
import { useToast } from './Toast'

interface AddTaskModalProps {
  open: boolean
  onClose: () => void
  onAdded: () => void
}

// One line is either a magnet URI (routed to POST /api/v1/torrents) or a
// plain URL (routed to POST /api/v1/tasks, which itself resolves
// Dropbox/Pixeldrain/Google Drive/Mediafire/mega.nz links server-side).
function classifyLine(line: string): 'magnet' | 'url' | null {
  const trimmed = line.trim()
  if (!trimmed) return null
  if (trimmed.startsWith('magnet:')) return 'magnet'
  return 'url'
}

export function AddTaskModal({ open, onClose, onAdded }: AddTaskModalProps) {
  const { showToast } = useToast()
  const [tab, setTab] = useState('Links')
  const [links, setLinks] = useState('')
  const [torrentFile, setTorrentFile] = useState<File | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // Options tab — applies to every link submitted in this batch.
  const [priority, setPriority] = useState(0)
  const [maxConnections, setMaxConnections] = useState(4)
  const [proxyAddr, setProxyAddr] = useState('')
  const [seed, setSeed] = useState(false)
  const [maxDownloadBps, setMaxDownloadBps] = useState(0)
  const [maxUploadBps, setMaxUploadBps] = useState(0)

  function reset() {
    setLinks('')
    setTorrentFile(null)
    setTab('Links')
  }

  async function handleSubmit() {
    const lines = links.split('\n').map((l) => l.trim()).filter(Boolean)
    if (lines.length === 0 && !torrentFile) {
      showToast('error', 'Paste at least one link/magnet URI or choose a .torrent file.')
      return
    }

    setSubmitting(true)
    let succeeded = 0
    let failed = 0
    let lastWarning: string | undefined

    async function submitOne(fn: () => Promise<{ id: string; warning?: string }>) {
      try {
        const result = await fn()
        succeeded++
        if (result.warning) lastWarning = result.warning
      } catch (err) {
        failed++
        showToast('error', (err as Error).message)
      }
    }

    for (const line of lines) {
      const kind = classifyLine(line)
      if (kind === 'magnet') {
        await submitOne(() =>
          addTorrent({ magnet_uri: line, proxy_addr: proxyAddr || undefined, seed, max_download_bps: maxDownloadBps || undefined, max_upload_bps: maxUploadBps || undefined, priority }),
        )
      } else if (kind === 'url') {
        await submitOne(() => addTask({ url: line, max_connections: maxConnections, priority }))
      }
    }

    if (torrentFile) {
      const torrentFileBase64 = await fileToBase64(torrentFile)
      await submitOne(() =>
        addTorrent({ torrent_file_base64: torrentFileBase64, proxy_addr: proxyAddr || undefined, seed, max_download_bps: maxDownloadBps || undefined, max_upload_bps: maxUploadBps || undefined, priority }),
      )
    }

    setSubmitting(false)
    if (succeeded > 0) {
      showToast('success', `Added ${succeeded} download${succeeded === 1 ? '' : 's'}.`)
      if (lastWarning) showToast('warning', lastWarning)
      reset()
      onAdded()
      onClose()
    } else if (failed === 0) {
      showToast('error', 'Nothing to add.')
    }
  }

  return (
    <Modal open={open} title="Add download" onClose={onClose}>
      <Tabs tabs={['Links', 'Options']} active={tab} onChange={setTab} />

      {tab === 'Links' ? (
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs text-text-muted" htmlFor="add-task-links">
              HTTP links, one-click-hoster share links, or magnet URIs — one per line
            </label>
            <textarea
              id="add-task-links"
              rows={5}
              placeholder={'https://...\nmagnet:?xt=urn:btih:...'}
              value={links}
              onChange={(e) => setLinks(e.target.value)}
              className="w-full resize-y rounded-lg border border-surface-border bg-surface-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
              autoFocus
            />
          </div>
          <p className="text-center text-xs text-text-muted">— or —</p>
          <div>
            <label className="mb-1 block text-xs text-text-muted" htmlFor="add-task-file">
              Upload a .torrent file
            </label>
            <input
              id="add-task-file"
              type="file"
              accept=".torrent"
              onChange={(e) => setTorrentFile(e.target.files?.[0] ?? null)}
              className="block w-full text-sm text-text-muted"
            />
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          <fieldset className="space-y-3">
            <legend className="mb-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
              All downloads
            </legend>
            <LabeledField label="Priority (higher runs first)">
              <Input
                type="number"
                value={priority}
                onChange={(e) => setPriority(Number(e.target.value))}
              />
            </LabeledField>
          </fieldset>

          <fieldset className="space-y-3">
            <legend className="mb-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
              HTTP / one-click-hoster links
            </legend>
            <LabeledField label="Max connections (segments)">
              <Input
                type="number"
                min={1}
                value={maxConnections}
                onChange={(e) => setMaxConnections(Number(e.target.value))}
              />
            </LabeledField>
          </fieldset>

          <fieldset className="space-y-3">
            <legend className="mb-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
              Torrents / magnet links
            </legend>
            <LabeledField label="SOCKS5 proxy (host:port) — leave blank to use your real IP">
              <Input
                placeholder="127.0.0.1:9050"
                value={proxyAddr}
                onChange={(e) => setProxyAddr(e.target.value)}
              />
            </LabeledField>
            <LabeledField label="Max download (bytes/sec, 0 = unlimited)">
              <Input
                type="number"
                min={0}
                value={maxDownloadBps}
                onChange={(e) => setMaxDownloadBps(Number(e.target.value))}
              />
            </LabeledField>
            <LabeledField label="Max upload (bytes/sec, 0 = unlimited)">
              <Input
                type="number"
                min={0}
                value={maxUploadBps}
                onChange={(e) => setMaxUploadBps(Number(e.target.value))}
              />
            </LabeledField>
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={seed}
                onChange={(e) => setSeed(e.target.checked)}
                className="h-4 w-4 rounded border-surface-border bg-surface-900 accent-brand-500"
              />
              Continue seeding after download completes
            </label>
            {!proxyAddr && (
              <p className="text-xs text-warning-500">
                No proxy set — torrents added this way will use your real IP.
              </p>
            )}
          </fieldset>
        </div>
      )}

      <div className="mt-6 flex justify-end gap-3">
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" onClick={handleSubmit} disabled={submitting}>
          {submitting ? 'Adding…' : 'Download Now'}
        </Button>
      </div>
    </Modal>
  )
}

function LabeledField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <label className="mb-1 block text-xs text-text-muted">{label}</label>
      {children}
    </div>
  )
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      resolve(result.split(',', 2)[1] ?? '')
    }
    reader.onerror = () => reject(reader.error ?? new Error('failed to read file'))
    reader.readAsDataURL(file)
  })
}
