import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { ImportDialog } from '../components/Dialogs'
import { useApp } from '../lib/app-context'
import type { APIToken, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'

type TabId = 'general' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'pen', sub: 'What a new drawing starts as, how sketches and text diagrams look, the list, the trash and history.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'A word when another app adds a drawing for you, or when copies stop reaching Pocket Files.' },
  { id: 'apps', label: 'Connected apps', icon: 'layers', sub: 'Pocket Files keeps a copy of each drawing; other Pocket apps can see and add drawings.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who drafts and fixes text diagrams: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box.' },
  { id: 'tokens', label: 'API & tokens', icon: 'key', sub: 'Tokens for scripts and widgets, and what the API offers.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Import from Excalidraw, draw.io and Mermaid; export everything.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const TRASH = [{ value: '7', label: 'a week' }, { value: '30', label: 'a month' }, { value: '90', label: '3 months' }, { value: '365', label: 'a year' }, { value: '0', label: 'until I empty it' }]
const VERSIONS = [{ value: '10', label: '10' }, { value: '25', label: '25' }, { value: '50', label: '50' }, { value: '100', label: '100' }, { value: '250', label: '250' }]

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  const opt = (list: { value: string; label: string }[], v: number, unit: string) => list.some(e => e.value === String(v)) ? list : [...list, { value: String(v), label: `${v} ${unit}` }]
  return (
    <div className="stack loose">
      <Section title="New drawings" icon="plus">
        <SettingRow title="The New button makes" help="“Ask me” shows the choice each time.">
          <Select ariaLabel="New drawing" value={f.default_kind} onChange={v => setF({ ...f, default_kind: v as SettingsT['default_kind'] })} options={[
            { value: 'ask', label: 'Ask me' }, { value: 'sketch', label: 'A sketch' }, { value: 'mermaid', label: 'A text diagram' },
            { value: 'drawio', label: 'A diagram (draw.io)', disabled: !f.drawio_url, hint: f.drawio_url ? undefined : 'needs a draw.io server' }]} />
        </SettingRow>
      </Section>
      <Section title="Sketches" icon="pen" sub="The hand-drawn whiteboard (Excalidraw).">
        <SettingRow title="Text in" help="For new text; each text can be changed in the sketch."><Segmented size="sm" value={f.sketch_font} onChange={v => setF({ ...f, sketch_font: v })} options={[{ value: 'hand', label: 'Handwriting' }, { value: 'normal', label: 'Normal' }, { value: 'code', label: 'Code' }]} /></SettingRow>
        <SettingRow title="Start with the grid on" help="Shapes line up on a grid in new sketches."><Switch checked={f.sketch_grid} onChange={v => setF({ ...f, sketch_grid: v })} label="Grid" /></SettingRow>
        <SettingRow title="Snap to other shapes" help="Shapes line up with each other while you move them."><Switch checked={f.sketch_snap} onChange={v => setF({ ...f, sketch_snap: v })} label="Snap" /></SettingRow>
      </Section>
      <Section title="Text diagrams" icon="code" sub="Diagrams written as text (Mermaid).">
        <SettingRow title="Colours" help="“Like the app” is light or dark with the app; pictures in the list are always light.">
          <Select ariaLabel="Diagram colours" value={f.mermaid_theme} onChange={v => setF({ ...f, mermaid_theme: v as SettingsT['mermaid_theme'] })} options={[
            { value: 'auto', label: 'Like the app' }, { value: 'default', label: 'Classic' }, { value: 'neutral', label: 'Grey (for printing)' }, { value: 'forest', label: 'Green' }, { value: 'base', label: 'Plain' }, { value: 'dark', label: 'Dark' }]} />
        </SettingRow>
        <SettingRow title="Look"><Segmented size="sm" value={f.mermaid_look} onChange={v => setF({ ...f, mermaid_look: v })} options={[{ value: 'classic', label: 'Neat' }, { value: 'handDrawn', label: 'Hand-drawn' }]} /></SettingRow>
      </Section>
      <Section title="Trash and history" icon="history">
        <SettingRow title="Keep deleted drawings for"><Select ariaLabel="Trash" value={String(f.trash_days)} onChange={v => setF({ ...f, trash_days: Number(v) })} options={opt(TRASH, f.trash_days, 'days')} /></SettingRow>
        <SettingRow title="Earlier versions per drawing" help="One is kept every ten minutes or so while you draw; the oldest go first."><Select ariaLabel="Versions" value={String(f.keep_versions)} onChange={v => setF({ ...f, keep_versions: Number(v) })} options={opt(VERSIONS, f.keep_versions, '')} /></SettingRow>
      </Section>
      <Section title="Dates" icon="calendar">
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
      </Section>
      {isAdmin && <Section title="This box" icon="server" sub="For everyone on this box — admins only.">
        <SettingRow title="App title" help="Shown in the sidebar."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>
        <SettingRow title="draw.io server" stack help={<>Diagrams open in draw.io: the address of your own <code>jgraph/drawio</code> container, or <code>https://embed.diagrams.net</code>. Empty: diagrams are greyed out. The diagrams themselves are always kept here.</>}>
          <input value={f.drawio_url} onChange={e => setF({ ...f, drawio_url: e.target.value })} placeholder="https://drawio.example.com" />
        </SettingRow>
      </Section>}
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where notifications go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/draw" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to send" icon="send">
        <SettingRow title="Another app added a drawing" help="Pocket AI drew a diagram for you, a project added a flowchart."><Switch checked={f.notify_link_adds} onChange={v => setF({ ...f, notify_link_adds: v })} label="Added by apps" /></SettingRow>
        <SettingRow title="Copies are not reaching Pocket Files" help="At most once a day, when keeping copies up to date fails."><Switch checked={f.notify_files_errors} onChange={v => setF({ ...f, notify_files_errors: v })} label="Files trouble" /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        {status('notify')}
      </div>
    </div>
  )
}

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Through <b>Pocket Core</b> Draw acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      <Section title="Pocket Files — a copy of every drawing" icon="layers" actions={connBadge(eff(f.files_mode), f.files_via)}
        sub="Each drawing becomes two files in a folder of yours: its picture (.svg) and the file Excalidraw, draw.io or Mermaid opens. Handy to find drawings among your files, share them, and have them in your file backups. The drawing itself stays here.">
        <Connection app="Pocket Files" mode={f.files_mode} via={f.files_via} detail={f.files_detail} core={coreOn} hasToken={!!f.files_token}
          onMode={m => save({ files_mode: m })} onDisconnect={() => save({ files_url: '', files_token: '' })}
          test={<><button type="button" className="btn sm" onClick={() => run('files', f.files_url, f.files_token)}>Test connection</button>{status('files')}</>}
          direct={<div className="two-col tight">
            <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f.files_url} onChange={e => setF({ ...f, files_url: e.target.value })} placeholder="http://host.docker.internal:8187" /></Field>
            <Field label="API token" help="Pocket Files → Settings → API tokens"><input value={f.files_token} onChange={e => setF({ ...f, files_token: e.target.value })} autoComplete="off" /></Field>
          </div>} />
        {eff(f.files_mode) !== 'off' && <div className="mt">
          <SettingRow title="Folder in Pocket Files" help="Made when missing; each Draw folder becomes a folder inside it."><input value={f.files_folder} onChange={e => setF({ ...f, files_folder: e.target.value })} placeholder="Drawings" className="set-short" /></SettingRow>
          <SettingRow title="Keep the copies up to date" help="A couple of minutes after you stop drawing, the copy follows. Off: use “Copy to Pocket Files” in a drawing’s menu."><Switch checked={f.files_auto} onChange={v => setF({ ...f, files_auto: v })} label="Automatic" /></SettingRow>
        </div>}
      </Section>
      <Section title="Other Pocket apps" icon="layers" sub="Draw offers your drawings to the rest of the box through Pocket Core. Each app asks for it and you switch it on in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>drawings:read</dt><dd className="muted">See your drawings and their pictures, and what you drew on a day (Pocket Day can write it into your journal).</dd></div>
          <div><dt>drawings:write</dt><dd className="muted">Add drawings for you — a diagram Pocket AI wrote, a flowchart from a project. An app may change or remove only what it added.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="Drafts a text diagram from a sentence (New drawing → describe it), changes one as you ask, and fixes a diagram with a mistake in it. Only what you type and that diagram’s text are sent, and only when you ask.">
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <Section title="Options" icon="sliders">
        <SettingRow title="Language of the labels" help="Empty: the language you write in."><input value={f.ai_language} onChange={e => setF({ ...f, ai_language: e.target.value })} placeholder="English" className="set-short" /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Draw does not talk to ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own drawings, folders and connections. Admins manage accounts; they do not see anyone else’s drawings.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their drawings." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent | null) => {
    e?.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (err) { toast((err as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>A token acts as you: it reads and changes all your drawings. Use it in <code>Authorization: Bearer &lt;token&gt;</code>, for a script, or a Homepage widget (<code>GET /api/v1/dashboard</code>).</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage widget)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Add a diagram from a script" icon="code" sub="Anything that can send a request can add a drawing — a text diagram from a build, a report, a home-automation rule.">
        <pre className="mono small set-pre">{`curl -X POST ${base}/api/v1/drawings \\\n  -H "Authorization: Bearer pdw_…" \\\n  -d '{"kind":"mermaid","title":"Deploy","folder":"Work","data":"flowchart LR\\n  build --> test --> ship"}'`}</pre>
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>; see <code>docs/api.md</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /drawings</code></dt><dd className="muted"><code>view=</code> recent, starred, trash · <code>kind=</code> sketch, mermaid, drawio · <code>folder=</code> · <code>q=</code> words in the name or on the drawing.</dd></div>
          <div><dt><code>GET /drawings/:id</code></dt><dd className="muted">One drawing with its content; <code>/preview.svg</code> its picture, <code>/file</code> the .excalidraw / .mmd / .drawio file.</dd></div>
          <div><dt><code>POST /drawings</code></dt><dd className="muted"><code>{'{"kind","title","data","folder"}'}</code> — add one; <code>PUT</code> changes it (send <code>rev</code> with <code>data</code>).</dd></div>
          <div><dt><code>POST /import</code></dt><dd className="muted">A file as the body (<code>?name=plan.drawio</code>): .excalidraw, .excalidrawlib, .drawio, .mmd, Markdown, a Mermaid Live link.</dd></div>
          <div><dt><code>GET /dashboard</code></dt><dd className="muted">Numbers per kind, folders, recently opened — for Homepage.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">What you drew and changed that day — what Pocket Day reads.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const [importing, setImporting] = useState(false)
  return (
    <div className="stack loose">
      <Section title="Import" icon="upload" sub="Moving from another app? Draw recognises the file by itself, and you can choose many files at once. Importing a Pocket Draw export twice adds nothing.">
        <ul className="set-list small">
          <li><b>Excalidraw</b> — Menu → Save to… → Save to disk gives a <code>.excalidraw</code> file per drawing; Library → Export gives your shapes (<code>.excalidrawlib</code>).</li>
          <li><b>draw.io / diagrams.net</b> — File → Save as → Device (<code>.drawio</code>, or <code>.xml</code>).</li>
          <li><b>Mermaid Live</b> — paste the link from the browser bar, or save the code as <code>.mmd</code>.</li>
          <li><b>Notes in Markdown</b> — every <code>```mermaid</code> block becomes a text diagram.</li>
          <li><b>Pocket Draw</b> — a JSON export from another box.</li>
        </ul>
        <div className="row wrap mt-s"><button type="button" className="btn primary" onClick={() => setImporting(true)}><Icon name="upload" size={15} />Import files…</button></div>
        {importing && <ImportDialog files={[]} folderId={null} onClose={() => setImporting(false)} />}
      </Section>
      <Section title="Export" icon="download" sub="Everything of yours: every drawing with its content and picture, your folders and your shapes library, as one JSON file; or a list of your drawings as CSV. A single drawing downloads from its menu as a picture or its own file.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Everything (JSON)</a><a className="btn" href={`${V1}/export.csv`}><Icon name="list" size={15} />List of drawings (CSV)</a></div>
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-draw.db</code> in the data directory.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="download" size={15} />Download database</a>
      </Section>}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Drawings', counts.drawings, 'pen'], ['Folders', counts.folders, 'folder'], ['Versions', counts.versions, 'history'], ['Tokens', counts.api_tokens, 'key']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Draw</h3>
          <p className="muted small">Sketches, text diagrams and draw.io diagrams in one place, each kept with its history: a hand-drawn whiteboard (Excalidraw), diagrams written as text (Mermaid) and detailed diagrams (draw.io). One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-draw" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Built on Excalidraw, Mermaid and draw.io. Part of the Pocket apps: Pocket Core connects them — Files keeps copies, Day can write what you drew into your journal, Pocket AI drafts diagrams.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}
