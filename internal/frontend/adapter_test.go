// Datastar adapter tests over the shared blocks core. The DB is the system
// boundary, so a fake BlockService stands in for *block.Service; the pub/sub
// Broker and templ rendering are real — tests pin observable wire behavior
// (frames, fragment ids, order), not SDK internals.
package frontend

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
)

// fakeService implements BlockService without a real database. Reorder applies the
// requested order to the in-memory blocks like the real service does, or
// returns reorderErr if set.
type fakeService struct {
	blocks    []block.Block
	bounds    block.Bounds
	listErr   error
	layoutErr error
	boundsErr error
	createErr error
	deleteErr error
	clearErr  error
	renameErr error

	gotOwner  string            // owner passed to the last mutation, for asserting scoping
	gotLayout []block.Placement // layout passed to SetLayout
	gotBounds block.Bounds      // bounds passed to SetBounds
	gotLabel  string            // label passed to Create
	gotSlot   int               // slot passed to Create
	gotType   block.BlockType   // type passed to Create
	gotID     string            // id passed to Delete
}

func (f *fakeService) Create(ctx context.Context, owner, label string, slot int, typ block.BlockType) (*block.CreateResult, error) {
	f.gotOwner, f.gotLabel, f.gotSlot, f.gotType = owner, label, slot, typ
	if f.createErr != nil {
		return nil, f.createErr
	}
	c := block.Block{ID: "new", Label: label, Position: slot, Span: 1, Type: typ}
	f.blocks = append(f.blocks, c)
	sort.Slice(f.blocks, func(i, j int) bool { return f.blocks[i].Position < f.blocks[j].Position })
	return &block.CreateResult{Blocks: f.blocks}, nil
}

func (f *fakeService) Delete(ctx context.Context, owner, id string) (*block.DeleteResult, error) {
	f.gotOwner, f.gotID = owner, id
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	out := f.blocks[:0]
	for _, c := range f.blocks {
		if c.ID != id {
			out = append(out, c)
		}
	}
	f.blocks = out
	return &block.DeleteResult{Blocks: f.blocks}, nil
}

func (f *fakeService) Clear(ctx context.Context, owner string) (*block.ClearResult, error) {
	f.gotOwner = owner
	if f.clearErr != nil {
		return nil, f.clearErr
	}
	f.blocks = nil
	return &block.ClearResult{Blocks: f.blocks}, nil
}

func (f *fakeService) Rename(ctx context.Context, owner, id, label string) (*block.RenameResult, error) {
	f.gotOwner, f.gotID = owner, id
	if f.renameErr != nil {
		return nil, f.renameErr
	}
	for i := range f.blocks {
		if f.blocks[i].ID == id {
			f.blocks[i].Label = label
		}
	}
	return &block.RenameResult{Blocks: f.blocks}, nil
}

func (f *fakeService) SetBounds(ctx context.Context, owner string, start, end int) error {
	f.gotOwner, f.gotBounds = owner, block.Bounds{Start: start, End: end}
	if f.boundsErr != nil {
		return f.boundsErr
	}
	f.bounds = f.gotBounds
	return nil
}

func (f *fakeService) SetLayout(ctx context.Context, owner string, layout []block.Placement) (*block.LayoutResult, error) {
	f.gotOwner, f.gotLayout = owner, layout
	if f.layoutErr != nil {
		return nil, f.layoutErr
	}
	byID := make(map[string]block.Block, len(f.blocks))
	for _, c := range f.blocks {
		byID[c.ID] = c
	}
	out := make([]block.Block, 0, len(layout))
	for _, p := range layout {
		c := byID[p.ID]
		c.Position, c.Span = p.Slot, p.Span
		out = append(out, c)
	}
	// The real service returns the committed column in slot order.
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	f.blocks = out
	return &block.LayoutResult{Blocks: out}, nil
}

