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

// PRD M1a Done-when: "a 100-random-reorder fuzz never trips the unique
// constraint" — exercises F1's bulk UPDATE against F10's DEFERRABLE unique.
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
	var prevTxid string
	for i := range 100 {
		rng.Shuffle(len(ids), func(a, b int) { ids[a], ids[b] = ids[b], ids[a] })
		order := append([]string(nil), ids...)

		res, err := svc.Reorder(ctx, order)
		if err != nil {
			t.Fatalf("iter %d order=%v: %v", i, order, err)
		}
		if res.Txid == "" || res.Txid == prevTxid {
			t.Fatalf("iter %d: bad txid %q (prev %q)", i, res.Txid, prevTxid)
		}
		prevTxid = res.Txid
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
