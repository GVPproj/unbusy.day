package block_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — start `task up` and `task migrate`")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newOwner creates a throwaway user and seeds their starter block. Cleanup
// deletes the user; the blocks go with it (ON DELETE CASCADE).
func newOwner(t *testing.T, pool *pgxpool.Pool, svc *block.Service) string {
	t.Helper()
	ctx := context.Background()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	id := "test-" + hex.EncodeToString(b)
	if _, err := pool.Exec(ctx, `INSERT INTO "user" (id, email) VALUES ($1, $2)`, id, id+"@example.test"); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, id)
	})
	if err := svc.Seed(ctx, id); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return id
}

// Seed is first-login-only: a second call must not duplicate the starter block.
func TestSeed_Idempotent(t *testing.T) {
	pool := newPool(t)
	svc := block.NewService(pool, nil)
	ctx := context.Background()
	owner := newOwner(t, pool, svc)

	if err := svc.Seed(ctx, owner); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	cs, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("want 3 starter blocks after re-seed, got %d", len(cs))
	}
}

// Starter blocks land in the first slots after the default day start (9:00),
// span 1 each, so a new user's plan is valid against their bounds.
func TestSeed_PlacesStarterBlocksAtDayStart(t *testing.T) {
	pool := newPool(t)
	svc := block.NewService(pool, nil)
	ctx := context.Background()
	owner := newOwner(t, pool, svc)

	cs, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("want 3 starter blocks, got %d", len(cs))
	}
	for i, c := range cs {
		if want := block.DefaultDayStart + i; c.Position != want {
			t.Fatalf("starter block %d at slot %d, want %d", i, c.Position, want)
		}
		if c.Span != 1 {
			t.Fatalf("starter block %d span %d, want 1", i, c.Span)
		}
	}
}

// Owner scoping (ADR 0003): one user's mutations never touch or read
// another's blocks, and both owners can hold position 0.
func TestOwnersAreIsolated(t *testing.T) {
	pool := newPool(t)
	svc := block.NewService(pool, nil)
	ctx := context.Background()
	a := newOwner(t, pool, svc)
	b := newOwner(t, pool, svc)

	acs, err := svc.List(ctx, a)
	if err != nil {
		t.Fatalf("list a: %v", err)
	}
	bcs, err := svc.List(ctx, b)
	if err != nil {
		t.Fatalf("list b: %v", err)
	}
	for _, ac := range acs {
		for _, bc := range bcs {
			if ac.ID == bc.ID {
				t.Fatalf("block id %s appears under both owners", ac.ID)
			}
		}
	}

	// A layout submitted under a's scope with b's ids is not a's block set, so
	// nothing of b's can be moved or resized through a.
	layout := make([]block.Placement, len(bcs))
	for i, c := range bcs {
		layout[i] = block.Placement{ID: c.ID, Slot: c.Position, Span: c.Span + 1}
	}
	if _, err := svc.SetLayout(ctx, a, layout); !errors.Is(err, block.ErrNotSameBlocks) {
		t.Fatalf("cross-owner layout: want ErrNotSameBlocks, got %v", err)
	}
	after, err := svc.List(ctx, b)
	if err != nil {
		t.Fatalf("list b after: %v", err)
	}
	if got := spanOf(after, bcs[0].ID); got != 1 {
		t.Fatalf("cross-owner layout leaked: span = %d, want 1", got)
	}
}

// A valid full layout (a move into a gap plus a grow) commits and is what
// List returns afterwards, ordered by slot.
func TestSetLayout_CommitsAndListReflects(t *testing.T) {
	pool := newPool(t)
	svc := block.NewService(pool, nil)
	ctx := context.Background()
	owner := newOwner(t, pool, svc)

	initial, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(initial) != 3 {
		t.Fatalf("want 3 seed blocks, got %d", len(initial))
	}
	a, b, c := initial[0], initial[1], initial[2]

	layout := []block.Placement{
		{ID: a.ID, Slot: 25, Span: 2}, // moved into a gap and grown
		{ID: b.ID, Slot: 19, Span: 1},
		{ID: c.ID, Slot: 20, Span: 1},
	}
	res, err := svc.SetLayout(ctx, owner, layout)
	if err != nil {
		t.Fatalf("setlayout: %v", err)
	}
	want := []struct {
		id         string
		slot, span int
	}{{b.ID, 19, 1}, {c.ID, 20, 1}, {a.ID, 25, 2}}
	check := func(cs []block.Block, label string) {
		if len(cs) != len(want) {
			t.Fatalf("%s: got %d blocks, want %d", label, len(cs), len(want))
		}
		for i, w := range want {
			if cs[i].ID != w.id || cs[i].Position != w.slot || cs[i].Span != w.span {
				t.Fatalf("%s[%d]: got {%s,%d,%d}, want {%s,%d,%d}",
					label, i, cs[i].ID, cs[i].Position, cs[i].Span, w.id, w.slot, w.span)
			}
		}
	}
	check(res.Blocks, "result")

	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	check(after, "list")
}

