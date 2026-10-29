// A draw.io diagram: the draw.io editor (the box's own server) in embed mode; it autosaves through postMessage.
import { useEffect, useRef, useState } from 'react'
import { dataUrlText } from '../lib/draw'
import { Icon } from './icons'
import { Spinner } from './ui'
import type { EditorProps } from './SketchEditor'

export default function DrawioEditor({ data, title, dark, settings, onChange }: EditorProps) {
  const frame = useRef<HTMLIFrameElement>(null)
  const last = useRef(data)
  const pending = useRef<((svg: string) => void)[]>([])
  const [ready, setReady] = useState(false)
  const base = (settings.drawio_url || '').replace(/\/+$/, '')
  const origin = (() => { try { return new URL(base).origin } catch { return '' } })()

  const post = (msg: Record<string, unknown>) => frame.current?.contentWindow?.postMessage(JSON.stringify(msg), origin || '*')

  useEffect(() => {
    if (!origin) return
    const h = (e: MessageEvent) => {
      if (e.origin !== origin || e.source !== frame.current?.contentWindow || typeof e.data !== 'string') return
      let msg: { event?: string; xml?: string; data?: string }
      try { msg = JSON.parse(e.data) } catch { return }
      switch (msg.event) {
        case 'init':
          post({ action: 'load', xml: last.current || '', autosave: 1, title })
          setReady(true)
          break
        case 'autosave':
        case 'save': {
          const xml = msg.xml || ''
          if (!xml || xml === last.current) return
          last.current = xml
          onChange(() => xml, () => new Promise<string>((resolve, reject) => {
            pending.current.push(resolve)
            post({ action: 'export', format: 'svg', background: '#ffffff', spinKey: '' })
            window.setTimeout(() => reject(new Error('no picture from draw.io')), 15000)
          }))
          break
        }
        case 'export': {
          const svg = msg.data ? dataUrlText(msg.data) : ''
          const waiting = pending.current
          pending.current = []
          waiting.forEach(r => r(svg))
          break
        }
      }
    }
    window.addEventListener('message', h)
    return () => window.removeEventListener('message', h)
  }, [origin, title, onChange]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!origin) return (
    <div className="dio-missing">
      <Icon name="shapes" size={28} />
      <h3>There is no draw.io server yet</h3>
      <p className="muted">Diagrams open in draw.io. An admin adds its address under Settings → General (the <code>jgraph/drawio</code> image on this box, or https://embed.diagrams.net). The diagram itself is kept here either way.</p>
    </div>
  )
  const q = new URLSearchParams({ embed: '1', proto: 'json', spin: '1', libraries: '1', noSaveBtn: '1', noExitBtn: '1', saveAndExit: '0', ui: 'min', dark: dark ? '1' : '0', lang: 'en' })
  return (
    <div className="dio-host">
      {!ready && <div className="dio-wait"><Spinner size={18} /><span className="muted small">Opening draw.io…</span></div>}
      <iframe ref={frame} title="draw.io" src={`${base}/?${q}`} className={ready ? 'ready' : ''} />
    </div>
  )
}
