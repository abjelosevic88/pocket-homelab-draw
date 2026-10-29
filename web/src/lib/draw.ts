// What every page needs to know about drawings: the kinds, pictures, Mermaid and Excalidraw rendering.
import { useEffect, useRef } from 'react'
import { api, V1 } from './api'
import type { Drawing, Kind, Settings } from './types'
import { ago, fmtDate } from './format'

export const KINDS: Record<Kind, { label: string; plural: string; icon: string; blurb: string; what: string }> = {
  sketch: { label: 'Sketch', plural: 'Sketches', icon: 'pen', blurb: 'A hand-drawn whiteboard: boxes, arrows, notes and pictures.', what: 'Excalidraw' },
  mermaid: { label: 'Text diagram', plural: 'Text diagrams', icon: 'code', blurb: 'Write a few lines, see a flowchart, timeline or mind map drawn.', what: 'Mermaid' },
  drawio: { label: 'Diagram', plural: 'Diagrams', icon: 'shapes', blurb: 'Detailed flowcharts, floor plans and org charts with shape libraries.', what: 'draw.io' },
}
export const KIND_ORDER: Kind[] = ['sketch', 'mermaid', 'drawio']

/** Something changed (a drawing added, moved, trashed): lists and counts reload. */
export function announce() { window.dispatchEvent(new CustomEvent('pdw:changed')) }
export function useOnChange(cb: () => void) {
  const ref = useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pdw:changed', h)
    return () => window.removeEventListener('pdw:changed', h)
  }, [])
}

/** When, short: "3 h ago" this week, then the date (the year only when it is not this year). */
export function since(iso: string): string {
  const t = new Date(iso)
  if (Number.isNaN(t.getTime())) return ''
  if (Date.now() - t.getTime() < 7 * 86400_000) return ago(iso)
  const local = `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`
  return fmtDate(local, t.getFullYear() !== new Date().getFullYear())
}

export const previewURL = (d: Pick<Drawing, 'id' | 'preview_rev'>) => `${V1}/drawings/${d.id}/preview.svg?r=${d.preview_rev}`

export function isDark(): boolean { return document.documentElement.dataset.theme === 'dark' || (document.documentElement.dataset.theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches) }

// ---- Mermaid ----

type MermaidAPI = typeof import('mermaid')['default']
let mermaidP: Promise<MermaidAPI> | null = null
let queue: Promise<unknown> = Promise.resolve()
let seq = 0

function loadMermaid(): Promise<MermaidAPI> {
  if (!mermaidP) mermaidP = import('mermaid').then(m => m.default)
  return mermaidP
}

export interface MermaidLook { theme: Settings['mermaid_theme']; look: Settings['mermaid_look'] }

/** The Mermaid theme to draw with: "auto" follows the app, pictures for the list are always light. */
export function mermaidTheme(theme: Settings['mermaid_theme'] | undefined, forPicture: boolean): string {
  const t = theme || 'auto'
  if (forPicture) return t === 'auto' || t === 'dark' ? 'default' : t
  if (t === 'auto') return isDark() ? 'dark' : 'default'
  return t
}

/** Draws Mermaid source as an SVG string; throws with Mermaid's message when it cannot. One at a time. */
export function renderMermaid(code: string, look: MermaidLook, forPicture = false): Promise<string> {
  const run = async () => {
    const mermaid = await loadMermaid()
    mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: mermaidTheme(look.theme, forPicture) as 'default', look: look.look || 'classic', fontFamily: 'ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif' })
    const id = `mmd-${++seq}`
    try {
      const { svg } = await mermaid.render(id, code)
      return svg
    } finally {
      document.getElementById(id)?.remove()
      document.getElementById('d' + id)?.remove()
    }
  }
  const p = queue.then(run, run)
  queue = p.catch(() => {})
  return p
}

/** A friendlier first line of a Mermaid error. */
export function mermaidError(e: unknown): string {
  const m = ((e as Error)?.message || String(e)).replace(/\s+/g, ' ').trim()
  return m.length > 400 ? m.slice(0, 400) + '…' : m
}

