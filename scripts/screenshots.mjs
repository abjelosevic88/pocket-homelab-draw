// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds sketches, text diagrams and draw.io diagrams):
//
//   DATA_DIR=/tmp/draw-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Maya DRAWIO_URL=http://127.0.0.1:8138 PORT=18213 ./pocket-draw
//   CDP=http://127.0.0.1:19281 BASE=http://127.0.0.1:18213 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                  # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
// Without DRAWIO_URL (or with NO_DRAWIO=1) the draw.io editor is not opened.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18213'
const shots = process.env.SHOTS !== '0'
const drawio = process.env.NO_DRAWIO !== '1'
const dir = new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { // Excalidraw's font-subsetting worker cannot start from a Vite chunk; it falls back to the main thread (harmless)
  if (m.type() === 'error' && !/Failed to load resource|favicon|Failed to use workers for subsetting/.test(m.text())) errors.push(`${page.url()}: ${m.text()}`) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1200, still = true) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  if (still) await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) {
  if (!shots) return
  for (let i = 0; ; i++) {
    try { await page.screenshot({ path: dir + name + '.png' }); return } catch (e) { if (i === 3) throw e; await page.waitForTimeout(800) }
  }
}
const api = (page, path, init) => page.evaluate(([p, i]) => fetch(p, i).then(async r => ({ status: r.status, body: await r.json().catch(() => null) })), [path, init || {}])
async function idOf(page, title) {
  const r = await api(page, '/api/v1/drawings?limit=200')
  const d = r.body.drawings.find(x => x.title === title)
  assert.ok(d, `demo drawing ${title}`)
  return d
}

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/')
  assert.ok((await page.content()).includes('All drawings'), 'the drawings list')
  // the browser draws the missing pictures of sketches and text diagrams
  await page.waitForFunction(() => document.querySelectorAll('.dr-card .thumb img').length >= 16, null, { timeout: 60000 })
  await page.waitForTimeout(1500)
  await shot(page, 'desktop-home')

  // a sketch: Excalidraw opens, a drawn rectangle is saved by itself, with a new picture
  const sk = await idOf(page, 'Homelab network')
  await go(page, `/d/${sk.id}`, 2500)
  await page.waitForSelector('.excalidraw canvas', { timeout: 20000 })
  await page.waitForTimeout(1500)
  await shot(page, 'desktop-sketch')
  // the checks that change things work on copies, so the demo stays as it is
  const full = await api(page, `/api/v1/drawings/${sk.id}`)
  const make = (kind, data) => api(page, '/api/v1/drawings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind, title: 'Browser check', data }) }).then(r => r.body)
  const forget = async id => { await api(page, `/api/v1/drawings/${id}`, { method: 'DELETE' }); await api(page, `/api/v1/drawings/${id}?forever=1`, { method: 'DELETE' }) }
  const tsk = await make('sketch', full.body.data)
  await go(page, `/d/${tsk.id}`, 2500)
  await page.waitForSelector('.excalidraw canvas', { timeout: 20000 })
  await page.keyboard.press('r')
  await page.mouse.move(1000, 640); await page.mouse.down(); await page.mouse.move(1180, 760, { steps: 8 }); await page.mouse.up()
  await page.waitForSelector('.ed-status.s-saved', { timeout: 15000 })
  await page.waitForTimeout(2500)
  const after = await api(page, `/api/v1/drawings/${tsk.id}`)
  assert.ok(after.body.rev > tsk.rev, `the sketch was saved (rev ${tsk.rev} → ${after.body.rev})`)
  assert.equal(after.body.preview_rev, after.body.rev, 'its picture follows')
  // a save from an old rev (another tab) is refused, and the page says so
  await api(page, `/api/v1/drawings/${tsk.id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ data: JSON.stringify({ type: 'excalidraw', elements: [], appState: {}, files: {} }), rev: after.body.rev }) })
  await page.keyboard.press('o')
  await page.mouse.move(400, 640); await page.mouse.down(); await page.mouse.move(500, 720, { steps: 6 }); await page.mouse.up()
  await page.waitForSelector('.ed-conflict', { timeout: 15000 })
  await page.getByRole('button', { name: 'Keep mine' }).click()
  await page.waitForSelector('.ed-status.s-saved', { timeout: 15000 })
  await forget(tsk.id)
  await go(page, `/d/${sk.id}`, 2500)
  await page.waitForSelector('.excalidraw canvas', { timeout: 20000 })
  await page.locator('.ed-head button[aria-label="History"]').click()
  await page.waitForSelector('.hist-item', { timeout: 8000 })
  await page.waitForTimeout(800)
  await shot(page, 'desktop-history')
  await page.keyboard.press('Escape')

  // a text diagram: Mermaid draws, a typo shows the mistake and is saved anyway
  const mm = await idOf(page, 'How the nightly backup runs')
  await go(page, `/d/${mm.id}`, 1500)
  await page.waitForSelector('.mm-svg svg', { timeout: 20000 })
  await page.waitForTimeout(800)
  await shot(page, 'desktop-diagram')
  const tmm = await make('mermaid', 'graph TD\n  A-->B\n')
  await go(page, `/d/${tmm.id}`, 1200)
  await page.waitForSelector('.mm-svg svg', { timeout: 20000 })
  await page.locator('.mm-text').press('End')
  await page.locator('.mm-text').type('\n    T->>')
  await page.waitForSelector('.mm-status.bad', { timeout: 10000 })
  await page.waitForSelector('.ed-status.s-saved', { timeout: 15000 })
  assert.ok((await api(page, `/api/v1/drawings/${tmm.id}`)).body.data.includes('A-->B'), 'the text diagram with a mistake was saved')
  await forget(tmm.id)

  if (drawio) {
    const dio = await idOf(page, 'Website sitemap')
    await go(page, `/d/${dio.id}`, 1500, false)
    await page.waitForSelector('.dio-host iframe.ready', { timeout: 30000 })
    await page.waitForTimeout(4000)
    await shot(page, 'desktop-drawio')
  }

  await go(page, '/')
  await page.keyboard.press('n')
  await page.waitForSelector('.new-dlg .kind-card')
  await page.locator('.kind-card.k-mermaid').click()
  await page.waitForTimeout(500)
  await shot(page, 'desktop-new')
  await page.keyboard.press('Escape')
  await go(page, '/folder/2')
  await shot(page, 'desktop-folder')
  for (const p of ['/recent', '/starred', '/trash', '/kind/sketch', '/kind/mermaid', '/search?q=router', '/settings/general', '/settings/notifications', '/settings/apps', '/settings/ai', '/settings/tokens', '/settings/data', '/settings/about']) await go(page, p, 400)
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true, dark: true })
  await go(page, '/')
  await page.waitForFunction(() => document.querySelectorAll('.dr-card .thumb img').length >= 6, null, { timeout: 30000 })
  await page.waitForTimeout(1200)
  await shot(page, 'phone-home')
  const mm = await idOf(page, 'Morning routine')
  await go(page, `/d/${mm.id}`, 1500)
  await page.waitForSelector('.mm-svg svg', { timeout: 20000 })
  await page.waitForTimeout(600)
  await shot(page, 'phone-diagram')
  const sk = await idOf(page, 'Living room layout')
  await go(page, `/d/${sk.id}`, 2500)
  await page.waitForSelector('.excalidraw canvas', { timeout: 20000 })
  await page.waitForTimeout(1500)
  await shot(page, 'phone-sketch')
  await ctx.close()
}

await browser.close()
if (errors.length) {
  console.error('browser errors:\n' + errors.join('\n'))
  process.exit(1)
}
console.log('ok')
