// Excalidraw loads its fonts at run time from window.EXCALIDRAW_ASSET_PATH;
// copy them next to the built app so nothing comes from a CDN.
import { cpSync, existsSync } from 'node:fs'

const from = 'node_modules/@excalidraw/excalidraw/dist/prod/fonts'
if (!existsSync(from)) throw new Error(`missing ${from}: run npm ci`)
cpSync(from, 'dist/excalidraw/fonts', { recursive: true })
console.log('excalidraw fonts copied to dist/excalidraw/fonts')