func (f *fakeService) List(ctx context.Context, owner string) ([]block.Block, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.blocks, nil
}

func (f *fakeService) Bounds(ctx context.Context, owner string) (block.Bounds, error) {
	if f.bounds == (block.Bounds{}) {
		return testBounds, nil
	}
	return f.bounds, nil
}

// testOwner is the authenticated user id tests inject in place of
// RequireSession. Deliberately unguessable so a hardcoded owner in a handler
// can't pass by coincidence.
const testOwner = "test-owner-7f3a"

// testBounds is the default 9:00–17:00 day the fake serves.
var testBounds = block.Bounds{Start: 18, End: 34}

// authedRequest is an httptest request carrying the owner RequireSession
// would have stashed.
func authedRequest(method, target string, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(withOwner(req.Context(), testOwner))
}

func threeBlocks() []block.Block {
	return []block.Block{
		{ID: "a", Label: "Alpha", Position: 18, Span: 1},
		{ID: "b", Label: "Bravo", Position: 19, Span: 1},
		{ID: "c", Label: "Charlie", Position: 20, Span: 1},
	}
}

// assertOrder checks that the ids appear in body in the given order — the
// observable contract of a server-rendered column, without pinning markup
// details beyond the data-id anchors DragInit reads.
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
	// Stand in for RequireSession: handlers read the owner from the context.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(withOwner(r.Context(), testOwner)))
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

// On every connect the server renders the current authoritative column as an
// element patch, so a (re)connecting client is made whole by one frame — no
// replay needed. Also pins the connection-hardening headers.
func TestEventsConnectShipsAuthoritativeColumn(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

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
	if !strings.Contains(frame, `id="block-list"`) {
		t.Errorf("patch missing #block-list morph anchor; frame:\n%s", frame)
	}
	assertOrder(t, frame, "a", "b", "c")
}

// One mutation published on the shared bus — by either adapter's reorder
// handler — reaches subscribers as an element patch in the committed order.
// This is what makes a reorder in one adapter move the blocks in the other's
// open tab.
func TestEventsStreamsPublishedReordersAsPatches(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, broker))
	readFrame(t, br) // connect snapshot (covered by its own test)
	readFrame(t, br) // connect envelope signals (covered by its own test)

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

// A published event's bounds render into the patch, so a bounds change in one
// tab resizes the grid in every other open tab.
func TestEventsStreamsPublishedBounds(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, broker))
	readFrame(t, br) // connect snapshot
	readFrame(t, br) // connect envelope signals

	broker.Publish(block.Event{Owner: testOwner, Blocks: threeBlocks(),
		Bounds: block.Bounds{Start: 17, End: 21}})

	frame := readFrame(t, br)
	for _, want := range []string{`data-day-start="17"`, `data-day-end="21"`} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing %q; frame:\n%s", want, frame)
		}
	}
}

