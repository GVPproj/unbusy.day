// Package cards is the transport-agnostic core service: the JSON/SSE and
// Datastar adapters both drive one Service, so reorder logic lives once.
package cards

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Card struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Position int    `json:"position"`
	// Span is the card's height in stretch slots (≥1). Persisted from the
	// grip-resize gesture; the server renders it so heights survive reload.
	Span int `json:"span"`
}

type ReorderResult struct {
	Cards []Card `json:"cards"`
	Txid  string `json:"txid"`
}

// ResizeResult mirrors ReorderResult: the full post-mutation column plus the
// txid, so the resize adapter renders one frame and the bus fans the same shape
// to every other tab.
type ResizeResult struct {
	Cards []Card `json:"cards"`
	Txid  string `json:"txid"`
}

// Event is one mutation fanned out over the in-process pub/sub: a txid plus
// the full ordered card list. Carries structured Cards rather than serialized
// bytes so each adapter renders it its own way off one published event.
type Event struct {
	Txid  string `json:"txid"`
	Cards []Card `json:"cards"`
}

// Publisher is the seam between the core mutation and transport fan-out. The
// Service owns the publish call but not the bus, so the Broker can live in
// another package without an import cycle. A nil Publisher skips fan-out.
type Publisher interface {
	Publish(Event)
}

// ErrNotPermutation signals that the supplied order is not a permutation of
// the current card ids (wrong length, unknown id, or duplicate). Adapters
// surface this as 4xx so the client rolls back its optimistic order.
var ErrNotPermutation = errors.New("order is not a permutation of current cards")

// ErrInvalidSpan signals a span below the one-slot floor. Adapters snap the
// card back to the authoritative height, same shape as a rejected reorder.
var ErrInvalidSpan = errors.New("span must be at least 1")

type Service struct {
	pool *pgxpool.Pool
	pub  Publisher
}

// NewService wires the core service over a Postgres pool and a Publisher. pub
// may be nil (e.g. unit tests with no bus): Reorder then skips fan-out.
func NewService(pool *pgxpool.Pool, pub Publisher) *Service {
	return &Service{pool: pool, pub: pub}
}

func (s *Service) List(ctx context.Context) ([]Card, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, label, position, span FROM card ORDER BY position`)
	if err != nil {
		return nil, err
	}
	return scanCards(rows)
}

func (s *Service) Reorder(ctx context.Context, order []string) (*ReorderResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// FOR UPDATE serialises concurrent reorders: the permutation check below
	// races no one, and the bulk UPDATE can't interleave with a sibling tx.
	idRows, err := tx.Query(ctx, `SELECT id FROM card FOR UPDATE`)
	if err != nil {
		return nil, err
	}
	current := make(map[string]struct{})
	for idRows.Next() {
		var id string
		if err := idRows.Scan(&id); err != nil {
			idRows.Close()
			return nil, err
		}
		current[id] = struct{}{}
	}
	idRows.Close()
	if err := idRows.Err(); err != nil {
		return nil, err
	}

	if err := validatePermutation(order, current); err != nil {
		return nil, err
	}

	// Single bulk UPDATE … FROM (VALUES …). The DEFERRABLE unique on
	// position lets intermediate row states overlap until commit.
	var b strings.Builder
	b.WriteString(`UPDATE card AS c SET position = v.pos FROM (VALUES `)
	args := make([]any, 0, len(order)*2)
	for i, id := range order {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "($%d::text, $%d::int)", i*2+1, i*2+2)
		args = append(args, id, i)
	}
	b.WriteString(`) AS v(id, pos) WHERE c.id = v.id`)
	if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
		return nil, err
	}

	// pg_current_xact_id() returns xid8 (64-bit). Never cast to ::xid — that
	// truncates to 32 bits and breaks handshake matching as values wrap.
	// Keep it a decimal string end-to-end.
	var txid string
	if err := tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text`).Scan(&txid); err != nil {
		return nil, err
	}

	cardRows, err := tx.Query(ctx, `SELECT id, label, position, span FROM card ORDER BY position`)
	if err != nil {
		return nil, err
	}
	cs, err := scanCards(cardRows)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Fan out post-commit so subscribers never observe an uncommitted order.
	// FOR UPDATE above serialises concurrent reorders, so publish order is
	// monotonic in txid.
	if s.pub != nil {
		s.pub.Publish(Event{Txid: txid, Cards: cs})
	}
	return &ReorderResult{Cards: cs, Txid: txid}, nil
}

// Resize persists a card's span and returns the full post-mutation column.
func (s *Service) Resize(ctx context.Context, id string, span int) (*ResizeResult, error) {
	// Guard the one-slot floor here (the DB CHECK is the backstop) so callers
	// get a typed error to snap back on, not an opaque constraint violation.
	if span < 1 {
		return nil, ErrInvalidSpan
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE card SET span = $2 WHERE id = $1`, id, span); err != nil {
		return nil, err
	}

	// See Reorder: keep the xid8 a decimal string end-to-end — ::xid truncates.
	var txid string
	if err := tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text`).Scan(&txid); err != nil {
		return nil, err
	}

	cardRows, err := tx.Query(ctx, `SELECT id, label, position, span FROM card ORDER BY position`)
	if err != nil {
		return nil, err
	}
	cs, err := scanCards(cardRows)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Fan out post-commit so subscribers never observe an uncommitted height.
	if s.pub != nil {
		s.pub.Publish(Event{Txid: txid, Cards: cs})
	}
	return &ResizeResult{Cards: cs, Txid: txid}, nil
}

func validatePermutation(order []string, current map[string]struct{}) error {
	if len(order) != len(current) {
		return ErrNotPermutation
	}
	seen := make(map[string]struct{}, len(order))
	for _, id := range order {
		if _, ok := current[id]; !ok {
			return ErrNotPermutation
		}
		if _, dup := seen[id]; dup {
			return ErrNotPermutation
		}
		seen[id] = struct{}{}
	}
	return nil
}

func scanCards(rows pgx.Rows) ([]Card, error) {
	defer rows.Close()
	var out []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.Label, &c.Position, &c.Span); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
