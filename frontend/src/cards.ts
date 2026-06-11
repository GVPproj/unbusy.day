import { createCollection, createLiveQueryCollection } from "@tanstack/react-db"

export type Card = {
  id: string
  label: string
  position: number
}

// The slice of EventSource the adapter touches — injectable so tests drive
// frames by hand.
export interface EventSourceLike {
  readonly readyState: number
  addEventListener(type: string, fn: (e: MessageEvent) => void): void
  close(): void
}

// EventSource.CLOSED — the browser gave up and will not auto-reconnect.
// (Transient drops stay readyState CONNECTING and the browser retries with
// Last-Event-ID on its own; those need nothing from us.)
const CLOSED = 2

const INITIAL_BACKOFF_MS = 1_000
const MAX_BACKOFF_MS = 30_000

// Upper bound on holding an optimistic overlay waiting for its txid frame. The
// write is already committed server-side by then; if the stream is silently
// dead, release the overlay and let reconnect/resync converge the state instead
// of pinning the transaction forever.
export const TXID_TIMEOUT_MS = 10_000

export interface CardsTransport {
  fetchCards: () => Promise<Card[]>
  // POST /api/cards/reorder with the full new top-to-bottom order. txid is a
  // string end-to-end — JS Number loses precision above 2^53.
  reorder: (order: string[]) => Promise<{ txid: string }>
  createEventSource: (url: string) => EventSourceLike
}

// Drag-drop entry point: apply the full new order optimistically by rewriting
// every card's position to its index. One transaction → one onUpdate call
// carrying the whole column, mirroring the POST payload shape.
export function reorderCards(
  collection: ReturnType<typeof createCardsCollection>,
  order: string[],
) {
  return collection.update(order, (drafts) => {
    drafts.forEach((draft, i) => {
      draft.position = i
    })
  })
}

// The column is a live query ordered by position; reorders from other clients
// re-sort it on SSE arrival. Defined here (not inline in the component) so node
// tests exercise the same query the UI renders.
export function createCardsView(
  collection: ReturnType<typeof createCardsCollection>,
) {
  return createLiveQueryCollection((q) =>
    q.from({ card: collection }).orderBy(({ card }) => card.position, "asc"),
  )
}

