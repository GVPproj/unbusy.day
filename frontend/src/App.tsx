import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core"
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable"
import { CSS } from "@dnd-kit/utilities"
import { useLiveQuery } from "@tanstack/react-db"
import type { Card } from "./cards"
import { reorderCards } from "./cards"
import { cardsCollection, cardsView } from "./live"

function SortableCard({ card }: { card: Card }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: card.id })

  return (
    <li
      ref={setNodeRef}
      className={isDragging ? "card dragging" : "card"}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      {...attributes}
      {...listeners}
    >
      {card.label}
    </li>
  )
}

// The column is a live query ordered by position: reorders from other clients
// arrive over SSE and re-sort the stack with no refetch. Dragging a card
// applies the new order optimistically and POSTs it; the overlay holds until
// the matching txid frame arrives, and a rejected reorder snaps back — all
// inside reorderCards/onUpdate.
export default function App() {
  const { data: cards, isReady } = useLiveQuery(cardsView)
  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const order = cards.map((card) => card.id)
    const from = order.indexOf(String(active.id))
    const to = order.indexOf(String(over.id))
    if (from === -1 || to === -1) return
    reorderCards(cardsCollection, arrayMove(order, from, to))
  }

  return (
    <main className="column">
      <h1>hello-cards</h1>
      {!isReady ? (
        <p className="status">connecting…</p>
      ) : (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
        >
          <SortableContext
            items={cards.map((card) => card.id)}
            strategy={verticalListSortingStrategy}
          >
            <ul className="cards">
              {cards.map((card) => (
                <SortableCard key={card.id} card={card} />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      )}
    </main>
  )
}
