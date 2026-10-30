// Share a drawing with other people on this box: each can edit or only view. Sketches open live for everyone.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { Drawing, Person, Share } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Avatar, Loading, Modal, Segmented, useToast } from './ui'

type Role = Share['role']
const ROLES: { value: Role; label: string }[] = [{ value: 'edit', label: 'Can edit' }, { value: 'view', label: 'Can view' }]

export function ShareDialog({ drawing, onClose, onCount }: { drawing: Drawing; onClose: () => void; onCount: (n: number) => void }) {
  const { toast } = useToast()
  const [shares, setShares] = useState<Share[] | null>(null)
  const [people, setPeople] = useState<Person[]>([])
  const [pick, setPick] = useState('')
  const [role, setRole] = useState<Role>('edit')
  const base = `${V1}/drawings/${drawing.id}/shares`

  const load = useCallback(async () => {
    try {
      const l = await api.get<Share[]>(base)
      setShares(l)
      onCount(l.length)
    } catch (e) { toast((e as Error).message, 'err') }
  }, [base, onCount, toast])
  useEffect(() => {
    void load()
    api.get<Person[]>(`${V1}/people`).then(setPeople).catch(() => setPeople([]))
  }, [load])

  const options = useMemo(() => people.filter(p => !shares?.some(s => s.user_id === p.id))
    .map(p => ({ value: String(p.id), label: p.name || p.email, hint: p.name ? p.email : undefined })), [people, shares])

  const set = async (uid: number, r: Role) => {
    try { await api.put(`${base}/${uid}`, { role: r }); await load() } catch (e) { toast((e as Error).message, 'err') }
  }
  const add = async () => { if (!pick) return; await set(Number(pick), role); setPick('') }
  const remove = async (s: Share) => {
    try { await api.del(`${base}/${s.user_id}`); await load() } catch (e) { toast((e as Error).message, 'err') }
  }

  return (
    <Modal title={`Share “${drawing.title}”`} onClose={onClose}>
      <p className="muted small">
        People on this Pocket Draw find it under <b>Shared with me</b>.{' '}
        {drawing.kind === 'sketch'
          ? 'Everyone who has it open sees the others’ pointers and changes as they happen.'
          : 'Live pointers and changes are for sketches; they open this one as it was last saved.'}
      </p>
      {people.length === 0 && shares?.length === 0 ? (
        <p className="muted">No one else has signed in to this Pocket Draw yet. Once they have, you can share with them here.</p>
      ) : (
        <div className="share-add">
          <Select className="grow" ariaLabel="Person" value={pick} onChange={setPick} options={options} placeholder={options.length ? 'Choose a person…' : 'Shared with everyone here'} disabled={!options.length} />
          <Segmented size="sm" label="Role" value={role} onChange={setRole} options={ROLES} />
          <button type="button" className="btn primary sm" disabled={!pick} onClick={add}>Share</button>
        </div>
      )}
      {!shares ? <Loading /> : shares.length > 0 && (
        <ul className="share-list" aria-label="Shared with">
          {shares.map(s => (
            <li key={s.user_id}>
              <Avatar name={s.name} email={s.email} size={28} />
              <span className="grow ellipsis"><b>{s.name || s.email}</b>{s.name && <span className="muted small"> · {s.email}</span>}{s.here && <span className="share-here"> · has it open</span>}</span>
              <Segmented size="sm" label={`What ${s.name || s.email} may do`} value={s.role} onChange={r => void set(s.user_id, r)} options={ROLES} />
              <button type="button" className="btn ghost icon-only sm" onClick={() => void remove(s)} aria-label={`Stop sharing with ${s.name || s.email}`} title="Stop sharing"><Icon name="x" size={15} /></button>
            </li>
          ))}
        </ul>
      )}
      <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Done</button></div>
    </Modal>
  )
}
