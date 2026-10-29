// One drawing, full screen: a slim header (name, saved state, star, history, menu) over the editor for its kind.
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, ApiError, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announce, fileBase, KINDS, saveBlob, svgToPng } from '../lib/draw'
import { ago, fmtDateTime } from '../lib/format'
import type { Drawing, Version } from '../lib/types'
import { Icon } from '../components/icons'
import { AskButton } from '../components/AskButton'
import { Confirm, Loading, type MenuItem, MenuButton, Modal, Spinner, download, useToast } from '../components/ui'
import { MoveDialog } from '../components/Dialogs'
import { UsedIn } from '../components/UsedIn'
import type { EditorProps } from '../components/SketchEditor'

const SketchEditor = lazy(() => import('../components/SketchEditor'))
const MermaidEditor = lazy(() => import('../components/MermaidEditor'))
const DrawioEditor = lazy(() => import('../components/DrawioEditor'))

type SaveState = 'saved' | 'waiting' | 'saving' | 'error' | 'conflict'

/** Saves changes a moment after they stop, one request at a time, always on the rev the drawing is at. */
function useSaver(id: number, startRev: number, onSaved: (d: Drawing) => void) {
  const rev = useRef(startRev)
  const next = useRef<{ getData: () => string; makePreview: () => Promise<string> } | null>(null)
  const inFlight = useRef(false)
  const timer = useRef(0)
  const [state, setState] = useState<SaveState>('saved')
  const stateRef = useRef<SaveState>('saved')
  const set = (s: SaveState) => { stateRef.current = s; setState(s) }

  const run = useCallback(async () => {
    window.clearTimeout(timer.current)
    if (inFlight.current || !next.current || stateRef.current === 'conflict') return
    const job = next.current
    next.current = null
    inFlight.current = true
    set('saving')
    try {
      const data = job.getData()
      let preview: string | undefined
      try { preview = await job.makePreview() } catch { preview = undefined }
      const body: Record<string, unknown> = { data, rev: rev.current }
      if (preview) body.preview = preview
      const d = await api.put<Drawing>(`${V1}/drawings/${id}`, body)
      rev.current = d.rev
      onSaved(d)
      set(next.current ? 'waiting' : 'saved')
    } catch (e) {
      if (!next.current) next.current = job
      if (e instanceof ApiError && e.status === 409) set('conflict')
      else { set('error'); timer.current = window.setTimeout(() => void run(), 5000) }
    } finally {
      inFlight.current = false
      const st = stateRef.current as SaveState
      if (next.current && st !== 'conflict' && st !== 'error') timer.current = window.setTimeout(() => void run(), 300)
    }
  }, [id, onSaved])

  const change = useCallback((getData: () => string, makePreview: () => Promise<string>) => {
    next.current = { getData, makePreview }
    if (stateRef.current !== 'conflict') set(inFlight.current ? 'saving' : 'waiting')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => void run(), 1200)
  }, [run])

  /** Keep mine: save the waiting change on top of whatever is there now. */
  const overwrite = useCallback(async () => {
    const d = await api.get<Drawing>(`${V1}/drawings/${id}`)
    rev.current = d.rev
    set('waiting')
    void run()
  }, [id, run])

  // leaving with a change not saved yet: save it now, and ask the browser to wait
  useEffect(() => {
    const before = (e: BeforeUnloadEvent) => { if (next.current || inFlight.current) { void run(); e.preventDefault() } }
    window.addEventListener('beforeunload', before)
    return () => { window.removeEventListener('beforeunload', before); window.clearTimeout(timer.current); if (next.current) void run() }
  }, [run])

  return { state, change, overwrite, flush: run, pending: () => !!next.current || inFlight.current, setRev: (r: number) => { rev.current = r } }
}

export default function Editor() {
  const { id: raw } = useParams()
  const id = Number(raw)
  const [d, setD] = useState<Drawing | null>(null)
  const [error, setError] = useState('')
  const [key, setKey] = useState(0) // remounts the editor (a version restored, the newer one loaded)

  const load = useCallback(() => {
    api.get<Drawing>(`${V1}/drawings/${id}?open=1`).then(x => { setD(x); setKey(k => k + 1) }).catch(e => setError((e as Error).message))
  }, [id])
  useEffect(() => { load() }, [load])

  if (error) return <div className="ed-page"><div className="ed-gone"><Icon name="alert" size={24} /><h3>{error === 'not found' ? 'This drawing is not here' : error}</h3><a className="btn" href="/">Back to the drawings</a></div></div>
  if (!d) return <div className="ed-page"><Loading /></div>
  return <EditorBody key={key} initial={d} reload={load} />
}

