// SSE read-path tests: EventsHandler over a real server (a recorder can't be
// read while a streaming handler writes). Shared fakes live in blocks_test.go.
package frontend

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	"github.com/GVPproj/unbusy.day/internal/web"
)

// openEvents connects to an SSE handler over a real server (a recorder can't
// be read while a streaming handler writes) and returns a frame reader.
func openEvents(t *testing.T, h http.Handler) (*http.Response, *bufio.Reader) {
	t.Helper()
	// Stand in for RequireSession: handlers read the owner from the context.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(web.WithOwner(r.Context(), testOwner)))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp, bufio.NewReader(resp.Body)
}

// readFrame returns the next SSE frame, skipping `:keepalive`-style comment
// frames; fails the test on EOF/timeout.
func readFrame(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	type result struct {
		frame string
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		var b strings.Builder
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				ch <- result{b.String(), err}
				return
			}
			if line == "\n" {
				if s := b.String(); s != "" && !strings.HasPrefix(s, ":") {
					ch <- result{s, nil}
					return
				}
				b.Reset()
				continue
			}
			b.WriteString(line)
		}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("read frame: %v (got %q)", r.err, r.frame)
		}
		return r.frame
	case <-time.After(2 * time.Second):
		t.Fatal("no SSE frame within deadline")
		return ""
	}
}

// A (re)connect is made whole by one full-column patch — no replay needed.
// Also pins the connection-hardening headers.
func TestEventsConnectShipsAuthoritativeColumn(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	resp, br := openEvents(t, EventsHandler(svc, newFakeJot(), broker))

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type: want text/event-stream prefix, got %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("cache-control: want no-cache, got %q", cc)
	}
	if ab := resp.Header.Get("X-Accel-Buffering"); ab != "no" {
		t.Errorf("x-accel-buffering: want %q, got %q", "no", ab)
	}

	frame := readFrame(t, br)
	if !strings.Contains(frame, "datastar-patch-elements") {
		t.Errorf("missing datastar-patch-elements event; frame:\n%s", frame)
	}
	if !strings.Contains(frame, `id="block-list"`) {
		t.Errorf("patch missing #block-list morph anchor; frame:\n%s", frame)
	}
	assertOrder(t, frame, "a", "b", "c")
}

// A mutation published on the shared bus reaches subscribers as an element
// patch in the committed order — what moves blocks in another open tab.
func TestEventsStreamsPublishedReordersAsPatches(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, newFakeJot(), broker))
	readFrame(t, br) // connect snapshot
	readFrame(t, br) // connect envelope signals
	readFrame(t, br) // connect jot snapshot signals

	broker.Publish(block.Event{Owner: testOwner, Blocks: []block.Block{
		{ID: "b", Label: "Bravo", Position: 0},
		{ID: "c", Label: "Charlie", Position: 1},
		{ID: "a", Label: "Alpha", Position: 2},
	}})

	frame := readFrame(t, br)
	if !strings.Contains(frame, "datastar-patch-elements") {
		t.Errorf("missing datastar-patch-elements event; frame:\n%s", frame)
	}
	assertOrder(t, frame, "b", "c", "a")
}

func TestEventsStreamsPublishedBounds(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, newFakeJot(), broker))
	readFrame(t, br) // connect snapshot
	readFrame(t, br) // connect envelope signals
	readFrame(t, br) // connect jot snapshot signals

	broker.Publish(block.Event{Owner: testOwner, Blocks: threeBlocks(),
		Bounds: block.Bounds{Start: 17, End: 21}})

	frame := readFrame(t, br)
	for _, want := range []string{`data-day-start="17"`, `data-day-end="21"`} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing %q; frame:\n%s", want, frame)
		}
	}
}

