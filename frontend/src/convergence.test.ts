import { afterEach, describe, expect, it, vi } from "vitest"
import type { Card, CardsTransport } from "./cards"
import { createCardsCollection, createCardsView, reorderCards } from "./cards"
import { FakeEventSource } from "./test-helpers"

// Two real collections against one fake server, with macrotask-realistic
// network delays (fake timers). Mimics two browsers on /api: every reorder
// commits server-side, then the response and the SSE broadcast race back to
// the clients — the same shape as dev against the Go server. Acceptance
// criteria 2/3: after any interleaving of drags, both clients converge to
// the server's order.

const NETWORK_MS = 2

const seed: Card[] = [
  { id: "a", label: "Card A", position: 0 },
  { id: "b", label: "Card B", position: 1 },
  { id: "c", label: "Card C", position: 2 },
]

function createSim() {
  let nextTxid = 1000
  let serverCards: Card[] = seed.map((c) => ({ ...c }))

  // Only the surface the broadcast loop touches, to avoid a type cycle
  // through makeClient's return type.
  const clients: Array<{
    frameDelay: number
    deliver: (frame: Card[], txid: string) => void
  }> = []

  function makeClient(name: string, frameDelay = 0) {
    const sources: FakeEventSource[] = []
    const transport: CardsTransport = {
      fetchCards: () =>
        new Promise((resolve) => {
          // Snapshot at response time, like a real GET.
          setTimeout(
            () => resolve(serverCards.map((c) => ({ ...c }))),
            NETWORK_MS,
          )
        }),
      reorder: (order) =>
        new Promise((resolve, reject) => {
          setTimeout(() => {
            // Server tx: validate permutation, rewrite positions (PRD F1).
            const ids = new Set(serverCards.map((c) => c.id))
            if (
              order.length !== ids.size ||
              !order.every((id) => ids.has(id))
            ) {
              setTimeout(
                () => reject(new Error("not a permutation")),
                NETWORK_MS,
              )
              return
            }
            serverCards = order.map((id, position) => ({
              ...serverCards.find((c) => c.id === id)!,
              position,
            }))
            const txid = String(nextTxid++)
            const frame = serverCards.map((c) => ({ ...c }))
            setTimeout(() => resolve({ txid }), NETWORK_MS)
            // Post-commit publish fans to every subscriber (PRD F2).
            for (const client of clients) {
              setTimeout(
                () => client.deliver(frame, txid),
                NETWORK_MS + client.frameDelay,
              )
            }
          }, NETWORK_MS)
        }),
      createEventSource: (url) => {
        const es = new FakeEventSource(url)
        sources.push(es)
        return es
      },
    }
    const collection = createCardsCollection(transport)
    const view = createCardsView(collection)
    const client = {
      name,
      frameDelay,
      collection,
      view,
      deliver: (frame: Card[], txid: string) => {
        sources[sources.length - 1]?.emit("message", frame, txid)
      },
      order: () => view.toArray.map((c) => c.id),
      // A user drags based on what they currently SEE (overlay included).
      dragTopToBottom: () => {
        const order = view.toArray.map((c) => c.id)
        reorderCards(collection, [...order.slice(1), order[0]!])
      },
    }
    clients.push(client)
    return client
  }

  return {
    makeClient,
    serverOrder: () =>
      [...serverCards].sort((x, y) => x.position - y.position).map((c) => c.id),
  }
}

describe("two-client convergence (criteria 2/3)", () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  // Sweep the start of B's drags across every offset relative to A's, so
  // every interleaving of POST round-trips and frame deliveries is covered
  // deterministically — including B dragging mid-flight of A's writes.
  it("drag storm in A, then drag storm in B, every interleaving", async () => {
    for (let offset = 0; offset <= 24; offset++) {
      vi.useFakeTimers()
      const sim = createSim()
      const a = sim.makeClient("A")
      const b = sim.makeClient("B", 3) // B's frames arrive a bit later

      void a.view.preload()
      void b.view.preload()
      await vi.advanceTimersByTimeAsync(10)
      expect(a.order()).toEqual(["a", "b", "c"])

      a.dragTopToBottom()
      await vi.advanceTimersByTimeAsync(3)
      a.dragTopToBottom()

      await vi.advanceTimersByTimeAsync(offset)
      b.dragTopToBottom()
      await vi.advanceTimersByTimeAsync(3)
      b.dragTopToBottom()

      // Quiesce well past round-trips and the txid timeout.
      await vi.advanceTimersByTimeAsync(60_000)

      expect(a.order(), `offset=${offset}: A vs server`).toEqual(
        sim.serverOrder(),
      )
      expect(b.order(), `offset=${offset}: B vs server`).toEqual(
        sim.serverOrder(),
      )
      vi.useRealTimers()
    }
  })

  // True concurrency: both clients drag at once, repeatedly. The final order
  // is last-writer-wins (a v1 limit the PRD accepts) but the views must
  // still converge on whatever the server settled on.
  it("simultaneous drag storms still converge", async () => {
    for (let offset = 0; offset <= 12; offset++) {
      vi.useFakeTimers()
      const sim = createSim()
      const a = sim.makeClient("A")
      const b = sim.makeClient("B", 3)

      void a.view.preload()
      void b.view.preload()
      await vi.advanceTimersByTimeAsync(10)

      a.dragTopToBottom()
      await vi.advanceTimersByTimeAsync(offset)
      b.dragTopToBottom()
      await vi.advanceTimersByTimeAsync(2)
      a.dragTopToBottom()
      b.dragTopToBottom()
      await vi.advanceTimersByTimeAsync(2)
      b.dragTopToBottom()

      await vi.advanceTimersByTimeAsync(60_000)

      expect(a.order(), `offset=${offset}: A vs server`).toEqual(
        sim.serverOrder(),
      )
      expect(b.order(), `offset=${offset}: B vs server`).toEqual(
        sim.serverOrder(),
      )
      vi.useRealTimers()
    }
  })
})
