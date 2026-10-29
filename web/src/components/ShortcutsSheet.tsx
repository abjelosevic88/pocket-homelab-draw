import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['1'], 'All drawings'], [['2'], 'Recent'], [['3'], 'Starred'], [[','], 'Settings'], [['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Make', keys: [[['n'], 'New drawing'], [['s'], 'New sketch'], [['d'], 'New text diagram'], [['i'], 'Import files']] },
  { title: 'In a sketch', keys: [[['r'], 'Rectangle'], [['o'], 'Ellipse'], [['a'], 'Arrow'], [['t'], 'Text'], [['p'], 'Pen'], [['⌘', 'Z'], 'Undo'], [['?'], 'All sketch keys (in the sketch)']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close a dialog or menu']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}
