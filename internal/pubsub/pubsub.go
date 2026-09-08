// Package pubsub is the in-process fan-out bus, keyed by user (ADR 0003).
// Single-machine only — cross-instance fan-out would need an external bus.
package pubsub

import (
	"sync"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/jot"
)

// Broker fans block, jot, and habit events to the owner's live subscribers.
// It keeps no history; reconnect recovery reads current state.
type Broker struct {
	mu   sync.Mutex
	subs map[string]map[*Subscription]struct{} // owner -> subscribers
}

func New() *Broker {
	return &Broker{subs: make(map[string]map[*Subscription]struct{})}
}

// Subscription is one client's live event channels. Close to unsubscribe.
type Subscription struct {
	Events <-chan block.Event
	Jots   <-chan jot.Event
	Habits <-chan habit.Event
	broker *Broker
	owner  string
	ch     chan block.Event
	jch    chan jot.Event
	hch    chan habit.Event
}

func (b *Broker) Subscribe(owner string) *Subscription {
	ch := make(chan block.Event, 16)
	jch := make(chan jot.Event, 16)
	hch := make(chan habit.Event, 16)
	sub := &Subscription{Events: ch, Jots: jch, Habits: hch, broker: b, owner: owner, ch: ch, jch: jch, hch: hch}

	b.mu.Lock()
	if b.subs[owner] == nil {
		b.subs[owner] = make(map[*Subscription]struct{})
	}
	b.subs[owner][sub] = struct{}{}
	b.mu.Unlock()
	return sub
}

// Publish fans an event to the owner's subscribers, non-blocking: a slow
// consumer is skipped and recovers on its next EventSource reconnect.
func (b *Broker) Publish(e block.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subs[e.Owner] {
		select {
		case sub.ch <- e:
		default:
		}
	}
}

// PublishJot fans a Jotpad event to the owner's subscribers, non-blocking like
// Publish: a slow consumer recovers via the snapshot on reconnect.
func (b *Broker) PublishJot(e jot.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subs[e.Owner] {
		select {
		case sub.jch <- e:
		default:
		}
	}
}

// PublishHabit fans an owner-scoped invalidation without blocking slow readers.
func (b *Broker) PublishHabit(e habit.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs[e.Owner] {
		select {
		case sub.hch <- e:
		default:
		}
	}
}

// Close unsubscribes.
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
