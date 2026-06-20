package block_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	_ "modernc.org/sqlite"
)

// newDB returns a handle to an ephemeral SQLite database with the schema
// migrated in. No external container needed; the file dies with the temp dir.
func newDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "block_test.db")
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	if err := migrate.Run(context.Background(), dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newOwner creates a throwaway user and seeds their starter block. Cleanup
// deletes the user; the blocks go with it (ON DELETE CASCADE).
func newOwner(t *testing.T, db *sql.DB, svc *block.Service) string {
	t.Helper()
	ctx := context.Background()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	id := "test-" + hex.EncodeToString(b)
	if _, err := db.ExecContext(ctx, `INSERT INTO "user" (id, email) VALUES (?, ?)`, id, id+"@example.test"); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM "user" WHERE id = ?`, id)
	})
	if err := svc.Seed(ctx, id); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return id
}

// Seed is first-login-only: a second call must not duplicate the starter block.
func TestSeed_Idempotent(t *testing.T) {
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

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
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

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
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	a := newOwner(t, db, svc)
	b := newOwner(t, db, svc)

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
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

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
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

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
			}, block.ErrOverlap,
		},
		"out of bounds": {
			[]block.Placement{
				{ID: a.ID, Slot: 33, Span: 2},
				{ID: b.ID, Slot: 19, Span: 1},
				{ID: c.ID, Slot: 20, Span: 1},
			}, block.ErrOutOfBounds,
		},
		"not same blocks": {
			[]block.Placement{
				{ID: a.ID, Slot: 20, Span: 1},
			}, block.ErrNotSameBlocks,
		},
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

// Overlap regression: the Postgres gist EXCLUDE backstop is gone under SQLite,
// so ValidateLayout is the sole overlap guard. An overlapping layout must reject
// whole and persist nothing through the full service path (not just the unit
// validator), since nothing downstream would catch it.
func TestSetLayout_OverlapRejectedWithoutDBBackstop(t *testing.T) {
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

	initial, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	a, b, c := initial[0], initial[1], initial[2]

	// a spans slots 20–21; b sits on 21 — a direct overlap ValidateLayout catches.
	overlap := []block.Placement{
		{ID: a.ID, Slot: 20, Span: 2},
		{ID: b.ID, Slot: 21, Span: 1},
		{ID: c.ID, Slot: 30, Span: 1},
	}
	if _, err := svc.SetLayout(ctx, owner, overlap); !errors.Is(err, block.ErrOverlap) {
		t.Fatalf("want ErrOverlap, got %v", err)
	}
	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	for i := range after {
		if after[i] != initial[i] {
			t.Fatalf("overlap rejection must persist nothing: block %d = %+v, want %+v", i, after[i], initial[i])
		}
	}
}

// A committed layout fans out one event carrying the owner key and new layout.
func TestSetLayout_PublishesEvent(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

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

// Concurrent layout mutations serialize on SQLite's write lock (_txlock=immediate
// + busy_timeout): every submission is a valid layout, so all succeed and the
// final state is exactly one of them — no torn or interleaved layout survives.
func TestSetLayout_ConcurrentMutationsLastWriterWins(t *testing.T) {
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

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
		wg.Go(func() {
			_, errs[i] = svc.SetLayout(ctx, owner, l)
		})
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
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc) // starter blocks at DefaultDayStart, +1, +2

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
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc) // starter blocks at DefaultDayStart, +1, +2

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
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

	cases := map[string][2]int{
		"before 4:00":    {7, 34},
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

// Create inserts a span-1 block at a free slot, returns it in the committed
// column ordered by slot, and fans out one event carrying the new column.
func TestCreate_InsertsBlockAndPublishes(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc) // starter blocks at DefaultDayStart, +1, +2

	// Slot 30 is free (after the three starter blocks).
	res, err := svc.Create(ctx, owner, "  Deep Work  ", 30, block.BlockDeep)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(res.Blocks) != 4 {
		t.Fatalf("want 4 blocks after create, got %d", len(res.Blocks))
	}
	last := res.Blocks[len(res.Blocks)-1]
	if last.Label != "Deep Work" { // label is trimmed
		t.Fatalf("created label = %q, want %q", last.Label, "Deep Work")
	}
	if last.Position != 30 || last.Span != 1 {
		t.Fatalf("created block = {%d,%d}, want {30,1}", last.Position, last.Span)
	}

	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 4 {
		t.Fatalf("want 4 persisted blocks, got %d", len(after))
	}
	if len(pub.events) != 1 {
		t.Fatalf("want 1 published event, got %d", len(pub.events))
	}
	if e := pub.events[0]; e.Owner != owner || len(e.Blocks) != 4 {
		t.Fatalf("published event = {owner %q, %d blocks}, want {%q, 4}", e.Owner, len(e.Blocks), owner)
	}
}

// A valid type passed to Create round-trips: it comes back on the created
// block, in the persisted column, and in the published event.
func TestCreate_TypeRoundTrips(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

	res, err := svc.Create(ctx, owner, "Shallow thing", 30, block.BlockShallow)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if last := res.Blocks[len(res.Blocks)-1]; last.Type != block.BlockShallow {
		t.Fatalf("created type = %q, want %q", last.Type, block.BlockShallow)
	}

	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if last := after[len(after)-1]; last.Type != block.BlockShallow {
		t.Fatalf("persisted type = %q, want %q", last.Type, block.BlockShallow)
	}
	if last := pub.events[0].Blocks[len(pub.events[0].Blocks)-1]; last.Type != block.BlockShallow {
		t.Fatalf("published type = %q, want %q", last.Type, block.BlockShallow)
	}
}

// A non-empty unknown type rejects with ErrInvalidBlockType, persisting nothing
// and fanning out nothing.
func TestCreate_InvalidTypeRejected(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

	if _, err := svc.Create(ctx, owner, "X", 30, block.BlockType("focus")); !errors.Is(err, block.ErrInvalidBlockType) {
		t.Fatalf("want ErrInvalidBlockType, got %v", err)
	}
	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("rejected create must persist nothing: got %d blocks", len(after))
	}
	if len(pub.events) != 0 {
		t.Fatalf("rejected create must not fan out: got %d events", len(pub.events))
	}
}

// A blank type defaults to shallow, both on create and on the seeded starters
// (which rely on the DB column default).
func TestCreate_BlankTypeDefaultsToShallow(t *testing.T) {
	db := newDB(t)
	svc := block.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

	for _, c := range mustList(t, svc, ctx, owner) {
		if c.Type != block.BlockShallow {
			t.Fatalf("seeded starter type = %q, want %q", c.Type, block.BlockShallow)
		}
	}

	res, err := svc.Create(ctx, owner, "Untyped", 30, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if last := res.Blocks[len(res.Blocks)-1]; last.Type != block.BlockShallow {
		t.Fatalf("blank-type create = %q, want %q", last.Type, block.BlockShallow)
	}
}

func mustList(t *testing.T, svc *block.Service, ctx context.Context, owner string) []block.Block {
	t.Helper()
	cs, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return cs
}

// Create rejects an empty (or whitespace-only) label, an out-of-bounds slot,
// and a slot already covered by a block — each persists nothing and fans out
// nothing.
func TestCreate_Rejections(t *testing.T) {
	cases := map[string]struct {
		label string
		slot  int
		want  error
	}{
		"empty label":   {"   ", 30, block.ErrEmptyLabel},
		"out of bounds": {"X", block.DefaultDayEnd, block.ErrOutOfBounds},
		"slot occupied": {"X", block.DefaultDayStart, block.ErrOverlap},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			db := newDB(t)
			pub := &capturePub{}
			svc := block.NewService(db, pub)
			ctx := context.Background()
			owner := newOwner(t, db, svc)

			if _, err := svc.Create(ctx, owner, tc.label, tc.slot, block.BlockDeep); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			after, err := svc.List(ctx, owner)
			if err != nil {
				t.Fatalf("list after: %v", err)
			}
			if len(after) != 3 {
				t.Fatalf("rejected create must persist nothing: got %d blocks", len(after))
			}
			if len(pub.events) != 0 {
				t.Fatalf("rejected create must not fan out: got %d events", len(pub.events))
			}
		})
	}
}

func TestRename_UpdatesLabelAndPublishes(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

	before, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	id := before[0].ID

	res, err := svc.Rename(ctx, owner, id, "  Renamed  ")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := labelOf(res.Blocks, id); got != "Renamed" { // label is trimmed
		t.Fatalf("renamed label = %q, want %q", got, "Renamed")
	}
	after, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if got := labelOf(after, id); got != "Renamed" {
		t.Fatalf("persisted label = %q, want %q", got, "Renamed")
	}
	if len(pub.events) != 1 {
		t.Fatalf("want 1 published event, got %d", len(pub.events))
	}
}

// Rename rejects a blank label and an id this owner doesn't own — each persists
// nothing and fans out nothing.
func TestRename_Rejections(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{}
	svc := block.NewService(db, pub)
	ctx := context.Background()
	owner := newOwner(t, db, svc)

	before, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	id := before[0].ID

	if _, err := svc.Rename(ctx, owner, id, "   "); !errors.Is(err, block.ErrEmptyLabel) {
		t.Fatalf("blank label: want ErrEmptyLabel, got %v", err)
	}
	if _, err := svc.Rename(ctx, owner, "no-such-id", "X"); !errors.Is(err, block.ErrBlockNotFound) {
		t.Fatalf("unknown id: want ErrBlockNotFound, got %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatalf("rejected rename must not fan out: got %d events", len(pub.events))
	}
}

// Valid accepts exactly the three canonical types; anything else (including
// blank or wrong-case) is invalid.
func TestBlockType_Valid(t *testing.T) {
	for _, bt := range []block.BlockType{block.BlockDeep, block.BlockShallow, block.BlockBreak} {
		if !bt.Valid() {
			t.Errorf("%q should be valid", bt)
		}
	}
	for _, bt := range []block.BlockType{"", "  ", "DEEP", "focus", "deepwork"} {
		if bt.Valid() {
			t.Errorf("%q should be invalid", bt)
		}
	}
}

// capturePub records every published Event so a test can assert fan-out.
type capturePub struct{ events []block.Event }

func (c *capturePub) Publish(e block.Event) { c.events = append(c.events, e) }

func labelOf(cs []block.Block, id string) string {
	for _, c := range cs {
		if c.ID == id {
			return c.Label
		}
	}
	return ""
}

func spanOf(cs []block.Block, id string) int {
	for _, c := range cs {
		if c.ID == id {
			return c.Span
		}
	}
	return -1
}
