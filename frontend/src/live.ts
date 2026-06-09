import type { CardsTransport } from "./cards"
import { createCardsCollection, createCardsView } from "./cards"

// Browser wiring for the SSE collection (PRD F6). Kept out of cards.ts so
// importing the adapter (e.g. in node tests) never opens a network
// connection as a side effect.
const liveTransport: CardsTransport = {
  fetchCards: async () => {
    const res = await fetch("/api/cards")
    if (!res.ok) {
      throw new Error(`GET /api/cards: ${res.status}`)
    }
    const body = (await res.json()) as { cards: Awaited<ReturnType<CardsTransport["fetchCards"]>> }
    return body.cards
  },
  createEventSource: (url) => new EventSource(url),
}

export const cardsCollection = createCardsCollection(liveTransport)
export const cardsView = createCardsView(cardsCollection)
