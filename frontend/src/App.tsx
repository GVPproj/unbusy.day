import { useLiveQuery } from "@tanstack/react-db"
import { cardsView } from "./live"

// The column is a live query ordered by position (PRD F7): reorders from
// other clients arrive over SSE and re-sort the stack with no refetch.
export default function App() {
  const { data: cards, isReady } = useLiveQuery(cardsView)

  return (
    <main className="column">
      <h1>hello-cards</h1>
      {!isReady ? (
        <p className="status">connecting…</p>
      ) : (
        <ul className="cards">
          {cards.map((card) => (
            <li key={card.id} className="card">
              {card.label}
            </li>
          ))}
        </ul>
      )}
    </main>
  )
}
