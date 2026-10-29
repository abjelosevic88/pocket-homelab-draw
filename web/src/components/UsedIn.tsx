// "Used in": the notes, issues and projects that show this drawing, asked of Pocket Notes and Pocket Projects;
// and how to put it into one (a new note, or the picture's Markdown for anywhere).
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { since } from '../lib/draw'
import type { Drawing } from '../lib/types'
import { Icon } from './icons'
import { Loading, Modal, copyText, useToast } from './ui'

interface UsedItem { kind: 'note' | 'issue' | 'project'; title: string; detail: string; url: string; modified: string }
interface UsedSource { app: string; name: string; connected: boolean; detail?: string; items: UsedItem[]; open_url?: string }
interface UsedAnswer { page_url: string; picture_url: string; markdown: string; detail?: string; sources: UsedSource[] }

const ICON: Record<UsedItem['kind'], string> = { note: 'note', issue: 'check-square', project: 'layers' }

export function UsedIn({ drawing, onClose }: { drawing: Drawing; onClose: () => void }) {
  const { toast } = useToast()
  const [data, setData] = useState<UsedAnswer | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<UsedAnswer>(`${V1}/drawings/${drawing.id}/used`).then(setData).catch(e => setErr((e as Error).message)) }, [drawing.id])
  const copy = async (text: string, what: string) => { if (await copyText(text)) toast(`${what} copied`); else toast('Could not copy', 'err') }
  const notes = data?.sources.find(s => s.app === 'notes')
  const total = data?.sources.reduce((n, s) => n + s.items.length, 0) ?? 0
  return (
    <Modal title="Used in" onClose={onClose}>
      {err ? <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div> : !data ? <Loading /> : (
        <div className="stack">
          <p className="muted small">Notes, issues and projects that show this drawing. They show it as it is now: change the drawing here and they follow.</p>
          {data.sources.map(src => (
            <section key={src.app} className="used-src">
              <h4 className="used-h">{src.name}</h4>
              {!src.connected ? (
                <p className="muted small">Not connected{src.detail ? `: ${src.detail}` : ''}. <Link to="/settings/apps" onClick={onClose}>Settings → Connected apps</Link></p>
              ) : src.items.length === 0 ? (
                <p className="muted small">{src.detail || (src.app === 'notes' ? 'No note shows it yet.' : 'No issue or project shows it yet.')}</p>
              ) : (
                <ul className="used-list">
                  {src.items.map((it, i) => (
                    <li key={i}>
                      <a href={it.url || undefined} target="_blank" rel="noreferrer" className="used-row">
                        <Icon name={ICON[it.kind]} size={15} />
                        <span className="grow ellipsis"><b>{it.title}</b>{it.detail && <span className="muted small"> · {it.detail}</span>}</span>
                        {it.modified && <span className="muted xsmall">{since(it.modified)}</span>}
                        <Icon name="external" size={13} className="muted" />
                      </a>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          ))}
          {data.detail && <p className="muted small">{data.detail}</p>}
          <div className="used-put">
            <h4 className="used-h">Put it somewhere</h4>
            <div className="row wrap">
              {notes?.open_url && <a className="btn primary sm" href={notes.open_url} target="_blank" rel="noreferrer"><Icon name="note" size={14} />A new note with this drawing</a>}
              {data.markdown && <button type="button" className="btn sm" onClick={() => copy(data.markdown, 'The picture for a note or an issue')}><Icon name="copy" size={14} />Copy for a note or an issue</button>}
              {data.page_url && <button type="button" className="btn ghost sm" onClick={() => copy(data.page_url, 'The link')}><Icon name="link" size={14} />Copy the link</button>}
            </div>
            <p className="muted xsmall">In Pocket Notes you can also type / → Drawing in any note; in Pocket Projects, the Drawing button under an issue’s text.{total === 0 ? '' : ` Shown in ${total} place${total === 1 ? '' : 's'} so far.`}</p>
          </div>
        </div>
      )}
    </Modal>
  )
}
