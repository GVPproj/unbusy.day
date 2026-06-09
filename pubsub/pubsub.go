// Package pubsub is the in-process fan-out bus (PRD §5, M1b): one mutation
// event is delivered to every live SSE subscriber. Single-machine only —
// cross-instance fan-out is the §9 scale-out path, by design out of scope.
package pubsub

import (
	"strconv"
	"sync"

	"github.com/grahamvanpelt/unbusy.day/cards"
)

// Broker fans cards.Events to subscribers and retains recent events in a ring
// for Last-Event-ID replay. It implements cards.Publisher.
type Broker struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}

	// ring holds the most-recent published events (a contiguous suffix of
	// history) for Last-Event-ID replay. Oldest first; capped at ringSize.
	ring     []cards.Event
	ringSize int
	evicted  bool // true once the ring has dropped an event (a gap can exist)
}

// New returns a Broker. ringSize bounds the replay buffer (PRD F2: 1024).
func New(ringSize int) *Broker {
	return &Broker{
		subs:     make(map[*Subscription]struct{}),
		ringSize: ringSize,
	}
}

// Subscription is one client's view of the stream: a channel of live events
// plus, on connect, any replayed backlog. Close to unsubscribe.
type Subscription struct {
	Events <-chan cards.Event

	// Replay holds events newer than the reconnect cursor, ordered oldest
	// first, to be flushed before live events.
	Replay []cards.Event

	// Overflow is set when the cursor predates the retained ring window: the
	// gap can't be replayed, so the client must fall back to a full refetch.
	Overflow bool

	broker *Broker
	ch     chan cards.Event
}

// Subscribe registers a new subscriber. lastEventID is the EventSource
// reconnect cursor (empty on first connect). Replay snapshot and registration
// happen under one lock so no event published concurrently is lost or dropped
// between the snapshot and going live.
func (b *Broker) Subscribe(lastEventID string) *Subscription {
	ch := make(chan cards.Event, 16)
	sub := &Subscription{Events: ch, broker: b, ch: ch}

	b.mu.Lock()
	sub.Replay, sub.Overflow = b.replayLocked(lastEventID)
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return sub
}

// replayLocked returns ring events with txid greater than the cursor (oldest
// first) and whether the cursor overflowed the retained window. An empty or
// unparseable cursor replays nothing (fresh connect → live only).
//
// Overflow means the cursor predates the oldest retained event *and* the ring
// has evicted: there may be an unrecoverable gap, so the caller refetches.
// This is deliberately conservative at the window boundary — a needless
// refetch is correct, a silently-missed event is not.
func (b *Broker) replayLocked(lastEventID string) (replay []cards.Event, overflow bool) {
	if lastEventID == "" {
		return nil, false
	}
	cursor, err := strconv.ParseUint(lastEventID, 10, 64)
	if err != nil {
		return nil, false
	}
	if b.evicted && len(b.ring) > 0 {
		if oldest, err := strconv.ParseUint(b.ring[0].Txid, 10, 64); err == nil && cursor < oldest {
			return nil, true
		}
	}
	for _, e := range b.ring {
		// txids are pg xid8 (64-bit unsigned); compare numerically so
		// "10" sorts after "9" (PRD §11).
		if id, err := strconv.ParseUint(e.Txid, 10, 64); err == nil && id > cursor {
			replay = append(replay, e)
		}
	}
	return replay, false
}

// Publish fans an event to every current subscriber. Delivery is
// non-blocking: a subscriber whose buffer is full is skipped rather than
// stalling the origin. A dropped client recovers on its next EventSource
// reconnect, replaying the gap from the ring via Last-Event-ID (PRD F2).
func (b *Broker) Publish(e cards.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Retain for replay, evicting the oldest once at capacity.
	b.ring = append(b.ring, e)
	if len(b.ring) > b.ringSize {
		b.ring = b.ring[1:]
		b.evicted = true
	}

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
