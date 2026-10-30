// A live session on a shared sketch: one WebSocket to /live/:id. The server relays pointers and changes between
// everyone who has it open and names one of them the saver; this keeps who is here and passes the rest on.
import { useSyncExternalStore } from 'react'
import type { Access } from './types'

export interface LivePeer { id: string; user_id: number; name: string; role: Access }

export interface LiveState {
  connected: boolean
  me: LivePeer | null
  /** everyone else with it open, in the order they came */
  peers: LivePeer[]
  /** the peer id that saves the drawing */
  saver: string
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type LiveMessage = { t: string; from?: string; [k: string]: any }

/** Why the server let us go when it will not take us back as we were. */
const FINAL = new Set(['unshared', 'trashed', 'role'])

export class LiveSession {
  state: LiveState = { connected: false, me: null, peers: [], saver: '' }
  /** set when the server ended it for good: "unshared", "trashed" or "role" (open it again) */
  ended = ''
  private ws: WebSocket | null = null
  private closed = false
  private retry = 0
  private timer = 0
  private watchers = new Set<() => void>()
  private handlers = new Set<(m: LiveMessage) => void>()

  constructor(readonly drawingId: number) { this.connect() }

  private connect() {
    if (this.closed) return
    const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/live/${this.drawingId}`)
    this.ws = ws
    ws.onmessage = e => {
      let m: LiveMessage
      try { m = JSON.parse(e.data as string) } catch { return }
      this.receive(m)
    }
    ws.onclose = () => {
      if (this.ws !== ws) return
      this.ws = null
      this.set({ connected: false, peers: [], saver: '' })
      if (this.closed || this.ended) return
      this.timer = window.setTimeout(() => this.connect(), Math.min(15000, 1000 * 2 ** this.retry++))
    }
  }

  private receive(m: LiveMessage) {
    const s = this.state
    switch (m.t) {
      case 'hello':
        this.retry = 0
        this.set({ connected: true, me: m.you, peers: m.peers || [], saver: m.saver || '' })
        break
      case 'join': this.set({ peers: [...s.peers.filter(p => p.id !== m.peer.id), m.peer] }); break
      case 'leave': this.set({ peers: s.peers.filter(p => p.id !== m.id) }); break
      case 'saver': this.set({ saver: m.id || '' }); break
      case 'bye': if (FINAL.has(m.reason)) this.ended = m.reason; break
    }
    for (const h of this.handlers) h(m)
  }

  private set(p: Partial<LiveState>) {
    this.state = { ...this.state, ...p }
    for (const w of this.watchers) w()
  }

  send(m: LiveMessage) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(m))
  }

  /** Messages as they come (after the session has taken what it needs from them). */
  on(h: (m: LiveMessage) => void) { this.handlers.add(h); return () => { this.handlers.delete(h) } }

  subscribe = (w: () => void) => { this.watchers.add(w); return () => { this.watchers.delete(w) } }
  snapshot = () => this.state

  /** This window saves the drawing: the server named it, or it is on its own (not connected). */
  get saves() { return !this.state.connected || (!!this.state.me && this.state.saver === this.state.me.id) }

  close() {
    this.closed = true
    window.clearTimeout(this.timer)
    this.ws?.close()
    this.ws = null
  }
}

const OFF: LiveState = { connected: false, me: null, peers: [], saver: '' }
const noop = () => () => {}

export function useLive(s: LiveSession | null): LiveState {
  return useSyncExternalStore(s ? s.subscribe : noop, s ? s.snapshot : () => OFF)
}
