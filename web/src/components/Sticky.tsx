// Sticky notes, the way Miro has them: one tap (or N) puts a coloured square note in the middle of the view and the
// typing starts at once. With a note selected, the next one goes right next to it in the same colour; a colour picked
// with notes selected recolours them. A note is a plain Excalidraw rectangle with its text inside (customData.sticky),
// so it opens, exports and travels like any other shape.
import { useEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { CaptureUpdateAction, convertToExcalidrawElements, newElementWith } from '@excalidraw/excalidraw'
import type { ExcalidrawImperativeAPI } from '@excalidraw/excalidraw/types'

// Miro's sticky colours (the default yellow, blue, violet and pink taken from Miro itself)
export const STICKY_COLOURS: { name: string; value: string }[] = [
  { name: 'Yellow', value: '#ffdd33' },
  { name: 'Light yellow', value: '#fff6b6' },
  { name: 'Orange', value: '#ffb26b' },
  { name: 'Red', value: '#ff9a92' },
  { name: 'Pink', value: '#f7a7ff' },
  { name: 'Violet', value: '#aba4ff' },
  { name: 'Blue', value: '#85b8ff' },
  { name: 'Light blue', value: '#c6dcff' },
  { name: 'Teal', value: '#7fdbcf' },
  { name: 'Green', value: '#a6e3a0' },
  { name: 'Light green', value: '#d5f692' },
  { name: 'Grey', value: '#e6e6e6' },
]
const COLOUR_KEY = 'pdw-sticky'
const SIZE = 200
const GAP = 24
// the text: Miro's clean sans, as big as fits inside a 10% margin, never bigger than 16% of the note
const FONT = 2 // Excalidraw's Helvetica
const LINE_HEIGHT = 1.15
const PAD = 0.1
const MAX_FONT = 0.16
const MIN_FONT = 6
const TEXT_COLOUR = '#1e1e1e'

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
    roundness: null, customData: { sticky: true },
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

let measurer: CanvasRenderingContext2D | null = null
const fontString = (size: number) => `${size}px Helvetica, sans-serif, Segoe UI Emoji`

/** Greedy word wrap; a word wider than the line is broken (said, so a bigger size can be refused for it). */
function wrap(text: string, size: number, maxW: number): { lines: string[]; width: number; broke: boolean } {
  measurer ||= document.createElement('canvas').getContext('2d')
  const m = measurer!
  m.font = fontString(size)
  const w = (t: string) => m.measureText(t).width
  const lines: string[] = []
  let broke = false
  for (const para of text.split('\n')) {
    let line = ''
    for (const word of para.split(/ +/)) {
      const tryLine = line ? `${line} ${word}` : word
      if (w(tryLine) <= maxW) { line = tryLine; continue }
      if (line) lines.push(line)
      if (w(word) <= maxW) { line = word; continue }
      broke = true // character by character
      line = ''
      for (const ch of word) { if (line && w(line + ch) > maxW) { lines.push(line); line = ch } else line += ch }
    }
    lines.push(line)
  }
  return { lines, width: Math.max(0, ...lines.map(w)), broke }
}

/** The biggest size the text fits the note at, Miro style: few words big, many words small. */
function layout(text: string, side: number) {
  const inner = side * (1 - 2 * PAD)
  let best = null as null | { size: number; lines: string[]; width: number; height: number }
  for (let size = Math.max(MIN_FONT, Math.floor(side * MAX_FONT)); size >= MIN_FONT; size--) {
    const r = wrap(text, size, inner)
    const height = r.lines.length * size * LINE_HEIGHT
    best = { size, lines: r.lines, width: r.width, height }
    if (!r.broke && r.width <= inner && height <= inner) break
  }
  return best!
}

const fitted = new Map<string, string>()
const fitKey = (c: El, t: El | undefined, editingId?: string) => t && t.type === 'text'
  ? `${t.id === editingId}|${c.width}|${c.height}|${c.x}|${c.y}|${t.originalText}|${t.text}|${t.fontSize}|${t.fontFamily}|${t.x}|${t.y}|${t.isDeleted}`
  : `${c.width}|${c.height}`

/** Keeps the notes this person is working on (selected, or being typed into) square with their text fitted. Only
 *  those: notes changed by others in a live session are fitted on their screens, so screens never argue. */
export function fitStickies(api: Api): void {
  const st = api.getAppState()
  const editing = st.editingTextElement as (El & { containerId?: string | null }) | null
  const all = api.getSceneElementsIncludingDeleted()
  const byId = new Map(all.map(e => [e.id, e]))
  const notes = all.filter(e => !e.isDeleted && isSticky(e) && (st.selectedElementIds[e.id] || editing?.containerId === e.id))
  if (!notes.length) return
  const changes = new Map<string, El>()
  for (const c of notes) {
    const tid = c.boundElements?.find(b => b.type === 'text')?.id
    const t = tid ? byId.get(tid) : undefined
    if (fitted.get(c.id) === fitKey(c, t, editing?.id)) continue
    const side = c.width
    let nc: El = c
    if (c.height !== side || c.roundness) nc = newElementWith(c, { height: side, roundness: null })
    if (t && t.type === 'text' && !t.isDeleted) {
      const live = editing?.id === t.id
      const text = t.originalText
      const l = layout(text, side)
      const h = Math.max(side, l.height / (1 - 2 * PAD))
      if (nc.height !== h) nc = newElementWith(nc, { height: h })
      const want = {
        fontFamily: FONT, fontSize: l.size, lineHeight: LINE_HEIGHT as never, strokeColor: TEXT_COLOUR,
        textAlign: 'center' as const, verticalAlign: 'middle' as const,
        // while typing, Excalidraw lays the text out itself on every key; only the size and font are set then
        ...(live ? {} : { text: l.lines.join('\n'), width: l.width, height: l.height, x: nc.x + (side - l.width) / 2, y: nc.y + (h - l.height) / 2 }),
      }
      const same = (Object.keys(want) as (keyof typeof want)[]).every(k => {
        const a = (t as never)[k] as unknown, b = want[k] as unknown
        return typeof a === 'number' && typeof b === 'number' ? Math.abs(a - b) < 0.01 : a === b
      })
      if (!same) changes.set(t.id, newElementWith(t, want as never))
    }
    if (nc !== c) changes.set(c.id, nc)
  }
  if (changes.size) {
    api.updateScene({ elements: all.map(e => changes.get(e.id) || e), captureUpdate: CaptureUpdateAction.EVENTUALLY })
  }
  // remember what each note looks like once fitted, so it is measured again only when something about it changes
  const now = new Map(api.getSceneElementsIncludingDeleted().map(e => [e.id, e]))
  for (const c of notes) {
    const nc = now.get(c.id)!
    const tid = nc.boundElements?.find(b => b.type === 'text')?.id
    fitted.set(c.id, fitKey(nc, tid ? now.get(tid) : undefined, editing?.id))
  }
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
