// A sketch: Excalidraw, kept in this app (fonts served from here, no AI calls out). Shared, it is live: the
// others' pointers and selections show, and changes travel through this app's own server as they are made.
import { useEffect, useMemo, useRef } from 'react'
import { CaptureUpdateAction, Excalidraw, MainMenu, WelcomeScreen, exportToSvg, getSceneVersion, reconcileElements, restore, restoreElements, serializeAsJSON } from '@excalidraw/excalidraw'
import '@excalidraw/excalidraw/index.css'
import type { BinaryFileData, BinaryFiles, Collaborator, ExcalidrawImperativeAPI, LibraryItems, SocketId } from '@excalidraw/excalidraw/types'
import type { RemoteExcalidrawElement } from '@excalidraw/excalidraw/data/reconcile'
import { api, V1 } from '../lib/api'
import { blankPicture } from '../lib/draw'
import type { LiveMessage, LivePeer, LiveSession } from '../lib/live'
import type { Settings } from '../lib/types'

const FONTS: Record<Settings['sketch_font'], number> = { hand: 5, normal: 6, code: 8 }

export interface EditorProps {
  data: string
  title: string
  dark: boolean
  settings: Settings
  /** Called on every real change: how to get the content and its picture when it is saved. */
  onChange: (getData: () => string, makePreview: () => Promise<string>) => void
  /** The saved picture is missing or older than the drawing: the editor hands one over once it has it. */
  onPicture?: (svg: string) => void
  /** Shared and open elsewhere too: the live session (sketches only). */
  live?: LiveSession | null
  /** Shared to view: nothing can be changed. */
  readOnly?: boolean
}

const SEND_EVERY = 60 // ms between batches of changes

/** Excalidraw colours a person's pointer by a hash of their id (hue = hash % 37 × 10): an id per account whose
 *  hue is 140° on from the previous account's, so people keep their colour and two people rarely look alike. */
function colourID(uid: number): string {
  const want = (uid * 14) % 37
  const hue = (id: string) => { let h = 0; for (let i = 0; i < id.length; i++) h = (h << 5) - h + id.charCodeAt(i); return Math.abs(h) % 37 }
  for (let n = 0; n < 400; n++) if (hue(`u${uid}.${n}`) === want) return `u${uid}.${n}`
  return `u${uid}`
}
const POINTER_EVERY = 40