// On an idle stream the server emits `:keepalive` comment frames so
// intermediaries (browser/NAT/Cloudflare) don't reap the connection. Interval
// shrunk for the test; production cadence is 25s.
func TestEventsEmitsKeepaliveComments(t *testing.T) {
	old := keepaliveInterval
	keepaliveInterval = 20 * time.Millisecond
	t.Cleanup(func() { keepaliveInterval = old })

	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()
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

// POST /blocks/layout carries the full proposed layout as Datastar signals
// ({"layout":[{id,slot,span},...]}, what the drag/resize gestures @post once
// the client computes the push), delegates to the core SetLayout, and responds
// with an SSE element-patch of the committed column anchored on #block-list.
func TestLayoutDelegatesToCoreAndPatchesColumn(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}

	req := authedRequest(http.MethodPost, "/blocks/layout",
		`{"layout":[{"id":"a","slot":20,"span":2},{"id":"b","slot":18,"span":1},{"id":"c","slot":19,"span":1}]}`)
	rec := httptest.NewRecorder()
	LayoutHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
	}
	if svc.gotOwner != testOwner {
		t.Errorf("core SetLayout called with owner %q, want %q", svc.gotOwner, testOwner)
	}
	want := []block.Placement{{ID: "a", Slot: 20, Span: 2}, {ID: "b", Slot: 18, Span: 1}, {ID: "c", Slot: 19, Span: 1}}
	if len(svc.gotLayout) != len(want) {
		t.Fatalf("core SetLayout called with %d placements, want %d", len(svc.gotLayout), len(want))
	}
	for i, p := range want {
		if svc.gotLayout[i] != p {
			t.Errorf("placement[%d] = %+v, want %+v", i, svc.gotLayout[i], p)
		}
	}

	body := rec.Body.String()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type: want text/event-stream prefix, got %q", ct)
	}
	if !strings.Contains(body, "datastar-patch-elements") {
		t.Errorf("missing datastar-patch-elements event; body:\n%s", body)
	}
	if !strings.Contains(body, `id="block-list"`) {
		t.Errorf("patch missing #block-list morph anchor; body:\n%s", body)
	}
	assertOrder(t, body, "b", "c", "a") // committed slot order, not submission order
}

// A layout the core rejects (overlap, out of bounds, stale block set, bad
// span) patches back the *current authoritative* column at 200 — the
// optimistic gesture visibly snaps back. Each typed domain error takes the
// rollback path; only unexpected errors 500.
func TestLayoutRejectionPatchesAuthoritativeColumn(t *testing.T) {
	for _, domainErr := range []error{
		block.ErrNotSameBlocks, block.ErrOutOfBounds, block.ErrOverlap, block.ErrInvalidSpan,
	} {
		t.Run(domainErr.Error(), func(t *testing.T) {
			svc := &fakeService{blocks: threeBlocks(), layoutErr: domainErr}

			req := authedRequest(http.MethodPost, "/blocks/layout",
				`{"layout":[{"id":"a","slot":18,"span":1},{"id":"b","slot":18,"span":1},{"id":"c","slot":19,"span":1}]}`)
			rec := httptest.NewRecorder()
			LayoutHandler(svc).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, "datastar-patch-elements") {
				t.Errorf("missing datastar-patch-elements event; body:\n%s", body)
			}
			if !strings.Contains(body, `id="block-list"`) {
				t.Errorf("patch missing #block-list morph anchor; body:\n%s", body)
			}
			assertOrder(t, body, "a", "b", "c") // authoritative column, not the rejected layout
		})
	}
}

// POST /blocks/bounds carries {"start","end"} (slot indexes), delegates to the
// core SetBounds, and responds with an element-patch of the column rendered at
// the new extent so the grid resizes in the same round-trip.
func TestBoundsDelegatesToCoreAndPatchesColumnAtNewExtent(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}

	req := authedRequest(http.MethodPost, "/blocks/bounds", `{"start":17,"end":21}`)
	rec := httptest.NewRecorder()
	BoundsHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
	}
	if svc.gotOwner != testOwner {
		t.Errorf("core SetBounds called with owner %q, want %q", svc.gotOwner, testOwner)
	}
	if svc.gotBounds != (block.Bounds{Start: 17, End: 21}) {
		t.Errorf("core SetBounds called with %+v, want {17 21}", svc.gotBounds)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "datastar-patch-elements") {
		t.Errorf("missing datastar-patch-elements event; body:\n%s", body)
	}
	if !strings.Contains(body, `id="block-list"`) {
		t.Errorf("patch missing #block-list morph anchor; body:\n%s", body)
	}
	for _, want := range []string{`data-day-start="17"`, `data-day-end="21"`} {
		if !strings.Contains(body, want) {
			t.Errorf("patch missing %q — column must render at the new extent; body:\n%s", want, body)
		}
	}
}