export function createCardsCollection(transport: CardsTransport) {
  // txid handshake: every SSE frame carries `id: <txid>`; the mutation handler
  // holds its optimistic overlay until the txid the POST returned shows up
  // here, so the overlay only drops once the synced state already reflects the
  // write — no snap-back flicker. Bounded set: old entries only matter for the
  // frame-beats-POST-response race, which a recent window fully covers.
  const SEEN_TXID_CAP = 1024
  const seenTxids = new Set<string>()
  const txidWaiters = new Map<string, Set<() => void>>()

  const markTxidSeen = (txid: string) => {
    if (!txid) return
    seenTxids.add(txid)
    if (seenTxids.size > SEEN_TXID_CAP) {
      seenTxids.delete(seenTxids.values().next().value!)
    }
    for (const settle of txidWaiters.get(txid) ?? []) settle()
  }

  const awaitTxid = (txid: string) =>
    new Promise<void>((resolve) => {
      if (seenTxids.has(txid)) {
        resolve()
        return
      }
      const waiters = txidWaiters.get(txid) ?? new Set<() => void>()
      txidWaiters.set(txid, waiters)
      const settle = () => {
        clearTimeout(timer)
        waiters.delete(settle)
        if (waiters.size === 0) txidWaiters.delete(txid)
        resolve()
      }
      const timer = setTimeout(settle, TXID_TIMEOUT_MS)
      waiters.add(settle)
    })

  return createCollection<Card, string>({
    id: "cards",
    getKey: (card) => card.id,
    // The server wants the full permutation, but neither handler input alone
    // has it: transaction.mutations drops cards whose position didn't change,
    // and the collection still reads the pre-mutation order here. Overlay the
    // mutated rows on the collection state to recover the complete order.
    onUpdate: async ({ transaction, collection }) => {
      const byId = new Map<string, Card>(
        collection.toArray.map((card) => [card.id, card]),
      )
      for (const m of transaction.mutations) {
        byId.set(m.modified.id, m.modified)
      }
      const order = [...byId.values()]
        .sort((a, b) => a.position - b.position)
        .map((card) => card.id)
      const { txid } = await transport.reorder(order)
      await awaitTxid(txid)
    },
    sync: {
      rowUpdateMode: "full",
      sync: ({ begin, write, commit, markReady }) => {
        let es: EventSourceLike | undefined
        let liveEvents = 0
        let backoffMs = INITIAL_BACKOFF_MS
        let stopped = false
        const retryTimers = new Set<ReturnType<typeof setTimeout>>()

        // One backoff schedule shared by both failure paths (stream hard
        // close, fetch failure); any success resets it.
        const scheduleRetry = (retry: () => void) => {
          if (stopped) return
          const timer = setTimeout(() => {
            retryTimers.delete(timer)
            retry()
          }, backoffMs)
          retryTimers.add(timer)
          backoffMs = Math.min(backoffMs * 2, MAX_BACKOFF_MS)
        }

        // Each frame on the wire is the full ordered card list, applied as
        // keyed upserts (plus deletes for vanished rows) in one committed
        // transaction. NOT truncate+reinsert: truncate snapshots the optimistic
        // overlay and re-applies it over every later frame, which pins a
        // client's own completed drag on top of newer foreign reorders — the
        // two-browser desync the convergence test reproduces. Per-key updates
        // also clear completed-mutation overlays and let frames queue while a
        // local transaction is persisting, as the library intends.
        const knownRows = new Map<string, Card>()
        const applySnapshot = (cards: Card[]) => {
          begin()
          for (const [id, row] of knownRows) {
            if (!cards.some((card) => card.id === id)) {
              write({ type: "delete", value: row })
              knownRows.delete(id)
            }
          }
          for (const card of cards) {
            write({ type: knownRows.has(card.id) ? "update" : "insert", value: card })
            knownRows.set(card.id, card)
          }
          commit()
        }

        // resync() = authoritative GET /api/cards. Used on first connect and
        // whenever the stream can't be trusted to be gapless (overflow
        // sentinel, reopen after hard close). A live frame is always newer
        // than any in-flight fetch, so a fetch result that loses that race
        // is discarded rather than letting it roll the column back. Failed
        // fetches retry — on a flaky network the column would otherwise stay
        // empty/stale until the next live frame happened to arrive.
        const resync = () => {
          const eventsAtFetch = liveEvents
          transport
            .fetchCards()
            .then((cards) => {
              if (liveEvents === eventsAtFetch) {
                applySnapshot(cards)
              }
              backoffMs = INITIAL_BACKOFF_MS
              markReady()
            })
            .catch(() => {
              scheduleRetry(resync)
            })
        }

        // The browser handles transient drops natively (auto-reconnect with
        // Last-Event-ID; the server replays the gap from its ring). Only a
        // hard close lands in the error handler: we reopen with backoff, and
        // since a fresh EventSource carries no Last-Event-ID, refetch instead
        // of relying on replay. Covers server restarts with no missed events.
        const connect = () => {
          es = transport.createEventSource("/api/events")
          es.addEventListener("open", () => {
            backoffMs = INITIAL_BACKOFF_MS
          })
          es.addEventListener("message", (e) => {
            liveEvents++
            applySnapshot(JSON.parse(e.data) as Card[])
            // After the snapshot, so waiters resume against synced state
            // that already includes their write.
            markTxidSeen(e.lastEventId)
          })
          // Server sentinel: our Last-Event-ID predates the replay ring;
          // deltas are unrecoverable, refetch the full state.
          es.addEventListener("overflow", () => {
            resync()
          })
          es.addEventListener("error", () => {
            if (stopped || es?.readyState !== CLOSED) return
            scheduleRetry(() => {
              connect()
              resync()
            })
          })
        }

        // Connect, then resync — in that order, so a mutation committed
        // between the two can't fall in the gap.
        connect()
        resync()

        return () => {
          stopped = true
          for (const timer of retryTimers) clearTimeout(timer)
          es?.close()
        }
      },
    },
  })
}
