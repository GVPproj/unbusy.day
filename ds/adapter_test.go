// M2.5b tests: Adapter B (PRD F12–F15) over the shared cards core. The DB is
// the system boundary, so a fake CardService stands in for *cards.Service;
// the pub/sub Broker and templ rendering are real — tests pin observable wire
// behavior (frames, fragment ids, order), not SDK internals.
package ds

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grahamvanpelt/unbusy.day/cards"
	"github.com/grahamvanpelt/unbusy.day/ds/components"
	"github.com/grahamvanpelt/unbusy.day/pubsub"
)

// fakeService implements CardService without Postgres. Reorder applies the
// requested order to the in-memory cards like the real service does, or
// returns reorderErr if set.
type fakeService struct {
	cards      []cards.Card
	txid       string
	listErr    error
	reorderErr error

	gotOrder []string // order passed to Reorder, for asserting delegation
}

func (f *fakeService) List(ctx context.Context) ([]cards.Card, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.cards, nil
}

func (f *fakeService) Reorder(ctx context.Context, order []string) (*cards.ReorderResult, error) {
	f.gotOrder = order
	if f.reorderErr != nil {
		return nil, f.reorderErr
	}
	byID := make(map[string]cards.Card, len(f.cards))
	for _, c := range f.cards {
		byID[c.ID] = c
	}
	out := make([]cards.Card, 0, len(order))
	for i, id := range order {
		c := byID[id]
		c.Position = i
		out = append(out, c)
	}
	f.cards = out
	return &cards.ReorderResult{Cards: out, Txid: f.txid}, nil
}

func threeCards() []cards.Card {
	return []cards.Card{
		{ID: "a", Label: "Alpha", Position: 0},
		{ID: "b", Label: "Bravo", Position: 1},
		{ID: "c", Label: "Charlie", Position: 2},
	}
}

// assertOrder checks that the ids appear in body in the given order — the
// observable contract of a server-rendered column (F14) without pinning
// markup details beyond the data-id anchors dragInit reads (F16).
func assertOrder(t *testing.T, body string, ids ...string) {
	t.Helper()
	last := -1
	for _, id := range ids {
		marker := `data-id="` + id + `"`
		i := strings.Index(body, marker)
		if i < 0 {
			t.Fatalf("body missing %s; body:\n%s", marker, body)
		}
		if i < last {
			t.Fatalf("ids out of order: %s appears before previous id; body:\n%s", id, body)
		}
		last = i
	}
}

