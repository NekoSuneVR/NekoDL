import { useEffect, useState } from 'react'
import { Modal } from './Modal'
import { Toggle } from './ui/Toggle'
import { getSettings, putSettings } from '../lib/api'
import type { ServerSettings } from '../lib/types'
import { useToast } from './Toast'

interface SettingsModalProps {
  open: boolean
  onClose: () => void
}

// Real server-enforced policy (core/internal/api/addtorrent.go rejects
// requests that violate these), not a cosmetic preference — see TODO.md
// Phase 3/7. Each toggle saves immediately; there's no separate "Save" step.
export function SettingsModal({ open, onClose }: SettingsModalProps) {
  const { showToast } = useToast()
  const [settings, setSettings] = useState<ServerSettings | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!open) return
    setLoading(true)
    getSettings()
      .then(setSettings)
      .catch((err: Error) => showToast('error', `Failed to load settings: ${err.message}`))
      .finally(() => setLoading(false))
  }, [open, showToast])

  async function update(patch: Partial<ServerSettings>) {
    if (!settings) return
    const next = { ...settings, ...patch }
    setSettings(next) // optimistic
    try {
      const saved = await putSettings(next)
      setSettings(saved)
      showToast('success', 'Settings saved.')
    } catch (err) {
      setSettings(settings) // revert on failure
      showToast('error', `Failed to save settings: ${(err as Error).message}`)
    }
  }

  return (
    <Modal open={open} title="Settings" onClose={onClose}>
      {loading || !settings ? (
        <p className="text-sm text-text-muted">Loading…</p>
      ) : (
        <div className="divide-y divide-surface-border">
          <Toggle
            label="Allow torrent downloads"
            description="When off, adding a new torrent is rejected outright."
            checked={settings.allow_torrents}
            onChange={(checked) => update({ allow_torrents: checked })}
          />
          <Toggle
            label="Require a proxy/VPN for torrents"
            description="When on, a torrent without a proxy configured is rejected instead of just warned about. Off by default — torrents run on your real IP with a warning unless you turn this on."
            checked={settings.require_proxy_for_torrents}
            onChange={(checked) => update({ require_proxy_for_torrents: checked })}
          />
        </div>
      )}
    </Modal>
  )
}
