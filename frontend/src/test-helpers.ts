import type { EventSourceLike } from "./cards"

// In-memory stand-in for EventSource: tests emit frames by hand. Mirrors the
// browser API surface the adapter touches so the adapter under test is the
// real one, with only the network boundary faked.
export class FakeEventSource implements EventSourceLike {
  static instances: FakeEventSource[] = []
  listeners = new Map<string, Array<(e: MessageEvent) => void>>()
  closed = false
  readyState = 1 // OPEN

  // A hard close: the browser gave up (readyState CLOSED) and will not
  // auto-reconnect — the adapter must take over.
  fail() {
    this.readyState = 2
    for (const fn of this.listeners.get("error") ?? []) {
      fn(new MessageEvent("error"))
    }
  }

  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    const fns = this.listeners.get(type) ?? []
    fns.push(fn)
    this.listeners.set(type, fns)
  }

  close() {
    this.closed = true
  }

  emit(type: string, data: unknown, lastEventId = "") {
    for (const fn of this.listeners.get(type) ?? []) {
      fn(new MessageEvent(type, { data: JSON.stringify(data), lastEventId }))
    }
  }
}