export default function SketchEditor({ data, title, dark, settings, onChange, live, readOnly }: EditorProps) {
  const apiRef = useRef<ExcalidrawImperativeAPI | null>(null)
  const lastKey = useRef('')
  const libTimer = useRef(0)
  const libLoaded = useRef(false)
  // live: the version of each element the others have (sent or received), the pictures they have, their pointers
  const known = useRef(new Map<string, number>())
  const knownFiles = useRef(new Set<string>())
  const collabs = useRef(new Map<SocketId, Collaborator>())
  const sendTimer = useRef(0)
  const lastPointer = useRef(0)
  const paint = useRef(0)

  const initial = useMemo(() => {
    let parsed: { elements?: unknown[]; appState?: Record<string, unknown>; files?: Record<string, unknown> } = {}
    try { parsed = JSON.parse(data || '{}') } catch { /* a broken scene opens empty */ }
    const fresh = !parsed.elements || parsed.elements.length === 0
    const scene = restore({ elements: (parsed.elements || []) as never, appState: (parsed.appState || {}) as never, files: (parsed.files || {}) as never }, null, null)
    const appState = {
      ...scene.appState,
      currentItemFontFamily: FONTS[settings.sketch_font] ?? 5,
      objectsSnapModeEnabled: settings.sketch_snap,
      ...(fresh ? { gridModeEnabled: settings.sketch_grid } : {}),
    }
    for (const e of scene.elements) known.current.set(e.id, e.version)
    for (const id of Object.keys(scene.files || {})) knownFiles.current.add(id)
    return { elements: scene.elements, appState, files: scene.files, scrollToContent: true }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // live: send what changed here, take in what changed there, show who is where
  useEffect(() => {
    if (!live) return
    const showCollaborators = () => {
      window.cancelAnimationFrame(paint.current)
      paint.current = window.requestAnimationFrame(() => apiRef.current?.updateScene({ collaborators: new Map(collabs.current) }))
    }
    const person = (p: LivePeer): Collaborator => ({ id: colourID(p.user_id), socketId: p.id as SocketId, username: p.name })
    const meet = () => { // everyone here is listed, with no pointer until they move
      const ids = new Set(live.state.peers.map(p => p.id))
      for (const id of collabs.current.keys()) if (!ids.has(id)) collabs.current.delete(id)
      for (const p of live.state.peers) if (!collabs.current.has(p.id as SocketId)) collabs.current.set(p.id as SocketId, person(p))
      showCollaborators()
    }
    const sendAll = (to: string) => {
      const api = apiRef.current
      if (!api) return
      const files = api.getFiles()
      live.send({ t: 'scene', to, full: true, elements: api.getSceneElementsIncludingDeleted(), files })
    }
    const take = (m: LiveMessage) => {
      const api = apiRef.current
      if (!api) return
      const remote = restoreElements(m.elements || [], null) as unknown as RemoteExcalidrawElement[]
      for (const e of remote) known.current.set(e.id, Math.max(known.current.get(e.id) ?? 0, e.version))
      const files = Object.values((m.files || {}) as BinaryFiles).filter(f => !knownFiles.current.has(f.id))
      if (files.length) { for (const f of files) knownFiles.current.add(f.id); api.addFiles(files as BinaryFileData[]) }
      if (!remote.length) return
      const elements = reconcileElements(api.getSceneElementsIncludingDeleted(), remote, api.getAppState())
      api.updateScene({ elements, captureUpdate: CaptureUpdateAction.NEVER })
    }
    const off = live.on(m => {
      switch (m.t) {
        case 'hello': meet(); sendChanges(); break // back after a break: what was drawn meanwhile goes out
        case 'join': case 'leave': meet(); break
        case 'sync': sendAll(m.to); break
        case 'scene': take(m); break
        case 'pointer': {
          const p = live.state.peers.find(x => x.id === m.from)
          if (!p) return
          collabs.current.set(p.id as SocketId, { ...person(p), pointer: m.pointer, button: m.button, selectedElementIds: m.selected || {} })
          showCollaborators()
          break
        }
      }
    })
    meet()
    return () => { off(); window.clearTimeout(sendTimer.current); window.cancelAnimationFrame(paint.current); collabs.current.clear(); apiRef.current?.updateScene({ collaborators: new Map() }) }
  }, [live])

  /** A moment after a change here, the elements (and pictures) the others do not have yet go to them. */
  const sendChanges = () => {
    if (!live || readOnly || sendTimer.current) return
    sendTimer.current = window.setTimeout(() => {
      sendTimer.current = 0
      const api = apiRef.current
      if (!api || !live.state.connected) return
      const changed = api.getSceneElementsIncludingDeleted().filter(e => (known.current.get(e.id) ?? -1) < e.version)
      const all = api.getFiles()
      const files: BinaryFiles = {}
      for (const id of Object.keys(all)) if (!knownFiles.current.has(id)) { files[id] = all[id]; knownFiles.current.add(id) }
      if (!changed.length && !Object.keys(files).length) return
      for (const e of changed) known.current.set(e.id, e.version)
      live.send({ t: 'scene', elements: changed, files })
    }, SEND_EVERY)
  }

  // the person's library of shapes lives on the server
  useEffect(() => {
    let stop = false
    api.get<{ items: LibraryItems }>(`${V1}/library`).then(r => {
      if (stop || !apiRef.current) return
      void apiRef.current.updateLibrary({ libraryItems: r.items, merge: false, openLibraryMenu: false })
      window.setTimeout(() => { libLoaded.current = true }, 500)
    }).catch(() => { libLoaded.current = true })
    return () => { stop = true }
  }, [])

  return (
    <div className="sketch-host">
      <Excalidraw
        excalidrawAPI={a => { apiRef.current = a }}
        initialData={initial as never}
        theme={dark ? 'dark' : 'light'}
        name={title}
        langCode="en"
        aiEnabled={false}
        viewModeEnabled={readOnly}
        isCollaborating={!!live}
        onPointerUpdate={live ? p => {
          const now = performance.now()
          if (now - lastPointer.current < POINTER_EVERY && p.button !== 'down') return
          lastPointer.current = now
          live.send({ t: 'pointer', pointer: p.pointer, button: p.button, selected: apiRef.current?.getAppState().selectedElementIds || {} })
        } : undefined}
        UIOptions={{ canvasActions: { loadScene: false, saveToActiveFile: false, toggleTheme: null, export: { saveFileToDisk: true }, saveAsImage: true } }}
        onChange={(elements, appState, files) => {
          sendChanges()
          const key = `${getSceneVersion(elements)}|${Object.keys(files).length}|${appState.viewBackgroundColor}|${appState.gridModeEnabled}`
          if (key === lastKey.current) return
          const first = lastKey.current === ''
          lastKey.current = key
          if (first) return // the scene just loaded
          onChange(
            () => serializeAsJSON(elements, appState, files, 'local'),
            async () => {
              const live = elements.filter(e => !e.isDeleted)
              if (live.length === 0) return blankPicture()
              const svg = await exportToSvg({ elements: live, files, appState: { ...appState, exportBackground: true, exportWithDarkMode: false, exportScale: 1, exportPadding: 16 } })
              return new XMLSerializer().serializeToString(svg)
            },
          )
        }}
        onLibraryChange={items => {
          if (!libLoaded.current) return
          window.clearTimeout(libTimer.current)
          libTimer.current = window.setTimeout(() => { void api.put(`${V1}/library`, { items }).catch(() => {}) }, 800)
        }}
      >
        <MainMenu>
          <MainMenu.DefaultItems.Export />
          <MainMenu.DefaultItems.SaveAsImage />
          <MainMenu.DefaultItems.SearchMenu />
          <MainMenu.DefaultItems.CommandPalette />
          <MainMenu.Separator />
          <MainMenu.DefaultItems.ChangeCanvasBackground />
          <MainMenu.DefaultItems.ClearCanvas />
          <MainMenu.DefaultItems.Help />
        </MainMenu>
        <WelcomeScreen>
          <WelcomeScreen.Hints.ToolbarHint />
          <WelcomeScreen.Hints.MenuHint />
          <WelcomeScreen.Hints.HelpHint />
          <WelcomeScreen.Center>
            <WelcomeScreen.Center.Heading>Draw anything. It saves by itself.</WelcomeScreen.Center.Heading>
          </WelcomeScreen.Center>
        </WelcomeScreen>
      </Excalidraw>
    </div>
  )
}
