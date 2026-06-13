package pubsub_test

import (
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
)

func recv(t *testing.T, ch <-chan block.Event) block.Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return block.Event{}
	}
}

func firstID(e block.Event) string {
	if len(e.Blocks) == 0 {
		return ""
	}
	return e.Blocks[0].ID
}

// One published event fans out to every subscriber of its owner.
func TestPublishFansOutToAllSubscribers(t *testing.T) {
	b := pubsub.New()
	a := b.Subscribe("u1")
	defer a.Close()
	c := b.Subscribe("u1")
	defer c.Close()

	b.Publish(block.Event{Owner: "u1", Blocks: []block.Block{{ID: "a"}}})

	if got := firstID(recv(t, a.Events)); got != "a" {
		t.Fatalf("sub a block = %q, want a", got)
	}
	if got := firstID(recv(t, c.Events)); got != "a" {
		t.Fatalf("sub c block = %q, want a", got)
	}
}

// A subscriber that never drains its channel must not stall Publish or starve
// other subscribers. The bus drops to the slow consumer (which recovers on
// reconnect) rather than blocking the origin.
func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	b := pubsub.New()
	slow := b.Subscribe("u1") // never read
	defer slow.Close()
	fast := b.Subscribe("u1")
	defer fast.Close()

	// Far more events than any channel buffer; if Publish blocked on slow,
	// this goroutine would never finish.
	done := make(chan struct{})
	go func() {
		for range 1000 {
			b.Publish(block.Event{Owner: "u1", Blocks: []block.Block{{ID: "a"}}})
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
	b := pubsub.New()
	sub := b.Subscribe("u1")
	defer sub.Close()

	b.Publish(block.Event{Owner: "u1", Blocks: []block.Block{{ID: "a"}}})

	if got := firstID(recv(t, sub.Events)); got != "a" {
		t.Fatalf("block = %q, want a", got)
	}
}

// Keyed fan-out (ADR 0003): another user's subscriber never receives the
// event — its connection doesn't even wake.
func TestPublishIsScopedToOwner(t *testing.T) {
	b := pubsub.New()
	mine := b.Subscribe("u1")
	defer mine.Close()
	other := b.Subscribe("u2")
	defer other.Close()

	b.Publish(block.Event{Owner: "u1", Blocks: []block.Block{{ID: "a"}}})

	if got := firstID(recv(t, mine.Events)); got != "a" {
		t.Fatalf("owner sub block = %q, want a", got)
	}
	select {
	case e := <-other.Events:
		t.Fatalf("foreign subscriber received %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}