function EditorBody({ initial, reload }: { initial: Drawing; reload: () => void }) {
  const navigate = useNavigate()
  const { settings, themeMode } = useApp()
  const { toast } = useToast()
  const [d, setD] = useState(initial)
  const [title, setTitle] = useState(initial.title)
  const [history, setHistory] = useState(false)
  const [move, setMove] = useState(false)
  const [trash, setTrash] = useState(false)
  const [used, setUsed] = useState(false)
  const preview = useRef<(() => Promise<string>) | null>(null)
  const dark = document.documentElement.dataset.theme === 'dark'
  useEffect(() => { document.title = `${d.title} · Pocket Draw`; return () => { document.title = settings?.title || 'Pocket Draw' } }, [d.title, settings?.title])

  const onSaved = useCallback((x: Drawing) => setD(cur => ({ ...cur, ...x, data: undefined })), [])
  const saver = useSaver(initial.id, initial.rev, onSaved)
  const changeRef = useRef(saver.change)
  changeRef.current = saver.change
  const onChange = useCallback<EditorProps['onChange']>((getData, makePreview) => { preview.current = makePreview; changeRef.current(getData, makePreview) }, [])

  const saveTitle = async () => {
    const t = title.trim() || d.title
    setTitle(t)
    if (t === d.title) return
    try { const x = await api.put<Drawing>(`${V1}/drawings/${d.id}`, { title: t }); setD(c => ({ ...c, title: x.title })); announce() } catch (e) { toast((e as Error).message, 'err') }
  }
  const star = async () => {
    const s = !d.starred
    setD(c => ({ ...c, starred: s }))
    try { await api.put(`${V1}/drawings/${d.id}`, { starred: s }); announce() } catch (e) { toast((e as Error).message, 'err') }
  }
  const back = async () => { if (saver.pending()) await saver.flush(); announce(); if ((window.history.state?.idx ?? 0) > 0) navigate(-1); else navigate('/') }
  const picture = async (): Promise<string> => {
    if (preview.current) { try { return await preview.current() } catch { /* the saved one, then */ } }
    return await api.getText(`${V1}/drawings/${d.id}/preview.svg`)
  }
  const exportSVG = async () => { try { saveBlob(new Blob([await picture()], { type: 'image/svg+xml' }), `${fileBase(d.title)}.svg`) } catch (e) { toast((e as Error).message, 'err') } }
  const exportPNG = async () => { try { saveBlob(await svgToPng(await picture()), `${fileBase(d.title)}.png`) } catch (e) { toast((e as Error).message, 'err') } }
  const copyToFiles = async () => {
    if (saver.pending()) await saver.flush()
    try {
      const r = await api.post<{ ok: boolean; error?: string; path?: string; open_url?: string }>(`${V1}/drawings/${d.id}/files`)
      if (!r.ok) { toast(r.error || 'failed', 'err'); return }
      toast(`Copied to ${r.path}`)
      setD(c => ({ ...c, files_path: r.path || '', files_rev: c.rev, files_error: '' }))
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const duplicate = async () => {
    if (saver.pending()) await saver.flush()
    try { const x = await api.post<Drawing>(`${V1}/drawings/${d.id}/duplicate`); announce(); navigate(`/d/${x.id}`) } catch (e) { toast((e as Error).message, 'err') }
  }
  const ext = d.kind === 'sketch' ? '.excalidraw' : d.kind === 'mermaid' ? '.mmd' : '.drawio'
  const menu = (): MenuItem[] => [
    { label: 'History…', icon: 'history', onSelect: () => setHistory(true), hint: d.versions ? String(d.versions) : undefined },
    { label: 'Used in notes and issues…', icon: 'link', onSelect: () => setUsed(true) },
    { label: 'Move to a folder…', icon: 'folder', onSelect: () => setMove(true), hint: d.folder_name || undefined },
    { label: 'Duplicate', icon: 'copy', onSelect: duplicate },
    'sep',
    { heading: 'Download' },
    { label: 'Picture (SVG)', icon: 'download', onSelect: exportSVG },
    { label: 'Picture (PNG)', icon: 'download', onSelect: exportPNG },
    { label: `File for ${KINDS[d.kind].what} (${ext})`, icon: 'file', onSelect: async () => { if (saver.pending()) await saver.flush(); download(`${V1}/drawings/${d.id}/file`) } },
    ...(settings?.files_configured ? ['sep' as const, { label: d.files_path ? 'Copy to Pocket Files again' : 'Copy to Pocket Files', icon: 'layers', onSelect: copyToFiles, hint: d.files_path && d.files_rev >= d.rev ? 'up to date' : undefined }] : []),
    'sep',
    { label: 'Move to the trash', icon: 'trash', danger: true, onSelect: () => setTrash(true) },
  ]

  const status: Record<SaveState, [string, string]> = {
    saved: ['Saved', 'check'], waiting: ['Saving…', ''], saving: ['Saving…', ''], error: ['Not saved — trying again', 'alert'], conflict: ['Changed elsewhere', 'alert'],
  }
  const [label, icon] = status[saver.state]
  const props: EditorProps = { data: initial.data || '', title: d.title, dark, settings: settings!, onChange }
  void themeMode
  return (
    <div className={`ed-page k-${d.kind}`}>
      <header className="ed-head">
        <button type="button" className="btn ghost icon-only" onClick={back} aria-label="Back to the drawings" title="Back to the drawings"><Icon name="chevron-left" size={18} /></button>
        <span className={`ed-kind k-${d.kind}`} title={KINDS[d.kind].label}><Icon name={KINDS[d.kind].icon} size={15} /></span>
        <input className="ed-title" value={title} onChange={e => setTitle(e.target.value)} onBlur={saveTitle} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); if (e.key === 'Escape') { setTitle(d.title); (e.target as HTMLInputElement).blur() } }} aria-label="Name" maxLength={200} />
        <span className={`ed-status s-${saver.state}`} role="status" aria-live="polite">{icon ? <Icon name={icon} size={13} /> : <Spinner size={11} />}<span>{label}</span></span>
        <span className="spacer" />
        <button type="button" className={`btn ghost icon-only star-btn ${d.starred ? 'on' : ''}`} onClick={star} aria-pressed={d.starred} aria-label={d.starred ? 'Remove the star' : 'Star'} title={d.starred ? 'Remove the star' : 'Star'}><Icon name="star" size={17} /></button>
        <button type="button" className="btn ghost icon-only ed-hide-s" onClick={() => setUsed(true)} aria-label="Used in" title="Used in notes and issues"><Icon name="link" size={17} /></button>
        <button type="button" className="btn ghost icon-only ed-hide-s" onClick={() => setHistory(true)} aria-label="History" title="History"><Icon name="history" size={17} /></button>
        <MenuButton label="More" align="right" width={260} items={menu}><Icon name="more" size={17} /></MenuButton>
        <AskButton />
      </header>
      {saver.state === 'conflict' && <div className="ed-conflict">
        <Icon name="alert" size={15} /><span className="grow">This drawing was changed in another window or by another app since you opened it.</span>
        <button type="button" className="btn sm" onClick={reload}>Load the newer one</button>
        <button type="button" className="btn sm primary" onClick={() => void saver.overwrite()}>Keep mine</button>
      </div>}
      <div className="ed-body">
        <Suspense fallback={<Loading label="Opening the editor…" />}>
          {d.kind === 'sketch' && <SketchEditor {...props} />}
          {d.kind === 'mermaid' && <MermaidEditor {...props} />}
          {d.kind === 'drawio' && <DrawioEditor {...props} />}
        </Suspense>
      </div>
      {history && <HistorySheet drawing={d} onClose={() => setHistory(false)} onRestored={async () => { setHistory(false); if (saver.pending()) await saver.flush(); reload(); toast('The earlier version is back; what you had is in History') }} />}
      {used && <UsedIn drawing={d} onClose={() => setUsed(false)} />}
      {move && <MoveDialog drawing={d} onClose={() => setMove(false)} onDone={x => { setMove(false); setD(c => ({ ...c, folder_id: x.folder_id, folder_name: x.folder_name })) }} />}
      {trash && <Confirm title={`Move “${d.title}” to the trash?`} message={`It stays in the trash for ${settings?.trash_days || 30} days; you can put it back from there.`} confirmLabel="Move to trash" onCancel={() => setTrash(false)}
        onConfirm={async () => { setTrash(false); if (saver.pending()) await saver.flush(); await api.del(`${V1}/drawings/${d.id}`); announce(); navigate('/') }} />}
    </div>
  )
}

