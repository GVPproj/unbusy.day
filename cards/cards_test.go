package cards_test

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"testing"

	"github.com/grahamvanpelt/unbusy.day/cards"
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

// 100 random reorders must never trip the unique-on-position constraint —
// exercises the bulk UPDATE against the DEFERRABLE unique.
func TestReorder_Fuzz(t *testing.T) {
	pool := newPool(t)
	svc := cards.NewService(pool, nil)
	ctx := context.Background()

	initial, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(initial) == 0 {
		t.Fatalf("seed missing — run `task migrate`")
	}
	ids := make([]string, len(initial))
	for i, c := range initial {
		ids[i] = c.ID
	}

	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(ids), func(a, b int) { ids[a], ids[b] = ids[b], ids[a] })
		order := append([]string(nil), ids...)

		res, err := svc.Reorder(ctx, order)
		if err != nil {
			t.Fatalf("iter %d order=%v: %v", i, order, err)
		}
		if len(res.Cards) != len(order) {
			t.Fatalf("iter %d: got %d cards, want %d", i, len(res.Cards), len(order))
		}
		for j, c := range res.Cards {
			if c.ID != order[j] || c.Position != j {
				t.Fatalf("iter %d pos %d: got {%s,%d}, want {%s,%d}",
					i, j, c.ID, c.Position, order[j], j)
			}
		}
	}
}

func TestReorder_RejectsNonPermutation(t *testing.T) {
	pool := newPool(t)
	svc := cards.NewService(pool, nil)
	ctx := context.Background()

	initial, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(initial) < 2 {
		t.Fatalf("need ≥2 seed cards")
	}
	a, b := initial[0].ID, initial[1].ID

	cases := map[string][]string{
		"too short":  {a},
		"too long":   append(append([]string{}, idsOf(initial)...), "extra"),
		"unknown id": replaceAt(idsOf(initial), 0, "zzz-not-real"),
		"duplicate":  fillWith(a, len(initial)),
		"empty":      {},
	}
	for name, order := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Reorder(ctx, order)
			if !errors.Is(err, cards.ErrNotPermutation) {
				t.Fatalf("want ErrNotPermutation, got %v", err)
			}
		})
	}
	_ = b
}

// After a resize, List reflects the new span; other cards stay at default 1.
func TestResize_PersistsSpan(t *testing.T) {
	pool := newPool(t)
	svc := cards.NewService(pool, nil)
	ctx := context.Background()

	initial, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(initial) < 2 {
		t.Fatalf("need ≥2 seed cards")
	}
	id := initial[0].ID
	other := initial[1].ID
	t.Cleanup(func() { _, _ = svc.Resize(ctx, id, 1) }) // restore the shared seed

	res, err := svc.Resize(ctx, id, 3)
	if err != nil {
		t.Fatalf("resize: %v", err)
	}
	if got := spanOf(res.Cards, id); got != 3 {
		t.Fatalf("resize result span for %s = %d, want 3", id, got)
	}
	if got := spanOf(res.Cards, other); got != 1 {
		t.Fatalf("untouched card %s span = %d, want default 1", other, got)
	}

	after, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list after resize: %v", err)
	}
	if got := spanOf(after, id); got != 3 {
		t.Fatalf("persisted span for %s = %d, want 3", id, got)
	}
}

// A span below one slot is rejected with ErrInvalidSpan and persists nothing.
func TestResize_RejectsSpanBelowOne(t *testing.T) {
	pool := newPool(t)
	svc := cards.NewService(pool, nil)
	ctx := context.Background()

	initial, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(initial) == 0 {
		t.Fatalf("seed missing — run `task migrate`")
	}
	id := initial[0].ID

	for _, span := range []int{0, -1} {
		if _, err := svc.Resize(ctx, id, span); !errors.Is(err, cards.ErrInvalidSpan) {
			t.Fatalf("Resize(%d): want ErrInvalidSpan, got %v", span, err)
		}
	}

	after, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list after reject: %v", err)
	}
	if got := spanOf(after, id); got != 1 {
		t.Fatalf("rejected resize must not persist: span for %s = %d, want 1", id, got)
	}
}

// capturePub records every published Event so a test can assert fan-out.
type capturePub struct{ events []cards.Event }

func (c *capturePub) Publish(e cards.Event) { c.events = append(c.events, e) }

// A committed resize fans out on the bus carrying the new span.
func TestResize_PublishesEvent(t *testing.T) {
	pool := newPool(t)
	pub := &capturePub{}
	svc := cards.NewService(pool, pub)
	ctx := context.Background()

	initial, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(initial) == 0 {
		t.Fatalf("seed missing — run `task migrate`")
	}
	id := initial[0].ID
	t.Cleanup(func() { _, _ = svc.Resize(ctx, id, 1) })

	if _, err := svc.Resize(ctx, id, 2); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("want 1 published event, got %d", len(pub.events))
	}
	if got := spanOf(pub.events[0].Cards, id); got != 2 {
		t.Fatalf("published span for %s = %d, want 2", id, got)
	}
}

func spanOf(cs []cards.Card, id string) int {
	for _, c := range cs {
		if c.ID == id {
			return c.Span
		}
	}
	return -1
}

func idsOf(cs []cards.Card) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func replaceAt(s []string, i int, v string) []string {
	out := append([]string(nil), s...)
	out[i] = v
	return out
}

func fillWith(v string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = v
	}
	return out
}