/** A picture of a text diagram with a white background, for the list and Pocket Files. */
export async function mermaidPicture(code: string, look: MermaidLook): Promise<string> {
  return pictureSize(await renderMermaid(code, look, true))
}

/** Mermaid draws to fit the page (width="100%", a max-width): as a picture elsewhere (an <img> in a note) it
 *  needs its own size, else browsers show it at a small default. The size comes from its viewBox. */
export function pictureSize(svg: string): string {
  const open = svg.match(/<svg\b[^>]*>/)
  if (!open) return svg
  const vb = open[0].match(/viewBox="\s*[-\d.]+[\s,]+[-\d.]+[\s,]+([\d.]+)[\s,]+([\d.]+)\s*"/)
  let tag = open[0].replace(/\s(width|height)="[^"]*"/g, '')
  tag = tag.replace(/style="([^"]*)"/, (_m, st: string) => `style="${st.replace(/max-width:[^;]*;?/g, '')};background-color:#ffffff"`)
  if (!/style="/.test(tag)) tag = tag.replace(/^<svg/, '<svg style="background-color:#ffffff"')
  if (vb) tag = tag.replace(/^<svg/, `<svg width="${Math.ceil(Number(vb[1]))}" height="${Math.ceil(Number(vb[2]))}"`)
  return svg.replace(open[0], tag)
}

// ---- Excalidraw ----

type ExcalidrawModule = typeof import('@excalidraw/excalidraw')
let excaliP: Promise<ExcalidrawModule> | null = null
export function loadExcalidraw(): Promise<ExcalidrawModule> {
  if (!excaliP) excaliP = import('@excalidraw/excalidraw')
  return excaliP
}

/** A picture of a sketch (light, with its background) from its saved scene. */
export async function sketchPicture(data: string): Promise<string> {
  const ex = await loadExcalidraw()
  const parsed = JSON.parse(data || '{}')
  const scene = ex.restore({ elements: parsed.elements || [], appState: parsed.appState || {}, files: parsed.files || {} }, null, null)
  const live = scene.elements.filter(e => !e.isDeleted)
  if (live.length === 0) return blankPicture()
  const svg = await ex.exportToSvg({
    elements: live, files: scene.files,
    appState: { ...scene.appState, exportBackground: true, exportWithDarkMode: false, exportScale: 1, exportPadding: 16 },
  })
  return new XMLSerializer().serializeToString(svg)
}

export function blankPicture(): string {
  return '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 300" width="400" height="300"><rect width="400" height="300" fill="#ffffff"/></svg>'
}

/** Turns a data: URL from draw.io's export into the SVG text. */
export function dataUrlText(url: string): string {
  const i = url.indexOf(',')
  if (i < 0) return ''
  const head = url.slice(0, i)
  const body = url.slice(i + 1)
  if (head.includes(';base64')) {
    const bin = atob(body)
    const bytes = Uint8Array.from(bin, c => c.charCodeAt(0))
    return new TextDecoder().decode(bytes)
  }
  return decodeURIComponent(body)
}

/** PNG from an SVG string, drawn at scale in the browser. */
export async function svgToPng(svg: string, scale = 2): Promise<Blob> {
  const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }))
  try {
    const img = new Image()
    await new Promise<void>((res, rej) => { img.onload = () => res(); img.onerror = () => rej(new Error('the picture could not be drawn')); img.src = url })
    const w = Math.max(1, Math.round((img.naturalWidth || 800) * scale)), h = Math.max(1, Math.round((img.naturalHeight || 600) * scale))
    const c = document.createElement('canvas')
    c.width = w; c.height = h
    const g = c.getContext('2d')!
    g.fillStyle = '#ffffff'; g.fillRect(0, 0, w, h)
    g.drawImage(img, 0, 0, w, h)
    return await new Promise<Blob>((res, rej) => c.toBlob(b => b ? res(b) : rej(new Error('PNG failed')), 'image/png'))
  } finally { URL.revokeObjectURL(url) }
}

export function saveBlob(blob: Blob, name: string) {
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(a.href), 5000)
}

