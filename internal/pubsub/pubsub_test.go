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
// other subscribers — the bus drops to slow consumers rather than blocking.
func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	b := pubsub.New()
	slow := b.Subscribe("u1") // never read
	defer slow.Close()
	fast := b.Subscribe("u1")
	defer fast.Close()

	// Far more events than any channel buffer.
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

	// Delivery must not have been serialised behind the slow subscriber.
	select {
	case <-fast.Events:
	default:
		t.Fatal("fast subscriber received nothing")
	}
}

func TestSubscribeReceivesPublishedEvent(t *testing.T) {
	b := pubsub.New()
	sub := b.Subscribe("u1")
	defer sub.Close()

	b.Publish(block.Event{Owner: "u1", Blocks: []block.Block{{ID: "a"}}})

	if got := firstID(recv(t, sub.Events)); got != "a" {
		t.Fatalf("block = %q, want a", got)
	}
}

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
