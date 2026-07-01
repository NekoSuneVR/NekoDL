import { useState } from 'react'
import { Modal } from './Modal'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { setToken } from '../lib/api'

interface AuthPromptProps {
  open: boolean
  onClose: () => void
  onSaved: () => void
}

// The custom-modal equivalent of a login screen (TODO.md Phase 7's "Auth
// screen" item) — a full separate route isn't needed since there's nothing
// else to gate; this modal is the entire auth flow.
export function AuthPrompt({ open, onClose, onSaved }: AuthPromptProps) {
  const [value, setValue] = useState('')

  return (
    <Modal open={open} title="API token" onClose={onClose}>
      <p className="mb-4 text-sm text-text-muted">
        Enter the <code className="text-text-primary">api_token</code> configured on the NekoDL
        server. Leave blank if the server has no token set.
      </p>
      <Input
        type="password"
        placeholder="Bearer token"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        autoFocus
      />
      <div className="mt-6 flex justify-end gap-3">
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="primary"
          onClick={() => {
            setToken(value.trim())
            onSaved()
          }}
        >
          Save
        </Button>
      </div>
    </Modal>
  )
}