// A rejected layout surfaces its typed domain error, persists nothing, and
// fans nothing out.
func TestSetLayout_RejectionLeavesStateUntouched(t *testing.T) {
	pool := newPool(t)
	pub := &capturePub{}
	svc := block.NewService(pool, pub)
	ctx := context.Background()
	owner := newOwner(t, pool, svc)

	initial, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	a, b, c := initial[0], initial[1], initial[2]

	cases := map[string]struct {
		layout []block.Placement
		want   error
	}{
		"overlap": {
			[]block.Placement{
				{ID: a.ID, Slot: 20, Span: 2},
				{ID: b.ID, Slot: 21, Span: 1},
				{ID: c.ID, Slot: 30, Span: 1},
			}, block.ErrOverlap},
		"out of bounds": {
			[]block.Placement{
				{ID: a.ID, Slot: 33, Span: 2},
				{ID: b.ID, Slot: 19, Span: 1},
				{ID: c.ID, Slot: 20, Span: 1},
			}, block.ErrOutOfBounds},
		"not same blocks": {
			[]block.Placement{
				{ID: a.ID, Slot: 20, Span: 1},
			}, block.ErrNotSameBlocks},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.SetLayout(ctx, owner, tc.layout); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}

	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	for i, c := range after {
		if c != initial[i] {
			t.Fatalf("rejected layouts must persist nothing: block %d = %+v, want %+v", i, c, initial[i])
		}
	}
	if len(pub.events) != 0 {
		t.Fatalf("rejected layouts must not fan out: got %d events", len(pub.events))
	}
}

// A committed layout fans out one event carrying the owner key and new layout.
func TestSetLayout_PublishesEvent(t *testing.T) {
	pool := newPool(t)
	pub := &capturePub{}
	svc := block.NewService(pool, pub)
	ctx := context.Background()
	owner := newOwner(t, pool, svc)

	initial, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	a, b, c := initial[0], initial[1], initial[2]

	if _, err := svc.SetLayout(ctx, owner, []block.Placement{
		{ID: a.ID, Slot: 30, Span: 1},
		{ID: b.ID, Slot: 19, Span: 1},
		{ID: c.ID, Slot: 20, Span: 1},
	}); err != nil {
		t.Fatalf("setlayout: %v", err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("want 1 published event, got %d", len(pub.events))
	}
	e := pub.events[0]
	if e.Owner != owner {
		t.Fatalf("published owner = %q, want %q", e.Owner, owner)
	}
	if last := e.Blocks[len(e.Blocks)-1]; last.ID != a.ID || last.Position != 30 {
		t.Fatalf("published layout tail = {%s,%d}, want {%s,30}", last.ID, last.Position, a.ID)
	}
	// Subscribers render the grid from the event alone, so it must carry bounds.
	if e.Bounds != (block.Bounds{Start: block.DefaultDayStart, End: block.DefaultDayEnd}) {
		t.Fatalf("published bounds = %+v, want default day", e.Bounds)
	}
}

// Concurrent layout mutations serialize on FOR UPDATE: every submission is a
// valid layout, so all succeed and the final state is exactly one of them.
func TestSetLayout_ConcurrentMutationsSerialize(t *testing.T) {
	pool := newPool(t)
	svc := block.NewService(pool, nil)
	ctx := context.Background()
	owner := newOwner(t, pool, svc)

	initial, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	a, b, c := initial[0].ID, initial[1].ID, initial[2].ID

	layouts := [][]block.Placement{
		{{ID: a, Slot: 18, Span: 2}, {ID: b, Slot: 20, Span: 1}, {ID: c, Slot: 30, Span: 1}},
		{{ID: a, Slot: 25, Span: 1}, {ID: b, Slot: 18, Span: 3}, {ID: c, Slot: 21, Span: 1}},
		{{ID: a, Slot: 33, Span: 1}, {ID: b, Slot: 19, Span: 1}, {ID: c, Slot: 24, Span: 2}},
		{{ID: a, Slot: 18, Span: 1}, {ID: b, Slot: 19, Span: 1}, {ID: c, Slot: 20, Span: 1}},
	}
	var wg sync.WaitGroup
	errs := make([]error, len(layouts))
	for i, l := range layouts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.SetLayout(ctx, owner, l)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent setlayout %d: %v", i, err)
		}
	}

	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	matchesOne := false
	for _, l := range layouts {
		ok := true
		for _, p := range l {
			i := slices.IndexFunc(after, func(c block.Block) bool { return c.ID == p.ID })
			if i < 0 || after[i].Position != p.Slot || after[i].Span != p.Span {
				ok = false
				break
			}
		}
		matchesOne = matchesOne || ok
	}
	if !matchesOne {
		t.Fatalf("final state %+v matches none of the submitted layouts", after)
	}
}

