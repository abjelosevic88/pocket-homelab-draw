// The drawings: all of them, recent, starred, shared with me, one kind, one folder, search results or the trash — as cards or a list.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announce, fillPreviews, KINDS, previewURL, since, useOnChange } from '../lib/draw'
import { fmtBytes, plural } from '../lib/format'
import type { Drawing, Kind, Settings } from '../lib/types'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Confirm, Empty, Loading, type MenuItem, MenuButton, Segmented, download, useDebounce, useToast } from '../components/ui'
import { useCrumb, useDraw } from '../components/Layout'
import { FolderDialog, MoveDialog, RenameDialog } from '../components/Dialogs'
import { DrawioHelp } from '../components/DrawioHelp'

export type View = 'all' | 'recent' | 'starred' | 'shared' | 'trash' | 'kind' | 'folder' | 'search'

const TITLES: Record<string, string> = { all: 'All drawings', recent: 'Recent', starred: 'Starred', shared: 'Shared with me', trash: 'Trash', search: 'Search' }

export default function Drawings({ view }: { view: View }) {
  const { id: param, kind: kindParam } = useParams()
  const [sp, setSp] = useSearchParams()
  const navigate = useNavigate()
  const { settings, setSettings, user } = useApp()
  const { folders, counts, newDrawing, importFiles, reload: reloadCounts } = useDraw()
  const { toast } = useToast()
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q.trim(), 250)
  const [list, setList] = useState<Drawing[] | null>(null)
  const [total, setTotal] = useState(0)
  const [limit, setLimit] = useState(120)
  const [over, setOver] = useState(false)
  const [rename, setRename] = useState<Drawing | null>(null)
  const [move, setMove] = useState<Drawing | null>(null)
  const [forever, setForever] = useState<Drawing | null>(null)
  const [emptying, setEmptying] = useState(false)
  const [folderEdit, setFolderEdit] = useState(false)
  const [folderDelete, setFolderDelete] = useState(false)
  const sort = settings?.sort_by || 'edited'
  const style = settings?.list_style || 'grid'

  const folder = view === 'folder' ? folders.find(f => String(f.id) === param) : undefined
  const kind = view === 'kind' && kindParam && kindParam in KINDS ? kindParam as Kind : undefined
  const title = view === 'folder' ? folder?.name || 'Folder' : view === 'kind' && kind ? KINDS[kind].plural : TITLES[view]
  useCrumb(view === 'folder' || view === 'kind' ? title : undefined)

  useEffect(() => { if (view === 'search') setQ(sp.get('q') || '') }, [sp, view])

  const url = useMemo(() => {
    const p = new URLSearchParams({ limit: String(limit), sort })
    if (view === 'recent' || view === 'starred' || view === 'shared' || view === 'trash') p.set('view', view)
    if (kind) p.set('kind', kind)
    if (view === 'folder' && param) p.set('folder', param)
    if (dq) p.set('q', dq)
    return `${V1}/drawings?${p}`
  }, [view, kind, param, dq, sort, limit])

  const load = useCallback(() => {
    api.get<{ drawings: Drawing[]; total: number }>(url).then(r => { setList(r.drawings); setTotal(r.total) }).catch(e => toast((e as Error).message, 'err'))
  }, [url, toast])
  useEffect(() => { load() }, [load])
  useOnChange(load)

  // pictures the browser still has to draw (imports, drawings other apps added, the demo)
  useEffect(() => {
    if (!list || !settings) return
    void fillPreviews(list, { theme: settings.mermaid_theme, look: settings.mermaid_look }, (id, rev) =>
      setList(l => l && l.map(d => d.id === id ? { ...d, has_preview: true, preview_rev: rev } : d)))
  }, [list, settings])

  const patch = async (d: Drawing, body: Partial<Drawing>) => {
    setList(l => l && l.map(x => x.id === d.id ? { ...x, ...body } : x))
    try { await api.put(`${V1}/drawings/${d.id}`, body); if ('starred' in body) reloadCounts() } catch (e) { toast((e as Error).message, 'err'); load() }
  }
  const act = async (fn: () => Promise<unknown>, msg: string) => {
    try { await fn(); toast(msg); announce() } catch (e) { toast((e as Error).message, 'err') }
  }
  const saveSetting = async (p: Partial<Settings>) => {
    if (!settings) return
    setSettings({ ...settings, ...p })
    try { setSettings(await api.put<Settings>(`${V1}/settings`, p)) } catch { /* the view still changed here */ }
  }

  const menu = (d: Drawing): MenuItem[] => d.access && d.access !== 'owner' ? [
    { label: 'Open', icon: 'edit', onSelect: () => navigate(`/d/${d.id}`) },
    { label: `Download the ${d.kind === 'sketch' ? '.excalidraw' : d.kind === 'mermaid' ? '.mmd' : '.drawio'} file`, icon: 'download', onSelect: () => download(`${V1}/drawings/${d.id}/file`) },
    'sep',
    { label: 'Remove from Shared with me', icon: 'x', danger: true, onSelect: () => act(() => api.del(`${V1}/drawings/${d.id}/shares/${user?.id ?? 0}`), 'Removed') },
  ] : view === 'trash' ? [
    { label: 'Put back', icon: 'undo', onSelect: () => act(() => api.post(`${V1}/drawings/${d.id}/restore`), 'Put back') },
    'sep',
    { label: 'Delete for good…', icon: 'trash', danger: true, onSelect: () => setForever(d) },
  ] : [
    { label: 'Open', icon: 'edit', onSelect: () => navigate(`/d/${d.id}`) },
    { label: d.starred ? 'Remove the star' : 'Star', icon: 'star', onSelect: () => patch(d, { starred: !d.starred }) },
    { label: 'Rename…', icon: 'type', onSelect: () => setRename(d) },
    { label: 'Move to a folder…', icon: 'folder', onSelect: () => setMove(d) },
    { label: 'Duplicate', icon: 'copy', onSelect: () => act(() => api.post(`${V1}/drawings/${d.id}/duplicate`), 'Copy made') },
    { label: `Download the ${d.kind === 'sketch' ? '.excalidraw' : d.kind === 'mermaid' ? '.mmd' : '.drawio'} file`, icon: 'download', onSelect: () => download(`${V1}/drawings/${d.id}/file`) },
    ...(settings?.files_configured ? [{ label: 'Copy to Pocket Files', icon: 'layers', onSelect: () => copyToFiles(d) }] : []),
    'sep',
    { label: 'Move to the trash', icon: 'trash', danger: true, onSelect: () => act(() => api.del(`${V1}/drawings/${d.id}`), 'Moved to the trash') },
  ]
  const copyToFiles = async (d: Drawing) => {
    try {
      const r = await api.post<{ ok: boolean; error?: string; path?: string }>(`${V1}/drawings/${d.id}/files`)
      if (r.ok) toast(`Copied to ${r.path}`); else toast(r.error || 'failed', 'err')
    } catch (e) { toast((e as Error).message, 'err') }
  }

  const headActions = (
    <div className="dr-tools">
      {view !== 'trash' && <div className="dr-search">
        <Icon name="search" size={15} />
        <input value={q} onChange={e => { setQ(e.target.value); if (view === 'search') setSp(e.target.value ? { q: e.target.value } : {}, { replace: true }) }} placeholder="Find by name or words on it" aria-label="Find" />
        {q && <button type="button" className="btn ghost icon-only xs" aria-label="Clear" onClick={() => setQ('')}><Icon name="x" size={14} /></button>}
      </div>}
      {view !== 'recent' && view !== 'trash' && <Select ariaLabel="Sort" value={sort} onChange={v => saveSetting({ sort_by: v as Settings['sort_by'] })}
        options={[{ value: 'edited', label: 'Last changed' }, { value: 'created', label: 'Newest' }, { value: 'title', label: 'Name' }]} />}
      <Segmented size="sm" label="Look" value={style} onChange={v => saveSetting({ list_style: v as Settings['list_style'] })}
        options={[{ value: 'grid', label: <Icon name="grid" size={15} />, title: 'Cards' }, { value: 'list', label: <Icon name="list" size={15} />, title: 'List' }]} />
    </div>
  )

  const empty = () => {
    if (dq) return <Empty icon="search" title="Nothing found">No drawing has “{dq}” in its name or on it.</Empty>
    switch (view) {
      case 'trash': return <Empty icon="trash" title="The trash is empty">Drawings you delete wait here for {settings?.trash_days ? plural(settings.trash_days, 'day') : 'you'} before they go for good.</Empty>
      case 'starred': return <Empty icon="star" title="No starred drawings">Star the ones you open often; they wait here.</Empty>
      case 'shared': return <Empty icon="users" title="Nothing shared with you">When someone here shares a drawing with you, it shows up here.</Empty>
      case 'recent': return <Empty icon="clock" title="Nothing opened yet">The drawings you open show up here, newest first.</Empty>
      case 'folder': return <Empty icon="folder" title="This folder is empty"><button type="button" className="btn primary sm mt-s" onClick={() => newDrawing('ask', folder?.id ?? null)}><Icon name="plus" size={14} />New drawing here</button></Empty>
      case 'kind': return <Empty icon={kind ? KINDS[kind].icon : 'grid'} title={`No ${kind ? KINDS[kind].plural.toLowerCase() : 'drawings'} yet`}><button type="button" className="btn primary sm mt-s" onClick={() => newDrawing(kind)}><Icon name="plus" size={14} />New {kind ? KINDS[kind].label.toLowerCase() : 'drawing'}</button></Empty>
    }
    return (
      <div className="dr-welcome">
        <h3>Start your first drawing</h3>
        <p className="muted">Pick what suits the idea. Everything is kept here with its history; nothing to install.</p>
        <div className="kind-cards">
          {(['sketch', 'mermaid', 'drawio'] as Kind[]).map(k => (
            <button key={k} type="button" className={`kind-card k-${k} ${k === 'drawio' && !settings?.drawio_url ? 'not-ready' : ''}`} onClick={() => newDrawing(k)}>
              <span className="kind-ico"><Icon name={KINDS[k].icon} size={22} /></span><span className="kind-name">{KINDS[k].label}</span><span className="kind-blurb">{KINDS[k].blurb}{k === 'drawio' && !settings?.drawio_url && <b className="kind-off"> Not switched on yet.</b>}</span>
            </button>
          ))}
        </div>
        <p className="muted small mt">Coming from Excalidraw, draw.io or Mermaid Live? <button type="button" className="link-btn" onClick={() => importFiles()}>Import your files</button> or drop them on this page.</p>
      </div>
    )
  }

  return (
    <div className={`dr-page ${over ? 'drop-over' : ''}`}
      onDragOver={e => { if (view !== 'trash' && e.dataTransfer.types.includes('Files')) { e.preventDefault(); setOver(true) } }}
      onDragLeave={e => { if (e.currentTarget === e.target) setOver(false) }}
      onDrop={e => { if (view === 'trash') return; e.preventDefault(); setOver(false); if (e.dataTransfer.files.length) importFiles(Array.from(e.dataTransfer.files)) }}>
      <div className="dr-head">
        <div className="dr-title">
          <h2>{title}</h2>
          {list && <span className="muted small tnum">{plural(total, 'drawing')}{view === 'all' && counts?.bytes ? ` · ${fmtBytes(counts.bytes)}` : ''}</span>}
          {view === 'folder' && folder && <MenuButton label="Folder" width={200} items={() => [
            { label: 'New drawing here', icon: 'plus', onSelect: () => newDrawing('ask', folder.id) },
            { label: 'Rename folder…', icon: 'type', onSelect: () => setFolderEdit(true) },
            'sep',
            { label: 'Delete folder…', icon: 'trash', danger: true, onSelect: () => setFolderDelete(true) },
          ]}><Icon name="more" size={16} /></MenuButton>}
          {view === 'trash' && !!list?.length && <button type="button" className="btn sm ghost danger-text" onClick={() => setEmptying(true)}>Empty the trash</button>}
        </div>
        {headActions}
      </div>

      {view === 'kind' && kind === 'drawio' && <div className="mb"><DrawioHelp /></div>}
      {!list ? <Loading /> : list.length === 0 ? empty() : style === 'grid' ? (
        <div className="dr-grid">
          {list.map(d => <Card key={d.id} d={d} trash={view === 'trash'} onOpen={() => view !== 'trash' && navigate(`/d/${d.id}`)} onStar={() => patch(d, { starred: !d.starred })} menu={() => menu(d)} />)}
        </div>
      ) : (
        <div className="dr-list" role="list">
          {list.map(d => <Row key={d.id} d={d} trash={view === 'trash'} onOpen={() => view !== 'trash' && navigate(`/d/${d.id}`)} onStar={() => patch(d, { starred: !d.starred })} menu={() => menu(d)} />)}
        </div>
      )}
      {list && total > list.length && <div className="row mt" style={{ justifyContent: 'center' }}><button type="button" className="btn" onClick={() => setLimit(l => l + 120)}>Show more</button></div>}
      {over && <div className="drop-hint"><Icon name="upload" size={22} />Drop to import</div>}

      {rename && <RenameDialog drawing={rename} onClose={() => setRename(null)} onDone={() => setRename(null)} />}
      {move && <MoveDialog drawing={move} onClose={() => setMove(null)} onDone={() => setMove(null)} />}
      {forever && <Confirm title={`Delete “${forever.title}” for good?`} message="It and every earlier version are gone; this cannot be undone." confirmLabel="Delete" onCancel={() => setForever(null)}
        onConfirm={() => { const d = forever; setForever(null); void act(() => api.del(`${V1}/drawings/${d.id}?forever=1`), 'Deleted') }} />}
      {emptying && <Confirm title="Empty the trash?" message={`${plural(total, 'drawing')} and their versions go for good.`} confirmLabel="Empty" onCancel={() => setEmptying(false)}
        onConfirm={() => { setEmptying(false); void act(() => api.post(`${V1}/trash/empty`), 'Trash emptied') }} />}
      {folderEdit && folder && <FolderDialog folder={folder} onClose={() => setFolderEdit(false)} onDone={() => setFolderEdit(false)} />}
      {folderDelete && folder && <Confirm title={`Delete the folder “${folder.name}”?`} message="The drawings in it stay; they are just no longer in a folder." confirmLabel="Delete folder" onCancel={() => setFolderDelete(false)}
        onConfirm={() => { setFolderDelete(false); void act(() => api.del(`${V1}/folders/${folder.id}`), 'Folder deleted').then(() => navigate('/')) }} />}
    </div>
  )
}

