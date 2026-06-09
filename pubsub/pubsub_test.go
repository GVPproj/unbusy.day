package pubsub_test

import (
	"testing"
	"time"

	"github.com/grahamvanpelt/unbusy.day/cards"
	"github.com/grahamvanpelt/unbusy.day/pubsub"
)

func recv(t *testing.T, ch <-chan cards.Event) cards.Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return cards.Event{}
	}
}

// Reconnecting with Last-Event-ID replays only events newer than the cursor,
// in order, comparing txids numerically (so "10" > "9", not lexically).
func TestReplayReturnsEventsAfterCursor(t *testing.T) {
	b := pubsub.New(1024)
	for _, id := range []string{"8", "9", "10", "11"} {
		b.Publish(cards.Event{Txid: id})
	}

	sub := b.Subscribe("9")
	defer sub.Close()

	if sub.Overflow {
		t.Fatal("unexpected overflow with cursor inside the ring")
	}
	got := make([]string, len(sub.Replay))
	for i, e := range sub.Replay {
		got[i] = e.Txid
	}
	want := []string{"10", "11"}
	if len(got) != len(want) {
		t.Fatalf("replay = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("replay = %v, want %v", got, want)
		}
	}
}

// When the ring has evicted past the cursor, the gap is unrecoverable: signal
// overflow and replay nothing so the client does a full refetch (PRD F2).
func TestOverflowWhenCursorEvicted(t *testing.T) {
	b := pubsub.New(3) // tiny ring to force eviction
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		b.Publish(cards.Event{Txid: id}) // ring now holds 3,4,5; 1,2 evicted
	}

	sub := b.Subscribe("1") // cursor predates oldest retained (3)
	defer sub.Close()

	if !sub.Overflow {
		t.Fatal("want overflow when cursor is older than the retained window")
	}
	if len(sub.Replay) != 0 {
		t.Fatalf("replay = %v, want empty on overflow", sub.Replay)
	}
}

// A cursor still inside a full ring replays cleanly — no false overflow.
func TestNoOverflowWhenCursorRetained(t *testing.T) {
	b := pubsub.New(3)
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		b.Publish(cards.Event{Txid: id}) // holds 3,4,5
	}

	sub := b.Subscribe("3") // oldest retained — events >3 (4,5) are all present
	defer sub.Close()

	if sub.Overflow {
		t.Fatal("unexpected overflow: events after cursor are all retained")
	}
	if len(sub.Replay) != 2 {
		t.Fatalf("replay = %v, want [4 5]", sub.Replay)
	}
}

// Eviction alone isn't overflow: if nothing was evicted past the cursor, a
// cursor below the oldest retained txid still replays the full tail.
func TestNoOverflowWhenRingNotFull(t *testing.T) {
	b := pubsub.New(1024)
	for _, id := range []string{"5", "6", "7"} {
		b.Publish(cards.Event{Txid: id})
	}

	sub := b.Subscribe("1") // below oldest, but nothing was ever evicted
	defer sub.Close()

	if sub.Overflow {
		t.Fatal("unexpected overflow: ring never evicted")
	}
	if len(sub.Replay) != 3 {
		t.Fatalf("replay = %v, want [5 6 7]", sub.Replay)
	}
}

// A fresh connection (no cursor) replays nothing — it only gets live events.
func TestNoCursorReplaysNothing(t *testing.T) {
	b := pubsub.New(1024)
	b.Publish(cards.Event{Txid: "1"})

	sub := b.Subscribe("")
	defer sub.Close()

	if len(sub.Replay) != 0 {
		t.Fatalf("replay = %v, want empty", sub.Replay)
	}
}

// One published event fans out to every subscriber.
func TestPublishFansOutToAllSubscribers(t *testing.T) {
	b := pubsub.New(1024)
	a := b.Subscribe("")
	defer a.Close()
	c := b.Subscribe("")
	defer c.Close()

	b.Publish(cards.Event{Txid: "7"})

	if got := recv(t, a.Events); got.Txid != "7" {
		t.Fatalf("sub a txid = %q, want 7", got.Txid)
	}
	if got := recv(t, c.Events); got.Txid != "7" {
		t.Fatalf("sub c txid = %q, want 7", got.Txid)
	}
}

// A subscriber that never drains its channel must not stall Publish or
// starve other subscribers. The bus drops to the slow consumer (which then
// reconnects with Last-Event-ID and replays) rather than blocking the origin.
func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	b := pubsub.New(1024)
	slow := b.Subscribe("") // never read
	defer slow.Close()
	fast := b.Subscribe("")
	defer fast.Close()

	// Far more events than any channel buffer; if Publish blocked on slow,
	// this goroutine would never finish.
	done := make(chan struct{})
	go func() {
		for i := range 1000 {
			b.Publish(cards.Event{Txid: string(rune('0' + i%10))})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	// The fast subscriber still receives (delivery wasn't serialised behind slow).
	select {
	case <-fast.Events:
	default:
		t.Fatal("fast subscriber received nothing")
	}
}

// Tracer: a subscriber receives an event published after it subscribed.
func TestSubscribeReceivesPublishedEvent(t *testing.T) {
	b := pubsub.New(1024)
	sub := b.Subscribe("")
	defer sub.Close()

	b.Publish(cards.Event{Txid: "10", Cards: []cards.Card{{ID: "a"}}})

	got := recv(t, sub.Events)
	if got.Txid != "10" {
		t.Fatalf("txid = %q, want 10", got.Txid)
	}
}
