// When there is no draw.io on this box: what the Diagram kind is, why it is greyed out, and how to switch it on.
import { Link } from 'react-router-dom'
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Settings } from '../lib/types'
import { Icon } from './icons'
import { Spinner, useToast } from './ui'

/** The free hosted draw.io editor; nothing to install. */
export const HOSTED_DRAWIO = 'https://embed.diagrams.net'

/** True when diagrams can be opened on this box. */
export const drawioReady = (s: Settings | null | undefined) => !!s?.drawio_url

export function DrawioHelp({ compact = false, onReady }: { compact?: boolean; onReady?: () => void }) {
  const { user, settings, setSettings } = useApp()
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const admin = user?.role === 'admin'
  const useHosted = async () => {
    setBusy(true)
    try {
      setSettings(await api.put<Settings>(`${V1}/settings`, { drawio_url: HOSTED_DRAWIO }))
      toast('Diagrams are on (draw.io from diagrams.net)')
      onReady?.()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  if (drawioReady(settings)) return null
  return (
    <div className={`dio-help ${compact ? 'compact' : ''}`} role="note">
      <div className="dio-help-head"><Icon name="info" size={16} /><b>Diagrams are not switched on yet</b></div>
      <p>Diagrams open in <b>draw.io</b>, a free editor for detailed flowcharts, floor plans, network maps and org charts, with large shape libraries. It is not part of this app, so it needs an address first. Sketches and text diagrams work without it.</p>
      {admin ? <>
        <div className="row wrap">
          <button type="button" className="btn primary sm" onClick={useHosted} disabled={busy}>{busy ? <Spinner /> : <Icon name="check" size={14} />}Switch on with diagrams.net</button>
          <Link to="/settings/general" className="btn sm">Use my own draw.io…</Link>
        </div>
        {!compact && <p className="muted xsmall">With diagrams.net the editor loads from diagrams.net each time; your diagrams are kept here, in Pocket Draw. To keep everything on your server, run the free <code>jgraph/drawio</code> container and put its address in Settings → General.</p>}
      </> : <p className="muted small">An admin of this box can switch it on in Settings → General — it takes one click.</p>}
    </div>
  )
}