// Bounds may shrink into empty slots; the change persists and fans out an
// event carrying the new bounds so live tabs re-render the grid.
func TestSetBounds_ShrinkIntoEmptySlots(t *testing.T) {
	pool := newPool(t)
	pub := &capturePub{}
	svc := block.NewService(pool, pub)
	ctx := context.Background()
	owner := newOwner(t, pool, svc) // starter blocks at 18,19,20

	if err := svc.SetBounds(ctx, owner, 17, 21); err != nil {
		t.Fatalf("setbounds: %v", err)
	}
	got, err := svc.Bounds(ctx, owner)
	if err != nil {
		t.Fatalf("bounds: %v", err)
	}
	if got != (block.Bounds{Start: 17, End: 21}) {
		t.Fatalf("bounds = %+v, want {17 21}", got)
	}
	if len(pub.events) != 1 {
		t.Fatalf("want 1 published event, got %d", len(pub.events))
	}
	if pub.events[0].Bounds != got {
		t.Fatalf("published bounds = %+v, want %+v", pub.events[0].Bounds, got)
	}
}

// A shrink that would strand a block outside the day is rejected whole, on
// either side, and persists nothing.
func TestSetBounds_RejectsShrinkIntoOccupied(t *testing.T) {
	pool := newPool(t)
	svc := block.NewService(pool, nil)
	ctx := context.Background()
	owner := newOwner(t, pool, svc) // starter blocks at 18,19,20

	cases := map[string][2]int{
		"start side": {19, 34},
		"end side":   {18, 20},
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if err := svc.SetBounds(ctx, owner, b[0], b[1]); !errors.Is(err, block.ErrBoundsOccupied) {
				t.Fatalf("want ErrBoundsOccupied, got %v", err)
			}
		})
	}
	got, err := svc.Bounds(ctx, owner)
	if err != nil {
		t.Fatalf("bounds: %v", err)
	}
	if got != (block.Bounds{Start: block.DefaultDayStart, End: block.DefaultDayEnd}) {
		t.Fatalf("rejected bounds must persist nothing: got %+v", got)
	}
}

// Hard limits: start ≥ 5:00, end ≤ 18:00, end > start.
func TestSetBounds_RejectsOutsideHardLimits(t *testing.T) {
	pool := newPool(t)
	svc := block.NewService(pool, nil)
	ctx := context.Background()
	owner := newOwner(t, pool, svc)

	cases := map[string][2]int{
		"before 5:00":    {9, 34},
		"after 18:00":    {18, 37},
		"end not beyond": {18, 18},
		"inverted":       {20, 18},
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if err := svc.SetBounds(ctx, owner, b[0], b[1]); !errors.Is(err, block.ErrInvalidBounds) {
				t.Fatalf("want ErrInvalidBounds, got %v", err)
			}
		})
	}
}

// capturePub records every published Event so a test can assert fan-out.
type capturePub struct{ events []block.Event }

func (c *capturePub) Publish(e block.Event) { c.events = append(c.events, e) }

func spanOf(cs []block.Block, id string) int {
	for _, c := range cs {
		if c.ID == id {
			return c.Span
		}
	}
	return -1
}
