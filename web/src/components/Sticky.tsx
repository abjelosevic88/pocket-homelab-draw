// Sticky notes, the way Miro has them: one tap (or N) puts a coloured square note in the middle of the view and the
// typing starts at once. With a note selected, the next one goes right next to it in the same colour; a colour picked
// with notes selected recolours them. A note is a plain Excalidraw rectangle with its text inside (customData.sticky),
// so it opens, exports and travels like any other shape.
import { useEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { CaptureUpdateAction, convertToExcalidrawElements, newElementWith } from '@excalidraw/excalidraw'
import type { ExcalidrawImperativeAPI } from '@excalidraw/excalidraw/types'

export const STICKY_COLOURS: { name: string; value: string }[] = [
  { name: 'Yellow', value: '#fff9b1' },
  { name: 'Orange', value: '#ffcf99' },
  { name: 'Pink', value: '#ffcee0' },
  { name: 'Violet', value: '#dccbf5' },
  { name: 'Blue', value: '#b8d8fa' },
  { name: 'Teal', value: '#b5ebe5' },
  { name: 'Green', value: '#d5f692' },
  { name: 'Grey', value: '#e6e6e6' },
]
const COLOUR_KEY = 'pdw-sticky-colour'
const SIZE = 200
const GAP = 24

type Api = ExcalidrawImperativeAPI
type El = ReturnType<Api['getSceneElements']>[number]

const isSticky = (e: El) => e.type === 'rectangle' && !!(e.customData as { sticky?: boolean } | undefined)?.sticky
const selectedStickies = (api: Api) => {
  const ids = api.getAppState().selectedElementIds
  return api.getSceneElements().filter(e => ids[e.id] && isSticky(e))
}

/** Puts a new note in the scene and starts typing into it. */
export function addSticky(api: Api, host: HTMLElement | null, colour: string) {
  const st = api.getAppState()
  const all = api.getSceneElements()
  const next = selectedStickies(api)
  let x: number, y: number
  let bg = colour
  if (next.length === 1) { // next to the selected note, Miro's "+" handle
    const s = next[0]
    x = s.x + s.width + GAP; y = s.y; bg = s.backgroundColor
    while (all.some(e => isSticky(e) && Math.abs(e.x - x) < 8 && Math.abs(e.y - y) < 8)) x += SIZE + GAP
  } else { // the middle of the view, stepped down-right past notes already there
    const z = st.zoom.value
    x = Math.round(st.width / 2 / z - st.scrollX - SIZE / 2)
    y = Math.round(st.height / 2 / z - st.scrollY - SIZE / 2)
    while (all.some(e => isSticky(e) && Math.abs(e.x - x) < 8 && Math.abs(e.y - y) < 8)) { x += GAP; y += GAP }
  }
  const [note] = convertToExcalidrawElements([{
    type: 'rectangle', x, y, width: SIZE, height: SIZE,
    backgroundColor: bg, fillStyle: 'solid', strokeColor: 'transparent', strokeWidth: 1, roughness: 0,
    roundness: { type: 3 }, customData: { sticky: true },
  }])
  api.setActiveTool({ type: 'selection' })
  const z = st.zoom.value
  const inView = x + st.scrollX >= 0 && y + st.scrollY >= 0 && (x + SIZE + st.scrollX) * z <= st.width && (y + SIZE + st.scrollY) * z <= st.height
  flushSync(() => {
    api.updateScene({
      elements: [...api.getSceneElementsIncludingDeleted(), note],
      appState: { selectedElementIds: { [note.id]: true } } as never,
      captureUpdate: CaptureUpdateAction.IMMEDIATELY,
    })
    if (!inView) api.scrollToContent(note, { animate: false }) // a note beside one at the edge of a phone screen
  })
  // Excalidraw starts typing into a selected shape on Enter; doing it in the same tap keeps the phone keyboard up
  host?.querySelector('.excalidraw')?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
}

function recolour(api: Api, colour: string): boolean {
  const notes = selectedStickies(api)
  if (!notes.length) return false
  const ids = new Set(notes.map(n => n.id))
  api.updateScene({
    elements: api.getSceneElementsIncludingDeleted().map(e => ids.has(e.id) ? newElementWith(e, { backgroundColor: colour }) : e),
    captureUpdate: CaptureUpdateAction.IMMEDIATELY,
  })
  return true
}

/** The note button with its colours, shown in Excalidraw's top bar. */
export function StickyTool({ api, host, mobile }: { api: Api | null; host: React.RefObject<HTMLDivElement | null>; mobile: boolean }) {
  const [colour, setColour] = useState(() => localStorage.getItem(COLOUR_KEY) || STICKY_COLOURS[0].value)
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const colourRef = useRef(colour)
  colourRef.current = colour

  useEffect(() => {
    if (!open) return
    const away = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('pointerdown', away, true)
    return () => document.removeEventListener('pointerdown', away, true)
  }, [open])

  // N, as in Miro: a new note (not while typing, not with a modifier)
  useEffect(() => {
    const el = host.current
    if (!el || !api) return
    const key = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() !== 'n' || e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return
      const t = e.target as HTMLElement
      if (t.closest('input, textarea, select, [contenteditable="true"]')) return
      if (api.getAppState().editingTextElement || api.getAppState().viewModeEnabled) return
      e.preventDefault(); e.stopPropagation()
      addSticky(api, el, colourRef.current)
    }
    el.addEventListener('keydown', key, true)
    return () => el.removeEventListener('keydown', key, true)
  }, [api, host])

  const pick = (c: string) => {
    setColour(c); localStorage.setItem(COLOUR_KEY, c); setOpen(false)
    if (api) recolour(api, c)
  }

  return (
    <div className={`sticky-tool${mobile ? ' mobile' : ''}`} ref={box}>
      <button type="button" className="sticky-add" title="Sticky note — N" aria-label="Sticky note"
        onClick={() => { if (api) addSticky(api, host.current, colour); setOpen(false) }}>
        <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
          <path d="M4 4h16v10l-6 6H4z" fill={colour} stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round" />
          <path d="M14 20v-6h6" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round" />
        </svg>
      </button>
      <button type="button" className="sticky-more" title="Note colour" aria-label="Note colour" aria-expanded={open} onClick={() => setOpen(o => !o)}>
        <svg viewBox="0 0 24 24" width="12" height="12" aria-hidden="true"><path d="M6 9l6 6 6-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
      </button>
      {open && (
        <div className="sticky-colours" role="menu">
          {STICKY_COLOURS.map(c => (
            <button key={c.value} type="button" role="menuitemradio" aria-checked={c.value === colour} title={c.name} aria-label={c.name}
              className={c.value === colour ? 'on' : ''} style={{ background: c.value }} onClick={() => pick(c.value)} />
          ))}
        </div>
      )}
    </div>
  )
}