// Keepalives stop intermediaries (browser/NAT/Cloudflare) reaping idle
// streams. Interval shrunk for the test; production cadence is 25s.
func TestEventsEmitsKeepaliveComments(t *testing.T) {
	old := keepaliveInterval
	keepaliveInterval = 20 * time.Millisecond
	t.Cleanup(func() { keepaliveInterval = old })

	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()
	_, br := openEvents(t, EventsHandler(svc, newFakeJot(), broker))

	deadline := time.After(2 * time.Second)
	lines := make(chan string)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- line
		}
	}()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before keepalive")
			}
			if strings.HasPrefix(line, ":keepalive") {
				return // heartbeat observed
			}
		case <-deadline:
			t.Fatal("no :keepalive comment within deadline")
		}
	}
}

// Every column patch carries the recomputed occupied envelope as patch-signals,
// so the once-rendered bounds modal's disabled options track the live layout.
func TestEventsPatchesEnvelopeSignals(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, newFakeJot(), broker))
	readFrame(t, br) // connect: column element patch
	// Connect also re-seeds the envelope so a reconnect after a change is current.
	sig := readFrame(t, br)
	if !strings.Contains(sig, "datastar-patch-signals") {
		t.Fatalf("connect missing patch-signals frame; frame:\n%s", sig)
	}
	for _, want := range []string{`"firstOccupiedSlot":18`, `"lastOccupiedEnd":21`} {
		if !strings.Contains(sig, want) {
			t.Errorf("connect envelope missing %q; frame:\n%s", want, sig)
		}
	}
	readFrame(t, br) // connect jot snapshot signals

	broker.Publish(block.Event{Owner: testOwner, Blocks: []block.Block{
		{ID: "a", Position: 12, Span: 2}, // occupies 12,13 → end 14
	}})
	readFrame(t, br) // element patch
	sig = readFrame(t, br)
	if !strings.Contains(sig, "datastar-patch-signals") {
		t.Fatalf("publish missing patch-signals frame; frame:\n%s", sig)
	}
	for _, want := range []string{`"firstOccupiedSlot":12`, `"lastOccupiedEnd":14`} {
		if !strings.Contains(sig, want) {
			t.Errorf("publish envelope missing %q; frame:\n%s", want, sig)
		}
	}
}

// The connect frame set includes the jot snapshot as signal patches, so a
// reconnecting editor is made whole without an element ever touching it.
func TestEventsConnectShipsTheJotSnapshotAsSignals(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	jots := newFakeJot()
	jots.pads[testOwner] = jot.Pad{Text: "notes from the phone", Version: 7}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, jots, broker))
	readFrame(t, br) // column element patch
	readFrame(t, br) // envelope signals

	frame := readFrame(t, br)
	if !strings.Contains(frame, "datastar-patch-signals") {
		t.Fatalf("jot snapshot must be a signal patch; frame:\n%s", frame)
	}
	for _, want := range []string{`"_jotv":7`, `"_jott":"notes from the phone"`} {
		if !strings.Contains(frame, want) {
			t.Errorf("jot snapshot missing %q; frame:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "datastar-patch-elements") {
		t.Errorf("jot state must never ride an element patch; frame:\n%s", frame)
	}
}

// A published jot event reaches the owner's other open devices live, again as
// a signal patch only.
func TestEventsStreamsPublishedJotEventsAsSignals(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, newFakeJot(), broker))
	readFrame(t, br) // connect snapshot
	readFrame(t, br) // connect envelope signals
	readFrame(t, br) // connect jot snapshot signals

	broker.PublishJot(jot.Event{Owner: testOwner, Version: 8, Text: "typed on device A"})

	frame := readFrame(t, br)
	if !strings.Contains(frame, "datastar-patch-signals") {
		t.Fatalf("jot event must be a signal patch; frame:\n%s", frame)
	}
	for _, want := range []string{`"_jotv":8`, `"_jott":"typed on device A"`} {
		if !strings.Contains(frame, want) {
			t.Errorf("jot event missing %q; frame:\n%s", want, frame)
		}
	}
}
