// Left column: the new button, the lists with their numbers, kinds and folders — the Pocket Calendar sidebar.
import { Link, NavLink } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { KINDS, KIND_ORDER } from '../lib/draw'
import { Icon } from './icons'
import { useDraw } from './Layout'

export function Sidebar({ onNavigate }: { onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { folders, counts, newDrawing, newFolder, importFiles } = useDraw()
  const title = settings?.title || status?.title || 'Pocket Draw'
  const n = (v?: number) => v ? (v > 999 ? '999+' : String(v)) : undefined
  const nav: { to: string; label: string; icon: string; end?: boolean; count?: string }[] = [
    { to: '/', label: 'All drawings', icon: 'grid', end: true, count: n(counts?.all) },
    { to: '/recent', label: 'Recent', icon: 'clock' },
    { to: '/starred', label: 'Starred', icon: 'star', count: n(counts?.starred) },
  ]
  return (
    <aside className="sidebar" aria-label="Drawings">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Draw' ? title : <>Pocket <b>Draw</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" onClick={() => newDrawing('ask')} title="New drawing (N)">
        <Icon name="plus" size={15} strokeWidth={2.2} />New drawing
      </button>

      <nav className="sb-nav" aria-label="Lists">
        {nav.map(x => (
          <NavLink key={x.to} to={x.to} end={x.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={x.icon} size={16} />{x.label}
            {!!x.count && <span className="sb-count">{x.count}</span>}
          </NavLink>
        ))}
      </nav>

      <section className="sb-sec" aria-label="Kinds">
        <h4 className="sb-h"><span>Kinds</span></h4>
        {KIND_ORDER.map(k => (
          <NavLink key={k} to={`/kind/${k}`} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={KINDS[k].icon} size={16} /><span className="grow">{KINDS[k].plural}</span>
            {k === 'drawio' && !settings?.drawio_url && !counts?.kinds?.[k] ? <span className="sb-off" title="Not switched on yet: needs draw.io">off</span> : !!counts?.kinds?.[k] && <span className="sb-count">{n(counts.kinds[k])}</span>}
          </NavLink>
        ))}
      </section>

      <section className="sb-sec sb-folders" aria-label="Folders">
        <h4 className="sb-h">
          <span>Folders</span>
          <button type="button" className="btn ghost icon-only xs" onClick={() => { onNavigate(); newFolder() }} title="New folder" aria-label="New folder"><Icon name="plus" size={14} /></button>
        </h4>
        {folders.length === 0 && <p className="sb-empty muted small">Folders keep drawings together — “House”, “Work”, “Trips”.</p>}
        {folders.map(f => (
          <NavLink key={f.id} to={`/folder/${f.id}`} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`} title={f.name}>
            <Icon name="folder" size={15} /><span className="ellipsis grow">{f.name}</span>{!!f.count && <span className="sb-count">{n(f.count)}</span>}
          </NavLink>
        ))}
      </section>

      <div className="sb-foot">
        <NavLink to="/trash" onClick={onNavigate} className={({ isActive }) => `sb-foot-link ${isActive ? 'active' : ''}`}><Icon name="trash" size={15} />Trash{!!counts?.trash && <span className="sb-count">{n(counts.trash)}</span>}</NavLink>
        <button type="button" className="sb-foot-link" onClick={() => importFiles()}><Icon name="upload" size={15} />Import</button>
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
      </div>
    </aside>
  )
}
