// Package pubsub is the in-process fan-out bus: one mutation event is
// delivered to every live SSE subscriber. Single-machine only — cross-instance
// fan-out would need an external bus (LISTEN/NOTIFY or Redis).
package pubsub

import (
	"sync"

	"github.com/grahamvanpelt/unbusy.day/cards"
)

// Broker fans cards.Events to every live subscriber. It implements
// cards.Publisher. Reconnect recovery is a full snapshot re-render on the read
// path (see EventsHandler), so the bus keeps no history.
type Broker struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

// New returns a Broker.
func New() *Broker {
	return &Broker{subs: make(map[*Subscription]struct{})}
}

// Subscription is one client's live event channel. Close to unsubscribe.
type Subscription struct {
	Events <-chan cards.Event

	broker *Broker
	ch     chan cards.Event
}

// Subscribe registers a new subscriber for live events.
func (b *Broker) Subscribe() *Subscription {
	ch := make(chan cards.Event, 16)
	sub := &Subscription{Events: ch, broker: b, ch: ch}

	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return sub
}

// Publish fans an event to every current subscriber. Delivery is
// non-blocking: a subscriber whose buffer is full is skipped rather than
// stalling the origin. A dropped client recovers on its next EventSource
// reconnect, which re-renders the full column.
func (b *Broker) Publish(e cards.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subs {
		select {
		case sub.ch <- e:
		default: // slow consumer — drop; it'll catch up on reconnect
		}
	}
}

// Close unsubscribes; safe to call once.
func (s *Subscription) Close() {
	s.broker.mu.Lock()
	delete(s.broker.subs, s)
	s.broker.mu.Unlock()
}
