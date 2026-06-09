// Package cards is the transport-agnostic core service (PRD §5).
// HTTP/SSE adapters (FE1) and the future Datastar adapter (FE2) sit over
// the same Service so business logic exists exactly once (PRD §2).
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
}

type ReorderResult struct {
	Cards []Card `json:"cards"`
	Txid  string `json:"txid"`
}

// ErrNotPermutation signals that the supplied order is not a permutation of
// the current card ids (wrong length, unknown id, or duplicate). Adapters
// surface this as 4xx so TanStack DB rolls back (PRD F5).
var ErrNotPermutation = errors.New("order is not a permutation of current cards")

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) List(ctx context.Context) ([]Card, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, label, position FROM card ORDER BY position`)
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
	// position (F10) lets intermediate row states overlap until commit.
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
	// truncates to 32 bits and breaks handshake matching as values wrap
	// (PRD §11). Keep as a decimal string end-to-end.
	var txid string
	if err := tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text`).Scan(&txid); err != nil {
		return nil, err
	}

	cardRows, err := tx.Query(ctx, `SELECT id, label, position FROM card ORDER BY position`)
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
	return &ReorderResult{Cards: cs, Txid: txid}, nil
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
		if err := rows.Scan(&c.ID, &c.Label, &c.Position); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
