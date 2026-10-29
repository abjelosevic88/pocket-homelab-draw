// A sketch: Excalidraw, kept in this app (fonts served from here, no collaboration server, no AI calls out).
import { useEffect, useMemo, useRef } from 'react'
import { Excalidraw, MainMenu, WelcomeScreen, exportToSvg, getSceneVersion, serializeAsJSON, restore } from '@excalidraw/excalidraw'
import '@excalidraw/excalidraw/index.css'
import type { ExcalidrawImperativeAPI, LibraryItems } from '@excalidraw/excalidraw/types'
import { api, V1 } from '../lib/api'
import { blankPicture } from '../lib/draw'
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
}

export default function SketchEditor({ data, title, dark, settings, onChange }: EditorProps) {
  const apiRef = useRef<ExcalidrawImperativeAPI | null>(null)
  const lastKey = useRef('')
  const libTimer = useRef(0)
  const libLoaded = useRef(false)

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
    return { elements: scene.elements, appState, files: scene.files, scrollToContent: true }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

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
        UIOptions={{ canvasActions: { loadScene: false, saveToActiveFile: false, toggleTheme: null, export: { saveFileToDisk: true }, saveAsImage: true } }}
        onChange={(elements, appState, files) => {
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