// openEvents connects to an SSE handler over a real server (the recorder
// can't be read while a streaming handler writes) and returns a frame reader.
// The connection dies with the test via context cancellation.
func openEvents(t *testing.T, h http.Handler) (*http.Response, *bufio.Reader) {
	t.Helper()
	srv := httptest.NewServer(h)
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

// readFrame returns the next SSE frame (terminated by a blank line), skipping
// `:keepalive`-style comment frames. Fails the test on EOF/timeout — callers
// always expect a frame.
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

// TestEventsConnectShipsAuthoritativeColumn pins F13's connect/reconnect
// contract: on every connect the server renders the current authoritative
// column as an element patch. That replaces FE1's ring-buffer/Last-Event-ID
// replay — a (re)connecting FE2 client is always made whole by one frame.
// Also pins the F2 hardening headers F13 reuses.
func TestEventsConnectShipsAuthoritativeColumn(t *testing.T) {
	svc := &fakeService{cards: threeCards()}
	broker := pubsub.New(16)

	resp, br := openEvents(t, EventsHandler(svc, broker))

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
	if !strings.Contains(frame, `id="card-list"`) {
		t.Errorf("patch missing #card-list morph anchor; frame:\n%s", frame)
	}
	assertOrder(t, frame, "a", "b", "c")
}

// TestEventsStreamsPublishedReordersAsPatches pins the criterion-9 fan-out
// path (F13/F14): one mutation published on the shared bus — by either
// adapter's reorder handler — reaches FE2 subscribers as an element patch in
// the committed order. This is what makes a reorder in FE1 move the cards in
// an open FE2 tab.
func TestEventsStreamsPublishedReordersAsPatches(t *testing.T) {
	svc := &fakeService{cards: threeCards()}
	broker := pubsub.New(16)

	_, br := openEvents(t, EventsHandler(svc, broker))
	readFrame(t, br) // connect snapshot (covered by its own test)

	broker.Publish(cards.Event{Txid: "202", Cards: []cards.Card{
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

// TestEventsEmitsKeepaliveComments pins the F2 heartbeat F13 reuses: on an
// idle stream the server emits `:keepalive` comment frames so intermediaries
// (browser/NAT/Cloudflare) don't reap the connection. Interval shrunk for the
// test; production cadence is 25s (PRD F2).
func TestEventsEmitsKeepaliveComments(t *testing.T) {
	old := keepaliveInterval
	keepaliveInterval = 20 * time.Millisecond
	t.Cleanup(func() { keepaliveInterval = old })

	svc := &fakeService{cards: threeCards()}
	broker := pubsub.New(16)
	_, br := openEvents(t, EventsHandler(svc, broker))

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

// TestReorderDelegatesToCoreAndPatchesNewOrder pins F12's contract:
// POST /ds/cards/reorder carries the order as Datastar signals (JSON body
// {"order": [...]}, what @post ships), delegates to the same core mutation as
// F1, and responds with an SSE element-patch of the post-mutation column so
// the dragging client settles on the committed order. The patch must anchor
// on #card-list — without that id the outer morph is a silent no-op (F16).
func TestReorderDelegatesToCoreAndPatchesNewOrder(t *testing.T) {
	svc := &fakeService{cards: threeCards(), txid: "101"}

	req := httptest.NewRequest(http.MethodPost, "/ds/cards/reorder",
		strings.NewReader(`{"order":["c","a","b"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ReorderHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
	}
	if got, want := strings.Join(svc.gotOrder, ","), "c,a,b"; got != want {
		t.Errorf("core Reorder called with %q, want %q", got, want)
	}

	body := rec.Body.String()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type: want text/event-stream prefix, got %q", ct)
	}
	if !strings.Contains(body, "datastar-patch-elements") {
		t.Errorf("missing datastar-patch-elements event; body:\n%s", body)
	}
	if !strings.Contains(body, `id="card-list"`) {
		t.Errorf("patch missing #card-list morph anchor; body:\n%s", body)
	}
	assertOrder(t, body, "c", "a", "b")
}

// TestReorderRejectionPatchesAuthoritativeOrder pins FE2's rollback contract
// (the F5 analogue, F15): when the core rejects the order (stale /
// non-permutation), the response is a patch of the *current authoritative*
// column. The dropped card visibly snaps back because the server re-asserts
// truth — no client-side rollback machinery, which is the point of the
// server-driven choice recorded in ds/NOTES.md.
func TestReorderRejectionPatchesAuthoritativeOrder(t *testing.T) {
	svc := &fakeService{cards: threeCards(), reorderErr: cards.ErrNotPermutation}

	req := httptest.NewRequest(http.MethodPost, "/ds/cards/reorder",
		strings.NewReader(`{"order":["c","a","zzz"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ReorderHandler(svc).ServeHTTP(rec, req)

	// 200, not 4xx: the response is hypermedia ("here is the truth"), and the
	// only patch-application behavior verified in M2.5a is on OK responses.
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "datastar-patch-elements") {
		t.Errorf("missing datastar-patch-elements event; body:\n%s", body)
	}
	assertOrder(t, body, "a", "b", "c") // authoritative order, not the rejected one
}

// TestColumnUsesVerifiedDatastarKeyedAttributeSyntax pins the browser-verified
// v1.0.2 wiring (ds/NOTES.md, F16): keyed Datastar attributes separate plugin
// and key with a COLON (`data-on:reorder`, `data-signals:order`). The dash
// forms (`data-on-reorder`) are looked up as nonexistent plugin names and
// silently skipped — no console error, the drop just never POSTs. This test
// exists because that exact regression shipped once.
func TestColumnUsesVerifiedDatastarKeyedAttributeSyntax(t *testing.T) {
	var b strings.Builder
	if err := components.CardColumn(threeCards()).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	for _, attr := range []string{`data-on:reorder=`, `data-signals:order=`} {
		if !strings.Contains(body, attr) {
			t.Errorf("column missing verified keyed attribute %q; body:\n%s", attr, body)
		}
	}
	for _, stale := range []string{`data-on-reorder`, `data-signals-order`} {
		if strings.Contains(body, stale) {
			t.Errorf("column carries dash-form attribute %q — a silent no-op on Datastar v1.0.2; body:\n%s", stale, body)
		}
	}
}

// TestPageRendersColumnInServiceOrder is the M2.5b tracer bullet (F14):
// GET /ds/ renders the column server-side, cards in the order the core
// service returns them, and wires the page to the live stream (/ds/events)
// so foreign reorders arrive as patches.
func TestPageRendersColumnInServiceOrder(t *testing.T) {
	svc := &fakeService{cards: threeCards()}

	req := httptest.NewRequest(http.MethodGet, "/ds/", nil)
	rec := httptest.NewRecorder()
	PageHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type: want text/html prefix, got %q", ct)
	}

	body := rec.Body.String()
	assertOrder(t, body, "a", "b", "c")
	for _, label := range []string{"Alpha", "Bravo", "Charlie"} {
		if !strings.Contains(body, label) {
			t.Errorf("body missing card label %q", label)
		}
	}
	if !strings.Contains(body, "/ds/events") {
		t.Errorf("body missing /ds/events SSE reference; body:\n%s", body)
	}
}
