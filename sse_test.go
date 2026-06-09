package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grahamvanpelt/unbusy.day/cards"
	"github.com/grahamvanpelt/unbusy.day/pubsub"
)

// connect opens the events stream and returns a scanner over the body plus the
// response. The caller closes resp.Body.
func connect(t *testing.T, url, lastEventID string) (*bufio.Scanner, *http.Response) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	return bufio.NewScanner(resp.Body), resp
}

// nextFrame collects scanner lines until a blank separator, with a timeout.
func nextFrame(t *testing.T, sc *bufio.Scanner) []string {
	t.Helper()
	type res struct{ lines []string }
	ch := make(chan res, 1)
	go func() {
		var lines []string
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				if len(lines) > 0 {
					break
				}
				continue // skip leading blanks / keepalive separators
			}
			if strings.HasPrefix(line, ":") {
				continue // comment / keepalive
			}
			lines = append(lines, line)
		}
		ch <- res{lines}
	}()
	select {
	case r := <-ch:
		return r.lines
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE frame")
		return nil
	}
}

func TestEvents_StreamsPublishedEvent(t *testing.T) {
	b := pubsub.New(1024)
	srv := httptest.NewServer(eventsHandler(b))
	defer srv.Close()

	sc, resp := connect(t, srv.URL, "")
	defer resp.Body.Close()

	// Give the handler a moment to subscribe before publishing.
	time.Sleep(50 * time.Millisecond)
	b.Publish(cards.Event{Txid: "42", Cards: []cards.Card{{ID: "a", Label: "Alpha", Position: 0}}})

	frame := nextFrame(t, sc)
	joined := strings.Join(frame, "\n")
	if !strings.Contains(joined, "id: 42") {
		t.Fatalf("frame missing id: 42:\n%s", joined)
	}
	if !strings.Contains(joined, `"id":"a"`) {
		t.Fatalf("frame missing card data:\n%s", joined)
	}
}

func TestEvents_ReplaysFromLastEventID(t *testing.T) {
	b := pubsub.New(1024)
	b.Publish(cards.Event{Txid: "5"})
	b.Publish(cards.Event{Txid: "6"})
	b.Publish(cards.Event{Txid: "7"})

	srv := httptest.NewServer(eventsHandler(b))
	defer srv.Close()

	sc, resp := connect(t, srv.URL, "5")
	defer resp.Body.Close()

	// First replayed frame should be txid 6 (events after the cursor).
	frame := nextFrame(t, sc)
	if joined := strings.Join(frame, "\n"); !strings.Contains(joined, "id: 6") {
		t.Fatalf("first replay frame = %q, want id: 6", joined)
	}
}

func TestEvents_OverflowSentinel(t *testing.T) {
	b := pubsub.New(2) // tiny ring forces eviction
	for _, id := range []string{"1", "2", "3", "4"} {
		b.Publish(cards.Event{Txid: id}) // holds 3,4; 1,2 evicted
	}

	srv := httptest.NewServer(eventsHandler(b))
	defer srv.Close()

	sc, resp := connect(t, srv.URL, "1") // cursor predates window
	defer resp.Body.Close()

	frame := nextFrame(t, sc)
	if joined := strings.Join(frame, "\n"); !strings.Contains(joined, "event: overflow") {
		t.Fatalf("frame = %q, want overflow sentinel", joined)
	}
}

func TestEvents_HardeningHeaders(t *testing.T) {
	b := pubsub.New(1024)
	srv := httptest.NewServer(eventsHandler(b))
	defer srv.Close()

	_, resp := connect(t, srv.URL, "")
	defer resp.Body.Close()

	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", got)
	}
}
