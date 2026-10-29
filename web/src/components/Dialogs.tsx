// Small dialogs: import files, a folder's name, rename, move to a folder.
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announce } from '../lib/draw'
import type { Drawing, Folder } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Modal, Spinner, useToast } from './ui'

interface ImportResult { kind: string; counts: Record<string, number>; drawings: number[] }

const KIND_LABEL: Record<string, string> = {
  excalidraw: 'Excalidraw sketch', excalidrawlib: 'Excalidraw library', drawio: 'draw.io diagram', mermaid: 'Mermaid diagram',
  markdown: 'Mermaid blocks from Markdown', 'mermaid-live': 'Mermaid Live link', 'pocket-draw': 'Pocket Draw export',
}

async function importOne(body: Blob | string, name: string, folderId: number | null): Promise<ImportResult> {
  const q = new URLSearchParams({ name })
  if (folderId) q.set('folder_id', String(folderId))
  const res = await fetch(`${V1}/import?${q}`, { method: 'POST', body, credentials: 'same-origin', headers: { 'Content-Type': 'application/octet-stream' } })
  const j = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(j.error || res.statusText)
  return j as ImportResult
}

/** Imports files (chosen, dropped or passed in) and Mermaid Live links. */
export function ImportDialog({ files: initial, folderId, onClose }: { files: File[]; folderId: number | null; onClose: () => void }) {
  const navigate = useNavigate()
  const [rows, setRows] = useState<{ name: string; state: 'wait' | 'ok' | 'err'; text: string; first?: number }[]>([])
  const [link, setLink] = useState('')
  const [over, setOver] = useState(false)
  const busy = rows.some(r => r.state === 'wait')
  const started = useRef(false)

  const run = async (items: { name: string; body: Blob | string }[]) => {
    const base = rows.length
    setRows(r => [...r, ...items.map(i => ({ name: i.name || 'link', state: 'wait' as const, text: 'Importing…' }))])
    for (let i = 0; i < items.length; i++) {
      const it = items[i]
      try {
        const r = await importOne(it.body, it.name, folderId)
        const parts = Object.entries(r.counts).filter(([, v]) => v).map(([k, v]) => `${v} ${k.replace('_', ' ')}`)
        setRows(rs => rs.map((x, j) => j === base + i ? { ...x, state: 'ok', text: `${KIND_LABEL[r.kind] || r.kind}: ${parts.join(', ') || 'nothing new'}`, first: r.drawings[0] } : x))
      } catch (e) {
        setRows(rs => rs.map((x, j) => j === base + i ? { ...x, state: 'err', text: (e as Error).message } : x))
      }
    }
    announce()
  }
  useEffect(() => { if (!started.current && initial.length) { started.current = true; void run(initial.map(f => ({ name: f.name, body: f }))) } }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const pick = (list: FileList | null) => { if (list?.length) void run(Array.from(list).map(f => ({ name: f.name, body: f }))) }
  const done = rows.filter(r => r.state === 'ok' && r.first)
  return (
    <Modal title="Import drawings" onClose={onClose}>
      <div className="stack">
        <label className={`drop ${over ? 'over' : ''}`} onDragOver={e => { e.preventDefault(); setOver(true) }} onDragLeave={() => setOver(false)}
          onDrop={e => { e.preventDefault(); setOver(false); pick(e.dataTransfer.files) }}>
          <Icon name="upload" size={22} />
          <b>Drop files here, or choose them</b>
          <span className="muted small">.excalidraw, .excalidrawlib (your shapes library), .drawio, .mmd, Markdown with ```mermaid blocks, or a Pocket Draw export.</span>
          <input type="file" multiple hidden accept=".excalidraw,.excalidrawlib,.drawio,.dio,.xml,.mmd,.mermaid,.md,.markdown,.json,.txt" onChange={e => { pick(e.target.files); e.target.value = '' }} />
        </label>
        <Field label="Or paste a Mermaid Live link" help="From mermaid.live: the address in the browser bar (#pako:…).">
          <div className="row"><input value={link} onChange={e => setLink(e.target.value)} placeholder="https://mermaid.live/edit#pako:…" />
            <button type="button" className="btn" disabled={!link.trim()} onClick={() => { void run([{ name: '', body: link.trim() }]); setLink('') }}>Import</button></div>
        </Field>
        <p className="muted xsmall">Moving from Excalidraw: in Excalidraw use Menu → Save to… → Save to disk for each drawing, and Library → Export for your shapes. From draw.io: File → Save as → Device.</p>
        {rows.length > 0 && <ul className="imp-list">
          {rows.map((r, i) => <li key={i} className={r.state}>
            {r.state === 'wait' ? <Spinner /> : <Icon name={r.state === 'ok' ? 'check-circle' : 'x-circle'} size={15} />}
            <span className="grow"><b className="ellipsis">{r.name}</b><span className="small muted">{r.text}</span></span>
            {r.first && <button type="button" className="btn ghost sm" onClick={() => { onClose(); navigate(`/d/${r.first}`) }}>Open</button>}
          </li>)}
        </ul>}
        <div className="form-actions">{done.length > 0 && !busy && <span className="small ok grow">{done.length === 1 ? 'Imported.' : `${done.length} files imported.`}</span>}<button type="button" className="btn primary" onClick={onClose} disabled={busy}>Done</button></div>
      </div>
    </Modal>
  )
}

/** Name a new folder, or rename one. */
export function FolderDialog({ folder, onClose, onDone }: { folder?: Folder; onClose: () => void; onDone: (f: Folder) => void }) {
  const { toast } = useToast()
  const [name, setName] = useState(folder?.name || '')
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      if (folder) { await api.put(`${V1}/folders/${folder.id}`, { name }); announce(); onDone({ ...folder, name: name.trim() }) }
      else { const f = await api.post<Folder>(`${V1}/folders`, { name }); announce(); onDone(f) }
    } catch (err) { toast((err as Error).message, 'err'); setBusy(false) }
  }
  return (
    <Modal title={folder ? 'Rename folder' : 'New folder'} onClose={onClose} size="sm">
      <form onSubmit={save} className="stack">
        <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} autoFocus required maxLength={80} placeholder="House" /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy || !name.trim()}>{folder ? 'Rename' : 'Create'}</button></div>
      </form>
    </Modal>
  )
}

/** Rename a drawing. */
export function RenameDialog({ drawing, onClose, onDone }: { drawing: Drawing; onClose: () => void; onDone: (d: Drawing) => void }) {
  const { toast } = useToast()
  const [title, setTitle] = useState(drawing.title)
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const d = await api.put<Drawing>(`${V1}/drawings/${drawing.id}`, { title }); announce(); onDone(d) } catch (err) { toast((err as Error).message, 'err') }
  }
  return (
    <Modal title="Rename" onClose={onClose} size="sm">
      <form onSubmit={save} className="stack">
        <Field label="Name"><input value={title} onChange={e => setTitle(e.target.value)} autoFocus maxLength={200} onFocus={e => e.target.select()} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Rename</button></div>
      </form>
    </Modal>
  )
}

/** Move a drawing to a folder (or out of one, or into a new one). */
export function MoveDialog({ drawing, onClose, onDone }: { drawing: Drawing; onClose: () => void; onDone: (d: Drawing) => void }) {
  const { toast } = useToast()
  const [folders, setFolders] = useState<Folder[]>([])
  const [folder, setFolder] = useState(drawing.folder_id ? String(drawing.folder_id) : '')
  const [fresh, setFresh] = useState('')
  useEffect(() => { api.get<Folder[]>(`${V1}/folders`).then(setFolders).catch(() => {}) }, [])
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      let id: number | null = folder ? Number(folder) : null
      if (folder === 'new') id = (await api.post<Folder>(`${V1}/folders`, { name: fresh })).id
      const d = await api.put<Drawing>(`${V1}/drawings/${drawing.id}`, { folder_id: id })
      announce(); onDone(d)
    } catch (err) { toast((err as Error).message, 'err') }
  }
  return (
    <Modal title={`Move “${drawing.title}”`} onClose={onClose} size="sm">
      <form onSubmit={save} className="stack">
        <Field label="Folder"><Select ariaLabel="Folder" value={folder} onChange={setFolder} options={[{ value: '', label: 'No folder' }, ...folders.map(f => ({ value: String(f.id), label: f.name })), { value: 'new', label: 'A new folder…' }]} /></Field>
        {folder === 'new' && <Field label="New folder"><input value={fresh} onChange={e => setFresh(e.target.value)} autoFocus maxLength={80} placeholder="House" required /></Field>}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={folder === 'new' && !fresh.trim()}>Move</button></div>
      </form>
    </Modal>
  )
}