function HistorySheet({ drawing, onClose, onRestored }: { drawing: Drawing; onClose: () => void; onRestored: () => void }) {
  const { toast } = useToast()
  const { settings } = useApp()
  const [list, setList] = useState<Version[] | null>(null)
  const [pick, setPick] = useState<Version | null>(null)
  useEffect(() => { api.get<Version[]>(`${V1}/drawings/${drawing.id}/versions`).then(setList).catch(e => toast((e as Error).message, 'err')) }, [drawing.id, toast])
  const restore = async (v: Version) => {
    try { await api.post(`${V1}/drawings/${drawing.id}/versions/${v.id}/restore`); announce(); onRestored() } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Modal title="History" onClose={onClose} size="lg">
      <p className="muted small">While you draw, the drawing is kept as it was every ten minutes or so, and before anything is put back. Up to {settings?.keep_versions || 50} versions per drawing are kept (Settings).</p>
      {!list ? <Loading /> : list.length === 0 ? <p className="muted">No earlier versions yet — they appear after the first changes.</p> : (
        <div className="hist-grid">
          {list.map(v => (
            <button key={v.id} type="button" className={`hist-item ${pick?.id === v.id ? 'on' : ''}`} onClick={() => setPick(v)}>
              <span className="thumb">{v.has_preview ? <img src={`${V1}/drawings/${drawing.id}/versions/${v.id}/preview.svg`} alt="" loading="lazy" /> : <Icon name={KINDS[drawing.kind].icon} size={22} />}</span>
              <span className="hist-when">{fmtDateTime(v.created_at)}</span>
              <span className="muted xsmall">{ago(v.created_at)}{v.reason === 'restore' ? ' · before a restore' : ''}</span>
            </button>
          ))}
        </div>
      )}
      <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Close</button><button type="button" className="btn primary" disabled={!pick} onClick={() => pick && restore(pick)}>Put this version back</button></div>
    </Modal>
  )
}
