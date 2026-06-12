// Package pubsub is the in-process fan-out bus, keyed by user (ADR 0003): a
// mutation event reaches only its owner's live SSE subscribers. Single-machine
// only — cross-instance fan-out would need an external bus (LISTEN/NOTIFY or
// Redis).
package pubsub

import (
	"sync"

	"github.com/GVPproj/unbusy.day/cards"
)

// Broker fans cards.Events to the owner's live subscribers. It implements
// cards.Publisher. Reconnect recovery is a full snapshot re-render on the read
// path (see EventsHandler), so the bus keeps no history.
type Broker struct {
	mu   sync.Mutex
	subs map[string]map[*Subscription]struct{} // owner -> subscribers
}

// New returns a Broker.
func New() *Broker {
	return &Broker{subs: make(map[string]map[*Subscription]struct{})}
}

// Subscription is one client's live event channel. Close to unsubscribe.
type Subscription struct {
	Events <-chan cards.Event

	broker *Broker
	owner  string
	ch     chan cards.Event
}

// Subscribe registers a new subscriber for owner's events.
func (b *Broker) Subscribe(owner string) *Subscription {
	ch := make(chan cards.Event, 16)
	sub := &Subscription{Events: ch, broker: b, owner: owner, ch: ch}

	b.mu.Lock()
	if b.subs[owner] == nil {
		b.subs[owner] = make(map[*Subscription]struct{})
	}
	b.subs[owner][sub] = struct{}{}
	b.mu.Unlock()
	return sub
}

// Publish fans an event to the owner's current subscribers only — other
// users' connections never wake (no cross-user activity timing leak).
// Delivery is non-blocking: a subscriber whose buffer is full is skipped
// rather than stalling the origin; it recovers on its next EventSource
// reconnect, which re-renders the full column.
func (b *Broker) Publish(e cards.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subs[e.Owner] {
		select {
		case sub.ch <- e:
		default: // slow consumer — drop; it'll catch up on reconnect
		}
	}
}

// Close unsubscribes; safe to call once.
func (s *Subscription) Close() {
	s.broker.mu.Lock()
	if set := s.broker.subs[s.owner]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(s.broker.subs, s.owner)
		}
	}
	s.broker.mu.Unlock()
}
