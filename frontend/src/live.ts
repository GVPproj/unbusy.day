import type { CardsTransport } from "./cards"
import { createCardsCollection, createCardsView } from "./cards"

// Browser wiring for the SSE collection. Kept out of cards.ts so importing the
// adapter (e.g. in node tests) never opens a network connection as a side
// effect.
const liveTransport: CardsTransport = {
  fetchCards: async () => {
    const res = await fetch("/api/cards")
    if (!res.ok) {
      throw new Error(`GET /api/cards: ${res.status}`)
    }
    const body = (await res.json()) as { cards: Awaited<ReturnType<CardsTransport["fetchCards"]>> }
    return body.cards
  },
  // Full new order in, txid string out. Non-2xx throws so the mutation
  // handler rejects and TanStack DB rolls the drag back.
  reorder: async (order) => {
    const res = await fetch("/api/cards/reorder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ order }),
    })
    if (!res.ok) {
      throw new Error(`POST /api/cards/reorder: ${res.status}`)
    }
    return (await res.json()) as { txid: string }
  },
  createEventSource: (url) => new EventSource(url),
}

export const cardsCollection = createCardsCollection(liveTransport)
export const cardsView = createCardsView(cardsCollection)
