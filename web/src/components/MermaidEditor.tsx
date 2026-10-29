// A text diagram: the Mermaid source on the left, the drawing on the right (tabs on a phone), AI to change or fix it.
import { useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import { mermaidError, mermaidPicture, renderMermaid, TEMPLATES } from '../lib/draw'
import { Icon } from './icons'
import { type MenuItem, MenuButton, Segmented, Spinner, useIsMobile, useToast } from './ui'
import type { EditorProps } from './SketchEditor'

export default function MermaidEditor({ data, dark, settings, onChange }: EditorProps) {
  const mobile = useIsMobile()
  const { toast } = useToast()
  const [code, setCode] = useState(data)
  const [svg, setSvg] = useState('')
  const [error, setError] = useState('')
  const [drawing, setDrawing] = useState(true)
  const [zoom, setZoom] = useState(1)
  const [pane, setPane] = useState<'code' | 'view'>('view')
  const [ask, setAsk] = useState('')
  const [asking, setAsking] = useState(false)
  const [busyAI, setBusyAI] = useState<'' | 'change' | 'fix'>('')
  const [undo, setUndo] = useState<string | null>(null)
  const area = useRef<HTMLTextAreaElement>(null)
  const first = useRef(true)
  const look = { theme: settings.mermaid_theme, look: settings.mermaid_look }

  // draw (a moment after typing stops), and save what was typed
  useEffect(() => {
    let stop = false
    setDrawing(true)
    const t = window.setTimeout(() => {
      renderMermaid(code, look).then(s => { if (!stop) { setSvg(s); setError('') } })
        .catch(e => { if (!stop) setError(mermaidError(e)) })
        .finally(() => { if (!stop) setDrawing(false) })
    }, first.current ? 0 : 350)
    if (!first.current) {
      const now = code
      onChange(() => now, () => mermaidPicture(now, look))
    }
    first.current = false
    return () => { stop = true; window.clearTimeout(t) }
  }, [code, dark, settings.mermaid_theme, settings.mermaid_look]) // eslint-disable-line react-hooks/exhaustive-deps

  const edit = (v: string) => { setCode(v); setUndo(null) }
  const tab = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key !== 'Tab') return
    e.preventDefault()
    const el = e.currentTarget
    const { selectionStart: a, selectionEnd: b, value } = el
    const next = value.slice(0, a) + '    ' + value.slice(b)
    edit(next)
    requestAnimationFrame(() => { el.selectionStart = el.selectionEnd = a + 4 })
  }

  const change = async () => {
    if (!ask.trim()) return
    setBusyAI('change')
    try {
      const r = await api.post<{ ok: boolean; code?: string; error?: string }>(`${V1}/ai/diagram`, { prompt: ask, code })
      if (!r.ok || !r.code) { toast(r.error || 'The AI did not answer', 'err'); return }
      const before = code
      setCode(r.code); setUndo(before); setAsk(''); setAsking(false)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusyAI('') }
  }
  const fix = async () => {
    setBusyAI('fix')
    try {
      const r = await api.post<{ ok: boolean; code?: string; change?: string; error?: string }>(`${V1}/ai/fix`, { code, error })
      if (!r.ok || !r.code) { toast(r.error || 'The AI did not answer', 'err'); return }
      const before = code
      setCode(r.code); setUndo(before)
      if (r.change) toast(r.change)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusyAI('') }
  }
  const templates = (): MenuItem[] => [
    { heading: 'Start again from' },
    ...TEMPLATES.map(t => ({ label: t.label, icon: t.icon, onSelect: () => { const before = code; setCode(t.code); setUndo(before) } })),
  ]

  const codePane = (
    <div className="mm-code">
      <div className="mm-bar">
        <MenuButton label="Templates" className="btn ghost sm" width={220} items={templates}><Icon name="layers" size={15} />Templates</MenuButton>
        <button type="button" className={`btn ghost sm ${asking ? 'active' : ''}`} onClick={() => setAsking(a => !a)}><Icon name="sparkles" size={15} />Change with AI</button>
        <span className="spacer" />
        {undo !== null && <button type="button" className="btn ghost sm" onClick={() => { setCode(undo); setUndo(null) }}><Icon name="undo" size={15} />Undo</button>}
        <a className="btn ghost icon-only sm" href="https://mermaid.js.org/intro/syntax-reference.html" target="_blank" rel="noreferrer" title="How to write diagrams (Mermaid)" aria-label="Help"><Icon name="help" size={15} /></a>
      </div>
      {asking && <div className="mm-ask">
        <input autoFocus value={ask} onChange={e => setAsk(e.target.value)} placeholder="e.g. add a step for paying, make it left to right" disabled={!!busyAI}
          onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); void change() } if (e.key === 'Escape') setAsking(false) }} />
        <button type="button" className="btn primary sm" onClick={change} disabled={!ask.trim() || !!busyAI}>{busyAI === 'change' ? <Spinner /> : 'Change'}</button>
      </div>}
      <textarea ref={area} className="mm-text" value={code} onChange={e => edit(e.target.value)} onKeyDown={tab} spellCheck={false} autoCapitalize="off" autoCorrect="off" aria-label="Diagram as text" />
      <div className={`mm-status ${error ? 'bad' : ''}`}>
        {error ? <>
          <Icon name="alert" size={14} /><span className="grow mm-err" title={error}>{error}</span>
          <button type="button" className="btn sm" onClick={fix} disabled={!!busyAI}>{busyAI === 'fix' ? <Spinner /> : <Icon name="sparkles" size={14} />}Fix with AI</button>
        </> : <span className="muted small">{code.split('\n').length} lines · the drawing follows as you type</span>}
      </div>
    </div>
  )
  const viewPane = (
    <div className="mm-view">
      <div className="mm-zoom">
        <button type="button" className="btn ghost icon-only sm" onClick={() => setZoom(z => Math.max(0.25, +(z - 0.25).toFixed(2)))} aria-label="Smaller"><Icon name="minus" size={15} /></button>
        <button type="button" className="btn ghost sm tnum mm-zoom-n" onClick={() => setZoom(1)} title="Fit">{Math.round(zoom * 100)}%</button>
        <button type="button" className="btn ghost icon-only sm" onClick={() => setZoom(z => Math.min(4, +(z + 0.25).toFixed(2)))} aria-label="Bigger"><Icon name="plus" size={15} /></button>
      </div>
      <div className={`mm-canvas ${error ? 'stale' : ''}`}>
        {svg ? <div className="mm-svg" style={{ zoom }} dangerouslySetInnerHTML={{ __html: svg }} /> : drawing ? <div className="loading"><Spinner /></div> : null}
      </div>
    </div>
  )
  if (mobile) return (
    <div className="mm-host mobile">
      <div className="mm-tabs"><Segmented size="sm" value={pane} onChange={setPane} options={[{ value: 'view', label: 'Drawing' }, { value: 'code', label: 'Text' }]} />{error && <span className="badge warn">has a mistake</span>}</div>
      {pane === 'code' ? codePane : viewPane}
    </div>
  )
  return <div className="mm-host">{codePane}{viewPane}</div>
}
