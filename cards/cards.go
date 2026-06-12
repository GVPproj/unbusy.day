// Package cards is the transport-agnostic core service: the JSON/SSE and
// Datastar adapters both drive one Service, so reorder logic lives once.
// Every query is owner-scoped (ADR 0003): each User privately owns their Cards.
package cards

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// Span is the card's height in stretch slots (≥1).
	Span int `json:"span"`
}

type ReorderResult struct {
	Cards []Card `json:"cards"`
}

// ResizeResult mirrors ReorderResult: the post-mutation column.
type ResizeResult struct {
	Cards []Card `json:"cards"`
}

// Event is one mutation fanned out over the in-process pub/sub: the owner key
// plus the full ordered card list. The broker routes by Owner so a mutation
// only wakes that User's subscribers (ADR 0003).
type Event struct {
	Owner string `json:"owner"`
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

// ErrInvalidSpan signals a span below the one-slot floor.
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

func (s *Service) List(ctx context.Context, owner string) ([]Card, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, label, position, span FROM card WHERE owner_id = $1 ORDER BY position`, owner)
	if err != nil {
		return nil, err
	}
	return scanCards(rows)
}

// Seed gives a new User their starter cards on first login (ADR 0003) so the
// reorder demo works before any create-card UI exists. No-op if the owner
// already has cards; ids are generated, never hand-picked.
func (s *Service) Seed(ctx context.Context, owner string) error {
	labels := []string{"Alpha", "Bravo", "Charlie"}
	var b strings.Builder
	b.WriteString(`INSERT INTO card (id, label, position, owner_id) SELECT v.id, v.label, v.pos, $1 FROM (VALUES `)
	args := []any{owner}
	for i, label := range labels {
		id, err := newID()
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "($%d::text, $%d::text, $%d::int)", len(args)+1, len(args)+2, len(args)+3)
		args = append(args, id, label, i)
	}
	b.WriteString(`) AS v(id, label, pos) WHERE NOT EXISTS (SELECT 1 FROM card WHERE owner_id = $1)`)
	_, err := s.pool.Exec(ctx, b.String(), args...)
	return err
}

func (s *Service) Reorder(ctx context.Context, owner string, order []string) (*ReorderResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// FOR UPDATE serialises concurrent reorders on this owner's rows: the
	// permutation check below races no one, and the bulk UPDATE can't
	// interleave with a sibling tx.
	idRows, err := tx.Query(ctx, `SELECT id FROM card WHERE owner_id = $1 FOR UPDATE`, owner)
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
	if len(order) == 0 {
		return &ReorderResult{Cards: nil}, nil
	}

	// Single bulk UPDATE … FROM (VALUES …), owner-scoped. The DEFERRABLE
	// unique on (owner_id, position) lets intermediate row states overlap
	// until commit.
	var b strings.Builder
	b.WriteString(`UPDATE card AS c SET position = v.pos FROM (VALUES `)
	args := make([]any, 0, len(order)*2+1)
	for i, id := range order {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "($%d::text, $%d::int)", i*2+1, i*2+2)
		args = append(args, id, i)
	}
	fmt.Fprintf(&b, `) AS v(id, pos) WHERE c.id = v.id AND c.owner_id = $%d`, len(args)+1)
	args = append(args, owner)
	if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
		return nil, err
	}

	cs, err := listTx(ctx, tx, owner)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Fan out post-commit so subscribers never observe an uncommitted order.
	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Cards: cs})
	}
	return &ReorderResult{Cards: cs}, nil
}

// Resize persists a card's span and returns the full post-mutation column.
func (s *Service) Resize(ctx context.Context, owner, id string, span int) (*ResizeResult, error) {
	// Typed error to snap back on; the DB CHECK is the backstop.
	if span < 1 {
		return nil, ErrInvalidSpan
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE card SET span = $3 WHERE id = $2 AND owner_id = $1`, owner, id, span); err != nil {
		return nil, err
	}

	cs, err := listTx(ctx, tx, owner)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Fan out post-commit so subscribers never observe an uncommitted height.
	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Cards: cs})
	}
	return &ResizeResult{Cards: cs}, nil
}

func listTx(ctx context.Context, tx pgx.Tx, owner string) ([]Card, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, label, position, span FROM card WHERE owner_id = $1 ORDER BY position`, owner)
	if err != nil {
		return nil, err
	}
	return scanCards(rows)
}

// newID is a generated unique card id — hand-picked ids can't repeat across Users.
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
