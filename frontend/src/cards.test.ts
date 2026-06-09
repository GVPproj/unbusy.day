import { afterEach, describe, expect, it, vi } from "vitest"
import type { Card, CardsTransport } from "./cards"
import {
  createCardsCollection,
  createCardsView,
  reorderCards,
  TXID_TIMEOUT_MS,
} from "./cards"
import { FakeEventSource } from "./test-helpers"

function harness(initial: Card[]) {
  FakeEventSource.instances = []
  const fetches: Array<{
    resolve: (cards: Card[]) => void
    reject: (err: Error) => void
  }> = []
  const reorders: Array<{
    order: string[]
    resolve: (result: { txid: string }) => void
    reject: (err: Error) => void
  }> = []
  const transport: CardsTransport = {
    fetchCards: () =>
      new Promise<Card[]>((resolve, reject) => {
        fetches.push({ resolve, reject })
      }),
    reorder: (order) =>
      new Promise<{ txid: string }>((resolve, reject) => {
        reorders.push({ order, resolve, reject })
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
    // Orders POSTed via transport.reorder, oldest first.
    reorderCalls: () => reorders.map((r) => r.order),
    // Resolve the oldest in-flight POST /api/cards/reorder with a txid.
    respondReorder: (txid: string) => {
      reorders.shift()?.resolve({ txid })
    },
    // Reject it with a structured mutation error (PRD F5).
    rejectReorder: () => {
      reorders.shift()?.reject(new Error("reorder rejected: not a permutation"))
    },
    es: () => FakeEventSource.instances[FakeEventSource.instances.length - 1]!,
  }
}

const seed: Card[] = [
  { id: "a", label: "Card A", position: 0 },
  { id: "b", label: "Card B", position: 1 },
  { id: "c", label: "Card C", position: 2 },
]

const ids = (cards: ReadonlyArray<Card>) => cards.map((c) => c.id)

// Drain microtasks plus one macrotask turn, so any (wrongly) settled handler
// promise has had every chance to drop the optimistic overlay.
const flushTasks = () => new Promise<void>((resolve) => setTimeout(resolve, 0))

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

  describe("optimistic write (PRD F8 — txid handshake)", () => {
    afterEach(() => {
      vi.useRealTimers()
    })

    it("applies the new order optimistically and POSTs the full order", async () => {
      const h = harness(seed)
      const view = createCardsView(h.collection)
      const ready = view.preload()
      h.respond()
      await ready

      reorderCards(h.collection, ["b", "a", "c"])

      // Renders before the POST resolves — the optimistic path (PRD: <1 frame).
      expect(ids(view.toArray)).toEqual(["b", "a", "c"])
      expect(h.reorderCalls()).toEqual([["b", "a", "c"]])
    })

    it("holds the optimistic order past the POST until the matching txid arrives (no snap-back)", async () => {
      const h = harness(seed)
      const view = createCardsView(h.collection)
      const ready = view.preload()
      h.respond()
      await ready

      const tx = reorderCards(h.collection, ["b", "a", "c"])
      let persisted = false
      void tx.isPersisted.promise.then(() => {
        persisted = true
      })

      h.respondReorder("42")
      await flushTasks()

      // The txid frame hasn't arrived yet: dropping the overlay now would
      // snap the column back to the stale synced order until SSE catches up.
      expect(ids(view.toArray)).toEqual(["b", "a", "c"])
      expect(persisted).toBe(false)
    })

    it("settles the transaction when the matching txid frame arrives", async () => {
      const h = harness(seed)
      const view = createCardsView(h.collection)
      const ready = view.preload()
      h.respond()
      await ready

      const tx = reorderCards(h.collection, ["b", "a", "c"])
      h.respondReorder("42")
      await flushTasks()

      // The mutation's own SSE frame: same order, stamped id: 42.
      h.es().emit(
        "message",
        [
          { id: "b", label: "Card B", position: 0 },
          { id: "a", label: "Card A", position: 1 },
          { id: "c", label: "Card C", position: 2 },
        ],
        "42",
      )

      await tx.isPersisted.promise
      expect(ids(view.toArray)).toEqual(["b", "a", "c"])
    })

    it("rolls back to the server order when the POST is rejected (F5)", async () => {
      const h = harness(seed)
      const view = createCardsView(h.collection)
      const ready = view.preload()
      h.respond()
      await ready

      const tx = reorderCards(h.collection, ["b", "a", "c"])
      expect(ids(view.toArray)).toEqual(["b", "a", "c"])

      h.rejectReorder()
      await expect(tx.isPersisted.promise).rejects.toThrow()

      // Dragged card visibly snaps back (PRD F5/F8).
      expect(ids(view.toArray)).toEqual(["a", "b", "c"])
    })

    it("POSTs the dropped order exactly, even mid-flight over a pending drag", async () => {
      const h = harness(seed)
      const view = createCardsView(h.collection)
      const ready = view.preload()
      h.respond()
      await ready

      // Drag 1 is still waiting on its txid round-trip…
      reorderCards(h.collection, ["b", "a", "c"])
      // …when drag 2 lands, built on drag 1's optimistic view. Its intent
      // must reach the server verbatim — not reconstructed from state that
      // straddles two pending transactions.
      reorderCards(h.collection, ["c", "a", "b"])

      expect(h.reorderCalls()).toEqual([
        ["b", "a", "c"],
        ["c", "a", "b"],
      ])
    })

    it("releases the overlay after a timeout if the txid frame never arrives", async () => {
      vi.useFakeTimers()
      const h = harness(seed)
      const ready = h.collection.preload()
      h.respond()
      await ready

      // POST committed server-side, but the stream is silently dead: never
      // settling would pin the transaction (and overlay) forever. Time out
      // and let reconnect/resync converge the state instead.
      const tx = reorderCards(h.collection, ["b", "a", "c"])
      h.respondReorder("42")
      await vi.advanceTimersByTimeAsync(TXID_TIMEOUT_MS)

      await expect(tx.isPersisted.promise).resolves.toBeDefined()
    })
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
