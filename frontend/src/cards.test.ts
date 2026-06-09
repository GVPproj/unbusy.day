import { afterEach, describe, expect, it, vi } from "vitest"
import type { Card, CardsTransport, EventSourceLike } from "./cards"
import { createCardsCollection, createCardsView } from "./cards"

// In-memory stand-in for EventSource: tests emit frames by hand. Mirrors the
// browser API surface the adapter touches (PRD F6) so the adapter under test
// is the real one, with only the network boundary faked.
class FakeEventSource implements EventSourceLike {
  static instances: FakeEventSource[] = []
  listeners = new Map<string, Array<(e: MessageEvent) => void>>()
  closed = false
  readyState = 1 // OPEN

  // A hard close: the browser gave up (readyState CLOSED) and will not
  // auto-reconnect — the adapter must take over (PRD F6).
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

function harness(initial: Card[]) {
  FakeEventSource.instances = []
  const fetches: Array<{
    resolve: (cards: Card[]) => void
    reject: (err: Error) => void
  }> = []
  const transport: CardsTransport = {
    fetchCards: () =>
      new Promise<Card[]>((resolve, reject) => {
        fetches.push({ resolve, reject })
      }),
    createEventSource: (url) => new FakeEventSource(url),
  }
  const collection = createCardsCollection(transport)
  return {
    collection,
    // Resolve the oldest in-flight GET /api/cards with the given snapshot
    // (defaults to `initial`). Tests control response timing explicitly.
    respond: (cards: Card[] = initial) => {
      fetches.shift()?.resolve(cards)
    },
    respondError: () => {
      fetches.shift()?.reject(new Error("network down"))
    },
    pendingFetches: () => fetches.length,
    es: () => FakeEventSource.instances[FakeEventSource.instances.length - 1]!,
  }
}

const seed: Card[] = [
  { id: "a", label: "Card A", position: 0 },
  { id: "b", label: "Card B", position: 1 },
  { id: "c", label: "Card C", position: 2 },
]

const ids = (cards: ReadonlyArray<Card>) => cards.map((c) => c.id)

describe("cards collection (PRD F6/F7 — SSE-backed read path)", () => {
  it("seeds from the authoritative read on connect", async () => {
    const h = harness(seed)
    const ready = h.collection.preload()
    h.respond()
    await ready

    expect(ids(h.collection.toArray)).toEqual(["a", "b", "c"])
  })

  it("orders the column by position, not arrival order (F7)", async () => {
    const shuffled: Card[] = [
      { id: "b", label: "Card B", position: 1 },
      { id: "c", label: "Card C", position: 2 },
      { id: "a", label: "Card A", position: 0 },
    ]
    const h = harness(shuffled)
    const view = createCardsView(h.collection)
    const ready = view.preload()
    h.respond()
    await ready

    expect(ids(view.toArray)).toEqual(["a", "b", "c"])
  })

  it("re-sorts the column when a reorder event arrives on the stream", async () => {
    const h = harness(seed)
    const view = createCardsView(h.collection)
    const ready = view.preload()
    h.respond()
    await ready

    h.es().emit(
      "message",
      [
        { id: "c", label: "Card C", position: 0 },
        { id: "a", label: "Card A", position: 1 },
        { id: "b", label: "Card B", position: 2 },
      ],
      "101",
    )

    expect(ids(view.toArray)).toEqual(["c", "a", "b"])
  })

  it("ignores a stale refetch that loses the race against a live event", async () => {
    const h = harness(seed)
    const view = createCardsView(h.collection)
    const ready = view.preload()

    // A reorder commits while GET /api/cards is still in flight: the live
    // frame is newer than whatever the fetch returns.
    h.es().emit(
      "message",
      [
        { id: "c", label: "Card C", position: 0 },
        { id: "a", label: "Card A", position: 1 },
        { id: "b", label: "Card B", position: 2 },
      ],
      "101",
    )
    h.respond() // stale a,b,c snapshot lands after the event
    await ready

    expect(ids(view.toArray)).toEqual(["c", "a", "b"])
  })

  it("refetches the full state on the overflow sentinel (F2)", async () => {
    const h = harness(seed)
    const view = createCardsView(h.collection)
    const ready = view.preload()
    h.respond()
    await ready

    // Server replay ring couldn't cover our Last-Event-ID gap; it tells us
    // to drop deltas and refetch the authoritative order.
    h.es().emit("overflow", { reason: "last-event-id outside replay window" })
    expect(h.pendingFetches()).toBe(1)

    h.respond([
      { id: "b", label: "Card B", position: 0 },
      { id: "c", label: "Card C", position: 1 },
      { id: "a", label: "Card A", position: 2 },
    ])
    await view.toArrayWhenReady()
    expect(ids(view.toArray)).toEqual(["b", "c", "a"])
  })

  describe("hard close (F6 — adapter-owned reconnect)", () => {
    afterEach(() => {
      vi.useRealTimers()
    })

    it("reopens the stream after a backoff delay and resyncs", async () => {
      vi.useFakeTimers()
      const h = harness(seed)
      const view = createCardsView(h.collection)
      const ready = view.preload()
      h.respond()
      await ready

      h.es().fail()
      // Not immediate: reconnect waits out the backoff delay.
      expect(FakeEventSource.instances.length).toBe(1)

      await vi.advanceTimersByTimeAsync(1000)
      expect(FakeEventSource.instances.length).toBe(2)
      // A fresh EventSource carries no Last-Event-ID, so the gap is
      // unrecoverable from the stream — adapter refetches the full state.
      expect(h.pendingFetches()).toBe(1)

      h.respond([
        { id: "c", label: "Card C", position: 0 },
        { id: "b", label: "Card B", position: 1 },
        { id: "a", label: "Card A", position: 2 },
      ])
      await vi.advanceTimersByTimeAsync(0)
      expect(ids(view.toArray)).toEqual(["c", "b", "a"])
    })

    it("backs off exponentially across consecutive failures", async () => {
      vi.useFakeTimers()
      const h = harness(seed)
      const ready = h.collection.preload()
      h.respond()
      await ready

      h.es().fail()
      await vi.advanceTimersByTimeAsync(1000)
      expect(FakeEventSource.instances.length).toBe(2)

      h.es().fail()
      // Second consecutive failure: 1s is no longer enough…
      await vi.advanceTimersByTimeAsync(1000)
      expect(FakeEventSource.instances.length).toBe(2)
      // …the delay doubled to 2s.
      await vi.advanceTimersByTimeAsync(1000)
      expect(FakeEventSource.instances.length).toBe(3)
    })

    it("retries a failed refetch instead of leaving the column empty", async () => {
      vi.useFakeTimers()
      const h = harness(seed)
      const view = createCardsView(h.collection)
      void view.preload()

      h.respondError()
      await vi.advanceTimersByTimeAsync(1000)
      expect(h.pendingFetches()).toBe(1)

      h.respond()
      await view.toArrayWhenReady()
      expect(ids(view.toArray)).toEqual(["a", "b", "c"])
    })
  })
})