function Thumb({ d }: { d: Drawing }) {
  const [broken, setBroken] = useState(false)
  if (!d.has_preview || broken) return <div className={`thumb empty k-${d.kind}`}><Icon name={KINDS[d.kind].icon} size={28} /></div>
  return <div className="thumb"><img src={previewURL(d)} alt="" loading="lazy" decoding="async" onError={() => setBroken(true)} /></div>
}

function meta(d: Drawing, trash: boolean) {
  if (trash) return `Deleted ${since(d.trashed_at)}`
  const whose = d.owner_name ? ` · ${d.owner_name}${d.access === 'view' ? ', view only' : ''}` : d.shares ? ' · shared' : ''
  return `${KINDS[d.kind].label} · ${since(d.edited_at)}${whose}`
}

interface ItemProps { d: Drawing; trash: boolean; onOpen: () => void; onStar: () => void; menu: () => MenuItem[] }

function Card({ d, trash, onOpen, onStar, menu }: ItemProps) {
  return (
    <article className={`dr-card ${trash ? 'trashed' : ''}`}>
      <button type="button" className="dr-card-open" onClick={onOpen} aria-label={`Open ${d.title}`} disabled={trash}><Thumb d={d} /></button>
      <div className="dr-card-foot">
        <div className="grow dr-card-text" onClick={onOpen}>
          <div className="dr-card-title ellipsis" title={d.title}>{d.title}</div>
          <div className="dr-card-meta ellipsis">{meta(d, trash)}{d.folder_name ? ` · ${d.folder_name}` : ''}</div>
        </div>
        {!trash && !d.owner_name && <button type="button" className={`btn ghost icon-only sm star-btn ${d.starred ? 'on' : ''}`} onClick={onStar} aria-label={d.starred ? 'Remove the star' : 'Star'} aria-pressed={d.starred}><Icon name="star" size={15} /></button>}
        <MenuButton label="More" className="btn ghost icon-only sm" align="right" width={240} items={menu}><Icon name="more" size={16} /></MenuButton>
      </div>
      {d.source !== 'app' && d.source !== 'import' && <span className="dr-src" title={`Added by ${d.source}`}><Icon name="sparkles" size={12} /></span>}
    </article>
  )
}

function Row({ d, trash, onOpen, onStar, menu }: ItemProps) {
  return (
    <div className={`dr-row ${trash ? 'trashed' : ''}`} role="listitem">
      <button type="button" className="dr-row-open" onClick={onOpen} disabled={trash}>
        <span className={`dr-row-ico k-${d.kind}`}><Icon name={KINDS[d.kind].icon} size={16} /></span>
        <span className="grow ellipsis"><b>{d.title}</b>{d.folder_name && <span className="muted small"> · {d.folder_name}</span>}</span>
        <span className="muted small dr-row-meta">{meta(d, trash)}</span>
      </button>
      {!trash && !d.owner_name && <button type="button" className={`btn ghost icon-only sm star-btn ${d.starred ? 'on' : ''}`} onClick={onStar} aria-label={d.starred ? 'Remove the star' : 'Star'} aria-pressed={d.starred}><Icon name="star" size={15} /></button>}
      <MenuButton label="More" className="btn ghost icon-only sm" align="right" width={240} items={menu}><Icon name="more" size={16} /></MenuButton>
    </div>
  )
}