export function fileBase(title: string): string {
  return (title || 'drawing').replace(/[/\\:*?"<>|]+/g, '-').replace(/\s+/g, ' ').trim().slice(0, 100) || 'drawing'
}

// ---- pictures for the list, made in the browser when missing ----

const tried = new Set<string>()
let working = false

/** Draws the missing or old pictures of sketches and text diagrams, one at a time, and keeps them. */
export async function fillPreviews(list: Drawing[], look: MermaidLook, onDone: (id: number, rev: number) => void) {
  const due = list.filter(d => d.kind !== 'drawio' && !d.trashed_at && (!d.has_preview || d.preview_rev < d.rev) && !tried.has(`${d.id}:${d.rev}`))
  if (working || due.length === 0) return
  working = true
  try {
    for (const d of due.slice(0, 40)) {
      tried.add(`${d.id}:${d.rev}`)
      try {
        const full = await api.get<Drawing>(`${V1}/drawings/${d.id}`)
        const svg = full.kind === 'mermaid' ? await mermaidPicture(full.data || '', look) : await sketchPicture(full.data || '')
        await api.put(`${V1}/drawings/${d.id}`, { preview: svg, preview_rev: full.rev })
        onDone(d.id, full.rev)
      } catch { /* a diagram with a mistake keeps its placeholder */ }
    }
  } finally { working = false }
}

// ---- templates for text diagrams ----

export const TEMPLATES: { id: string; label: string; icon: string; code: string }[] = [
  { id: 'flowchart', label: 'Flowchart', icon: 'route', code: 'flowchart TD\n    A[Start] --> B{Is it ready?}\n    B -- Yes --> C[Ship it]\n    B -- No --> D[Fix it]\n    D --> B\n' },
  { id: 'sequence', label: 'Who talks to whom', icon: 'users', code: 'sequenceDiagram\n    participant You\n    participant Shop\n    participant Bank\n    You->>Shop: Order\n    Shop->>Bank: Charge the card\n    Bank-->>Shop: Paid\n    Shop-->>You: On its way\n' },
  { id: 'mindmap', label: 'Mind map', icon: 'brain', code: 'mindmap\n  root((Holiday))\n    Where\n      Coast\n      Mountains\n    When\n      July\n    Who\n      Family\n' },
  { id: 'timeline', label: 'Timeline', icon: 'clock', code: 'timeline\n    title This year\n    Spring : Plan the garden\n    Summer : Holiday\n    Autumn : Move house\n    Winter : Rest\n' },
  { id: 'gantt', label: 'Plan with dates', icon: 'calendar', code: 'gantt\n    title A small project\n    dateFormat YYYY-MM-DD\n    section Prepare\n    Gather ideas   :a1, 2026-11-02, 5d\n    Decide         :a2, after a1, 2d\n    section Do\n    Build          :b1, after a2, 10d\n    Celebrate      :b2, after b1, 1d\n' },
  { id: 'pie', label: 'Pie chart', icon: 'chart', code: 'pie showData\n    title Where the time goes\n    "Work" : 40\n    "Sleep" : 35\n    "Family" : 15\n    "Other" : 10\n' },
  { id: 'state', label: 'States', icon: 'repeat', code: 'stateDiagram-v2\n    [*] --> Idea\n    Idea --> Doing : start\n    Doing --> Done : finish\n    Doing --> Idea : pause\n    Done --> [*]\n' },
  { id: 'er', label: 'Data model', icon: 'database', code: 'erDiagram\n    PERSON ||--o{ ORDER : places\n    ORDER ||--|{ LINE : contains\n    PRODUCT ||--o{ LINE : "is in"\n' },
  { id: 'journey', label: 'A journey', icon: 'footprints', code: 'journey\n    title Saturday morning\n    section Out\n      Coffee: 5: Me\n      Market: 4: Me, Partner\n    section Home\n      Cook lunch: 3: Partner\n' },
  { id: 'quadrant', label: 'Four squares', icon: 'grid', code: 'quadrantChart\n    title What to do first\n    x-axis Little effort --> Lots of effort\n    y-axis Little value --> Lots of value\n    quadrant-1 Plan it\n    quadrant-2 Do it now\n    quadrant-3 Maybe later\n    quadrant-4 Skip it\n    Fix the tap: [0.2, 0.8]\n    New kitchen: [0.9, 0.7]\n' },
]