// A rejected bounds change (outside hard limits, or a shrink onto occupied
// slots) responds 200 with the column re-rendered at the *current* bounds —
// the plan is re-shown, nothing is lost silently.
func TestBoundsRejectionPatchesCurrentExtent(t *testing.T) {
	for _, domainErr := range []error{block.ErrInvalidBounds, block.ErrBoundsOccupied} {
		t.Run(domainErr.Error(), func(t *testing.T) {
			svc := &fakeService{blocks: threeBlocks(), boundsErr: domainErr}

			req := authedRequest(http.MethodPost, "/blocks/bounds", `{"start":19,"end":20}`)
			rec := httptest.NewRecorder()
			BoundsHandler(svc).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, `id="block-list"`) {
				t.Errorf("patch missing #block-list morph anchor; body:\n%s", body)
			}
			// Current (default) bounds, not the rejected ones.
			for _, want := range []string{`data-day-start="18"`, `data-day-end="34"`} {
				if !strings.Contains(body, want) {
					t.Errorf("patch missing %q; body:\n%s", want, body)
				}
			}
			assertOrder(t, body, "a", "b", "c")
		})
	}
}

// CreateHandler delegates to the core with the modal's slot and label, then
// patches the committed column carrying the new block.
func TestCreateDelegatesToCoreAndPatchesColumn(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}

	req := authedRequest(http.MethodPost, "/blocks", `{"addslot":30,"addlabel":"Deep Work"}`)
	rec := httptest.NewRecorder()
	CreateHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
	}
	if svc.gotOwner != testOwner {
		t.Errorf("core Create called with owner %q, want %q", svc.gotOwner, testOwner)
	}
	if svc.gotLabel != "Deep Work" || svc.gotSlot != 30 {
		t.Errorf("core Create called with {%q,%d}, want {Deep Work,30}", svc.gotLabel, svc.gotSlot)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="block-list"`) {
		t.Errorf("patch missing #block-list morph anchor; body:\n%s", body)
	}
	if !strings.Contains(body, "Deep Work") {
		t.Errorf("patch missing the new block label; body:\n%s", body)
	}
}

// A rejected create (empty label, occupied or out-of-bounds slot) responds 200
// with the current column re-rendered — the optimistic intent yields to truth.
func TestCreateRejectionPatchesCurrentColumn(t *testing.T) {
	for _, domainErr := range []error{block.ErrEmptyLabel, block.ErrOverlap, block.ErrOutOfBounds} {
		t.Run(domainErr.Error(), func(t *testing.T) {
			svc := &fakeService{blocks: threeBlocks(), createErr: domainErr}

			req := authedRequest(http.MethodPost, "/blocks", `{"addslot":18,"addlabel":"X"}`)
			rec := httptest.NewRecorder()
			CreateHandler(svc).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, `id="block-list"`) {
				t.Errorf("patch missing #block-list morph anchor; body:\n%s", body)
			}
			assertOrder(t, body, "a", "b", "c")
		})
	}
}

// Clear delegates to the core (owner-scoped) and patches back the now-empty
// column — the morph anchor survives, the cleared block labels don't.
func TestClearDelegatesToCoreAndPatchesEmptyColumn(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}

	req := authedRequest(http.MethodPost, "/blocks/clear", `{}`)
	rec := httptest.NewRecorder()
	ClearHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d; body:\n%s", rec.Code, rec.Body.String())
	}
	if svc.gotOwner != testOwner {
		t.Errorf("core Clear called with owner %q, want %q", svc.gotOwner, testOwner)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="block-list"`) {
		t.Errorf("patch missing #block-list morph anchor; body:\n%s", body)
	}
	for _, id := range []string{`data-id="a"`, `data-id="b"`, `data-id="c"`} {
		if strings.Contains(body, id) {
			t.Errorf("cleared column still shows block %s; body:\n%s", id, body)
		}
	}
}

