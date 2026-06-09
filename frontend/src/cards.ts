import { createCollection, createLiveQueryCollection } from "@tanstack/react-db"

export type Card = {
  id: string
  label: string
  position: number
}

// The slice of EventSource the adapter touches — injectable so tests drive
// frames by hand (PRD F6).
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

export interface CardsTransport {
  fetchCards: () => Promise<Card[]>
  createEventSource: (url: string) => EventSourceLike
}

// The column is a live query ordered by position (PRD F7); reorders from
// other clients re-sort it on SSE arrival. Defined here (not inline in the
// component) so node tests exercise the same query the UI renders.
export function createCardsView(
  collection: ReturnType<typeof createCardsCollection>,
) {
  return createLiveQueryCollection((q) =>
    q.from({ card: collection }).orderBy(({ card }) => card.position, "asc"),
  )
}

export function createCardsCollection(transport: CardsTransport) {
  return createCollection<Card, string>({
    id: "cards",
    getKey: (card) => card.id,
    sync: {
      rowUpdateMode: "full",
      sync: ({ begin, write, commit, markReady, truncate }) => {
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

        // Each frame on the wire is the full ordered card list (PRD F2), so
        // sync is snapshot-replacement: truncate + reinsert in one committed
        // transaction. Live queries see each reorder atomically.
        const applySnapshot = (cards: Card[]) => {
          begin()
          truncate()
          for (const card of cards) {
            write({ type: "insert", value: card })
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
        // since a fresh EventSource carries no Last-Event-ID, refetch
        // instead of relying on replay. Covers server restarts with no
        // missed events (M2a).
        const connect = () => {
          es = transport.createEventSource("/api/events")
          es.addEventListener("open", () => {
            backoffMs = INITIAL_BACKOFF_MS
          })
          es.addEventListener("message", (e) => {
            liveEvents++
            applySnapshot(JSON.parse(e.data) as Card[])
          })
          // Server sentinel: our Last-Event-ID predates the replay ring;
          // deltas are unrecoverable, refetch the full state (PRD F2).
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
