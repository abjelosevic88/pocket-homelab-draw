// "New drawing": choose what kind, start from a template, or describe a diagram and let AI draw it.
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announce, KINDS, KIND_ORDER, TEMPLATES } from '../lib/draw'
import type { Drawing, Kind } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { CoreHint, Field, Modal, Spinner, useToast } from './ui'
import { useDraw } from './Layout'

export function NewDrawing({ kind: initial, folderId, onClose }: { kind: Kind | 'ask'; folderId: number | null; onClose: () => void }) {
  const { settings } = useApp()
  const { folders } = useDraw()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [kind, setKind] = useState<Kind>(initial === 'ask' ? 'sketch' : initial)
  const [title, setTitle] = useState('')
  const [folder, setFolder] = useState(folderId ? String(folderId) : '')
  const [template, setTemplate] = useState('flowchart')
  const [prompt, setPrompt] = useState('')
  const [busy, setBusy] = useState<'' | 'create' | 'ai'>('')
  const [ai, setAI] = useState<{ ready: boolean; detail: string } | null>(null)
  const drawio = !!settings?.drawio_url
  const coreOn = !!settings?.core_configured && settings?.core_registered !== false

  useEffect(() => { if (kind === 'mermaid' && !ai) api.get<{ ready: boolean; detail: string }>(`${V1}/ai/status`).then(setAI).catch(() => setAI({ ready: false, detail: '' })) }, [kind, ai])

  const create = async (data?: string, t?: string) => {
    setBusy('create')
    try {
      const body: Record<string, unknown> = { kind, title: (t || title).trim(), folder_id: folder ? Number(folder) : null }
      if (kind === 'mermaid') body.data = data ?? TEMPLATES.find(x => x.id === template)?.code ?? ''
      const d = await api.post<Drawing>(`${V1}/drawings`, body)
      announce()
      onClose()
      navigate(`/d/${d.id}`)
    } catch (e) { toast((e as Error).message, 'err'); setBusy('') }
  }
  const draft = async () => {
    if (!prompt.trim()) return
    setBusy('ai')
    try {
      const r = await api.post<{ ok: boolean; code?: string; title?: string; error?: string }>(`${V1}/ai/diagram`, { prompt })
      if (!r.ok || !r.code) { toast(r.error || 'The AI did not answer', 'err'); setBusy(''); return }
      await create(r.code, title.trim() || r.title || '')
    } catch (e) { toast((e as Error).message, 'err'); setBusy('') }
  }

  return (
    <Modal title="New drawing" onClose={onClose} size="lg" className="new-dlg">
      <div className="kind-cards" role="radiogroup" aria-label="Kind of drawing">
        {KIND_ORDER.map(k => {
          const off = k === 'drawio' && !drawio
          return (
            <button key={k} type="button" role="radio" aria-checked={kind === k} disabled={off} className={`kind-card k-${k} ${kind === k ? 'on' : ''}`} onClick={() => setKind(k)}
              onDoubleClick={() => { if (!off) { setKind(k); if (k !== 'mermaid') void create() } }}
              title={off ? 'Needs a draw.io server: an admin adds its address in Settings → General' : undefined}>
              <span className="kind-ico"><Icon name={KINDS[k].icon} size={22} /></span>
              <span className="kind-name">{KINDS[k].label}</span>
              <span className="kind-blurb">{off ? 'Needs a draw.io server — an admin adds its address in Settings.' : KINDS[k].blurb}</span>
            </button>
          )
        })}
      </div>

      {kind === 'mermaid' && <div className="new-mermaid">
        <div className="tpl-grid" role="radiogroup" aria-label="Start from">
          {TEMPLATES.map(t => (
            <button key={t.id} type="button" role="radio" aria-checked={template === t.id} className={`chip tpl ${template === t.id && !prompt.trim() ? 'on' : ''}`} onClick={() => { setTemplate(t.id); setPrompt('') }}>
              <Icon name={t.icon} size={14} />{t.label}
            </button>
          ))}
        </div>
        <Field label={<span className="row" style={{ gap: 6 }}><Icon name="sparkles" size={14} />Or describe it — AI draws it</span>}
          help={ai && !ai.ready ? <>No AI is connected{ai.detail ? ` (${ai.detail})` : ''} — Settings → AI.{!coreOn && <CoreHint />}</> : 'For example: “how a pizza order goes from the phone call to the door”, or “my family: grandparents, parents, us and the kids”.'}>
          <textarea rows={3} value={prompt} onChange={e => setPrompt(e.target.value)} disabled={!!ai && !ai.ready} placeholder="What should the diagram show?" />
        </Field>
      </div>}

      <div className="two-col tight mt">
        <Field label="Name (optional)"><input value={title} onChange={e => setTitle(e.target.value)} placeholder={`Untitled ${KINDS[kind].label.toLowerCase()}`} maxLength={200}
          onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); if (kind === 'mermaid' && prompt.trim()) void draft(); else void create() } }} /></Field>
        <Field label="Folder"><Select ariaLabel="Folder" value={folder} onChange={setFolder} options={[{ value: '', label: 'No folder' }, ...folders.map(f => ({ value: String(f.id), label: f.name }))]} /></Field>
      </div>

      <div className="form-actions">
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        {kind === 'mermaid' && prompt.trim()
          ? <button type="button" className="btn primary" onClick={draft} disabled={!!busy}>{busy ? <Spinner /> : <Icon name="sparkles" size={15} />}{busy === 'ai' ? 'Drawing…' : 'Draw it'}</button>
          : <button type="button" className="btn primary" onClick={() => create()} disabled={!!busy}>{busy ? <Spinner /> : <Icon name="plus" size={15} />}Create</button>}
      </div>
    </Modal>
  )
}
