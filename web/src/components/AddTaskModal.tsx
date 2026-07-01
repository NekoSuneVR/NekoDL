import { useState, type ReactNode } from 'react'
import { Modal } from './Modal'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { Tabs } from './ui/Tabs'
import { addTask, addTorrent, addYtdlp } from '../lib/api'
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
  const [useYtdlp, setUseYtdlp] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // Options tab — applies to every link submitted in this batch.
  const [priority, setPriority] = useState(0)
  const [maxConnections, setMaxConnections] = useState(4)
  const [proxyAddr, setProxyAddr] = useState('')
  const [seed, setSeed] = useState(false)
  const [maxDownloadBps, setMaxDownloadBps] = useState(0)
  const [maxUploadBps, setMaxUploadBps] = useState(0)

  // yt-dlp-specific options — only sent when "Download with yt-dlp" is on.
  const [ytdlpFormat, setYtdlpFormat] = useState('')
  const [ytdlpNoPlaylist, setYtdlpNoPlaylist] = useState(false)
  const [ytdlpSubtitles, setYtdlpSubtitles] = useState(false)
  const [ytdlpOutputTemplate, setYtdlpOutputTemplate] = useState('')
  const [ytdlpProxyAddr, setYtdlpProxyAddr] = useState('')
  const [ytdlpCookiesFile, setYtdlpCookiesFile] = useState<File | null>(null)

  function reset() {
    setLinks('')
    setTorrentFile(null)
    setYtdlpCookiesFile(null)
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

    const ytdlpCookiesFileBase64 = ytdlpCookiesFile ? await fileToBase64(ytdlpCookiesFile) : undefined

    for (const line of lines) {
      const kind = classifyLine(line)
      if (kind === 'magnet') {
        await submitOne(() =>
          addTorrent({ magnet_uri: line, proxy_addr: proxyAddr || undefined, seed, max_download_bps: maxDownloadBps || undefined, max_upload_bps: maxUploadBps || undefined, priority }),
        )
      } else if (kind === 'url' && useYtdlp) {
        await submitOne(() =>
          addYtdlp({
            url: line,
            format: ytdlpFormat || undefined,
            no_playlist: ytdlpNoPlaylist,
            subtitles: ytdlpSubtitles,
            output_template: ytdlpOutputTemplate || undefined,
            proxy_addr: ytdlpProxyAddr || undefined,
            cookies_file_base64: ytdlpCookiesFileBase64,
            priority,
          }),
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
          <label className="flex items-center gap-2 text-sm text-text-primary">
            <input
              type="checkbox"
              checked={useYtdlp}
              onChange={(e) => setUseYtdlp(e.target.checked)}
              className="h-4 w-4 rounded border-surface-border bg-surface-900 accent-brand-500"
            />
            Download with yt-dlp (YouTube and 1000+ other sites)
          </label>
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
              yt-dlp (only applies when "Download with yt-dlp" is checked on the Links tab)
            </legend>
            <LabeledField label='Format (yt-dlp -f syntax, e.g. "best", blank = yt-dlp default)'>
              <Input
                placeholder="best"
                value={ytdlpFormat}
                onChange={(e) => setYtdlpFormat(e.target.value)}
              />
            </LabeledField>
            <LabeledField label="Output filename template (blank = yt-dlp default)">
              <Input
                placeholder="%(title)s.%(ext)s"
                value={ytdlpOutputTemplate}
                onChange={(e) => setYtdlpOutputTemplate(e.target.value)}
              />
            </LabeledField>
            <LabeledField label="Proxy for this download (host:port, optional)">
              <Input
                placeholder="127.0.0.1:9050"
                value={ytdlpProxyAddr}
                onChange={(e) => setYtdlpProxyAddr(e.target.value)}
              />
            </LabeledField>
            <LabeledField label="Cookies file (Netscape cookies.txt, optional — for sites requiring login)">
              <input
                type="file"
                accept=".txt"
                onChange={(e) => setYtdlpCookiesFile(e.target.files?.[0] ?? null)}
                className="block w-full text-sm text-text-muted"
              />
            </LabeledField>
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={ytdlpNoPlaylist}
                onChange={(e) => setYtdlpNoPlaylist(e.target.checked)}
                className="h-4 w-4 rounded border-surface-border bg-surface-900 accent-brand-500"
              />
              Single video only (ignore playlist if the URL is part of one)
            </label>
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={ytdlpSubtitles}
                onChange={(e) => setYtdlpSubtitles(e.target.checked)}
                className="h-4 w-4 rounded border-surface-border bg-surface-900 accent-brand-500"
              />
              Download subtitles
            </label>
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