// A core failure on clear surfaces as a 500 (no domain rejection path).
func TestClearCoreErrorIs500(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks(), clearErr: errors.New("boom")}

	req := authedRequest(http.MethodPost, "/blocks/clear", `{}`)
	rec := httptest.NewRecorder()
	ClearHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: want 500, got %d; body:\n%s", rec.Code, rec.Body.String())
	}
}

// Keyed Datastar attributes separate plugin and key with a COLON on v1.0.2
// (`data-on:reorder`, `data-signals:order`). The dash forms (`data-on-reorder`)
// are looked up as nonexistent plugin names and silently skipped — no console
// error, the drop just never POSTs. This test exists because that exact
// regression shipped once.
func TestColumnUsesVerifiedDatastarKeyedAttributeSyntax(t *testing.T) {
	var b strings.Builder
	if err := components.BlockColumn(threeBlocks(), testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	for _, attr := range []string{`data-on:layout=`, `data-signals:layout=`} {
		if !strings.Contains(body, attr) {
			t.Errorf("column missing verified keyed attribute %q; body:\n%s", attr, body)
		}
	}
	for _, stale := range []string{`data-on-layout`, `data-signals-layout`, `data-on:reorder`, `data-on:cardresize`} {
		if strings.Contains(body, stale) {
			t.Errorf("column carries dash-form attribute %q — a silent no-op on Datastar v1.0.2; body:\n%s", stale, body)
		}
	}
}

// Each block carries its persisted span as data-span; default blocks render
// data-span="1". drag.js reads slot/span from these to seed each gesture.
func TestColumnRendersPersistedSpan(t *testing.T) {
	cs := []block.Block{
		{ID: "a", Label: "Alpha", Position: 0, Span: 2},
		{ID: "b", Label: "Bravo", Position: 1, Span: 1},
	}
	var b strings.Builder
	if err := components.BlockColumn(cs, testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	for _, want := range []string{
		`data-id="a" data-span="2"`,
		`data-id="b" data-span="1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("column missing %q; body:\n%s", want, body)
		}
	}
}

// Each block carries its immutable type as data-type, which the block's
// data-[type=…] utilities key the per-type fill on; the attribute must survive
// every morph so the color is stable across SSE re-renders.
func TestColumnRendersBlockType(t *testing.T) {
	cs := []block.Block{
		{ID: "a", Label: "Alpha", Position: 0, Span: 1, Type: block.BlockShallow},
		{ID: "b", Label: "Bravo", Position: 1, Span: 1, Type: block.BlockBreak},
	}
	var b strings.Builder
	if err := components.BlockColumn(cs, testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	for _, want := range []string{`data-slot="0" data-type="shallow"`, `data-slot="1" data-type="break"`} {
		if !strings.Contains(body, want) {
			t.Errorf("column missing %q; body:\n%s", want, body)
		}
	}
}

// The column renders every slot of the day as a first-class element carrying
// its clock-slot index, so empty time is real markup (drop targets) and the
// grid extent always matches the owner's bounds.
func TestColumnRendersEverySlotInDay(t *testing.T) {
	var b strings.Builder
	if err := components.BlockColumn(threeBlocks(), testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	// Slots/blocks carry utility classes after the structural hook, so match the
	// `slot `/`block-item ` prefix (excludes slot-add, block-label, the blocks ul)
	// rather than an exact class attribute.
	if got, want := strings.Count(body, `class="slot `), testBounds.End-testBounds.Start; got != want {
		t.Errorf("want %d slot elements, got %d; body:\n%s", want, got, body)
	}
	for _, want := range []string{`data-slot="18"`, `data-slot="33"`} {
		if !strings.Contains(body, want) {
			t.Errorf("column missing %q; body:\n%s", want, body)
		}
	}
	if strings.Contains(body, `data-slot="34"`) {
		t.Errorf("slot 34 is past day end (end-exclusive); body:\n%s", body)
	}
	if lastSlot, firstCard := strings.LastIndex(body, `class="slot `), strings.Index(body, `class="block-item `); firstCard < lastSlot {
		t.Errorf("blocks must render after slots so they paint above; body:\n%s", body)
	}
}

// Each slot carries a time gutter label: hour slots read like "9:00", half-hour
// slots like "9:30" — the paper time-block-notebook axis.
func TestColumnRendersTimeGutter(t *testing.T) {
	var b strings.Builder
	if err := components.BlockColumn(threeBlocks(), testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	for _, want := range []string{">9:00<", ">16:00<"} {
		if !strings.Contains(body, want) {
			t.Errorf("gutter missing hour label %q; body:\n%s", want, body)
		}
	}
	if got, want := strings.Count(body, ":30<"), (testBounds.End-testBounds.Start)/2; got != want {
		t.Errorf("want %d half-hour marks, got %d; body:\n%s", want, got, body)
	}
}

// Blocks render their placement from slot/span: the clock slot as data-slot and
// a grid-row computed against the day's start, so a block at 11:00 paints at
// 11:00 whatever the bounds are and morphs stay idempotent.
func TestColumnPlacesBlocksBySlotAndSpan(t *testing.T) {
	cs := []block.Block{
		{ID: "a", Label: "Alpha", Position: 18, Span: 1},
		{ID: "b", Label: "Bravo", Position: 22, Span: 2},
	}
	var b strings.Builder
	if err := components.BlockColumn(cs, testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	for _, want := range []string{
		`data-id="a" data-span="1" data-slot="18"`,
		`data-id="b" data-span="2" data-slot="22"`,
		`grid-row:1 / span 1`,
		`grid-row:5 / span 2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("column missing %q; body:\n%s", want, body)
		}
	}
}

// The column carries the owner's day bounds as data attributes on #block-list,
// so every morph re-asserts the grid extent the client renders and drags
// against — bounds and blocks can never drift apart across patches.
func TestColumnRendersDayBounds(t *testing.T) {
	var b strings.Builder
	if err := components.BlockColumn(threeBlocks(), testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := b.String()
	for _, want := range []string{`data-day-start="18"`, `data-day-end="34"`} {
		if !strings.Contains(body, want) {
			t.Errorf("column missing %q; body:\n%s", want, body)
		}
	}
}

// GET / renders the column server-side, blocks in the order the core service
// returns them, and wires the page to the live stream (/events) so foreign
// reorders arrive as patches.
func TestPageRendersColumnInServiceOrder(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}

	req := authedRequest(http.MethodGet, "/", "")
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
			t.Errorf("body missing block label %q", label)
		}
	}
	if !strings.Contains(body, "/events") {
		t.Errorf("body missing /events SSE reference; body:\n%s", body)
	}
}

// Every column patch carries the recomputed occupied envelope as a
// patch-signals frame, so the once-rendered bounds modal's disabled options
// track the live layout (a block added/moved/removed in this or another tab).
func TestEventsPatchesEnvelopeSignals(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}
	broker := pubsub.New()

	_, br := openEvents(t, EventsHandler(svc, broker))
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

	// A publish recomputes the envelope from the event's blocks.
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

// A mutation response (here a bounds edit) re-patches the envelope alongside
// the committed column, so the modal's options reflect the new truth at 200.
func TestBoundsResponsePatchesEnvelopeSignals(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}

	req := authedRequest(http.MethodPost, "/blocks/bounds", `{"start":17,"end":22}`)
	rec := httptest.NewRecorder()
	BoundsHandler(svc).ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "datastar-patch-signals") {
		t.Fatalf("bounds response missing patch-signals; body:\n%s", body)
	}
	for _, want := range []string{`"firstOccupiedSlot":18`, `"lastOccupiedEnd":21`} {
		if !strings.Contains(body, want) {
			t.Errorf("bounds response envelope missing %q; body:\n%s", want, body)
		}
	}
}
