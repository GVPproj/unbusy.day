// Package block is the transport-agnostic core service: the Datastar/SSE
// frontend drives one Service, so day-plan layout logic lives once.
// Every query is owner-scoped (ADR 0003): each User privately owns their Blocks.
package block

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

type Block struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Position int    `json:"position"`
	// Span is the block's height in stretch slots (≥1).
	Span int `json:"span"`
}

// Event is one mutation fanned out over the in-process pub/sub: the owner key
// plus the full ordered block list. The broker routes by Owner so a mutation
// only wakes that User's subscribers (ADR 0003).
type Event struct {
	Owner  string  `json:"owner"`
	Blocks []Block `json:"blocks"`
	Bounds Bounds  `json:"bounds"`
}

// Publisher is the seam between the core mutation and transport fan-out. The
// Service owns the publish call but not the bus, so the Broker can live in
// another package without an import cycle. A nil Publisher skips fan-out.
type Publisher interface {
	Publish(Event)
}

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

func (s *Service) List(ctx context.Context, owner string) ([]Block, error) {
	return queryBlocks(ctx, s.pool, owner, false)
}

// Seed gives a new User their starter blocks on first login (ADR 0003) so the
// day plan is populated before any create-block UI exists. No-op if the owner
// already has blocks; ids are generated, never hand-picked.
func (s *Service) Seed(ctx context.Context, owner string) error {
	labels := []string{"Alpha", "Bravo", "Charlie"}
	var b strings.Builder
	// Starter blocks take the first slots after the owner's day start, span 1.
	b.WriteString(`INSERT INTO block (id, label, position, owner_id) SELECT v.id, v.label, u.day_start + v.pos, $1 FROM (VALUES `)
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
	b.WriteString(`) AS v(id, label, pos), "user" u WHERE u.id = $1 AND NOT EXISTS (SELECT 1 FROM block WHERE owner_id = $1)`)
	_, err := s.pool.Exec(ctx, b.String(), args...)
	return err
}

// LayoutResult is the post-mutation column returned to the caller.
type LayoutResult struct {
	Blocks []Block `json:"blocks"`
}

// SetLayout replaces the owner's whole layout in one mutation (ADR 0005): the
// client computes the push, the server enforces the invariants via
// ValidateLayout inside the FOR UPDATE transaction.
func (s *Service) SetLayout(ctx context.Context, owner string, layout []Placement) (*LayoutResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	bounds, err := queryBounds(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	current, err := queryBlocks(ctx, tx, owner, true)
	if err != nil {
		return nil, err
	}
	if err := ValidateLayout(bounds, current, layout); err != nil {
		return nil, err
	}

	// Single bulk UPDATE; the DEFERRABLE EXCLUDE backstop tolerates
	// transiently overlapping intermediate row states until commit.
	var b strings.Builder
	b.WriteString(`UPDATE block AS c SET position = v.slot, span = v.span FROM (VALUES `)
	args := make([]any, 0, len(layout)*3+1)
	for i, p := range layout {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "($%d::text, $%d::int, $%d::int)", i*3+1, i*3+2, i*3+3)
		args = append(args, p.ID, p.Slot, p.Span)
	}
	fmt.Fprintf(&b, `) AS v(id, slot, span) WHERE c.id = v.id AND c.owner_id = $%d`, len(args)+1)
	args = append(args, owner)
	if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
		return nil, err
	}

	bs, err := queryBlocks(ctx, tx, owner, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Fan out post-commit so subscribers never observe an uncommitted layout.
	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: bounds})
	}
	return &LayoutResult{Blocks: bs}, nil
}

// Bounds reads the owner's day bounds for the render path.
func (s *Service) Bounds(ctx context.Context, owner string) (Bounds, error) {
	return queryBounds(ctx, s.pool, owner)
}

// SetBounds edits the owner's day extent. Hard limits 5:00–18:00, end after
// start; the day may only shrink into empty slots — a shrink onto an occupied
// slot rejects whole, same shape as a layout rejection.
func (s *Service) SetBounds(ctx context.Context, owner string, start, end int) error {
	if start < MinDayStart || end > MaxDayEnd || end <= start {
		return ErrInvalidBounds
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Locked so a concurrent layout mutation can't slip a block outside the
	// new bounds between check and commit.
	bs, err := queryBlocks(ctx, tx, owner, true)
	if err != nil {
		return err
	}
	for _, c := range bs {
		if c.Position < start || c.Position+c.Span > end {
			return ErrBoundsOccupied
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE "user" SET day_start = $2, day_end = $3 WHERE id = $1`, owner, start, end); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Fan out post-commit so live tabs re-render the grid at its new extent.
	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: Bounds{Start: start, End: end}})
	}
	return nil
}

// querier is the read surface shared by *pgxpool.Pool and pgx.Tx, so the same
// query helpers run on the render path (pool) and inside a mutation (tx).
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// queryBlocks reads the owner's blocks in position order. lock=true adds FOR
// UPDATE to serialise concurrent mutations (callable only inside a tx). The
// explicit column list lives here once — keep it in sync with scanBlocks.
func queryBlocks(ctx context.Context, q querier, owner string, lock bool) ([]Block, error) {
	sql := `SELECT id, label, position, span FROM block WHERE owner_id = $1 ORDER BY position`
	if lock {
		sql += " FOR UPDATE"
	}
	rows, err := q.Query(ctx, sql, owner)
	if err != nil {
		return nil, err
	}
	return scanBlocks(rows)
}

// queryBounds reads the owner's day extent.
func queryBounds(ctx context.Context, q querier, owner string) (Bounds, error) {
	var b Bounds
	err := q.QueryRow(ctx,
		`SELECT day_start, day_end FROM "user" WHERE id = $1`, owner).Scan(&b.Start, &b.End)
	return b, err
}

// newID is a generated unique block id — hand-picked ids can't repeat across Users.
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func scanBlocks(rows pgx.Rows) ([]Block, error) {
	defer rows.Close()
	var out []Block
	for rows.Next() {
		var c Block
		if err := rows.Scan(&c.ID, &c.Label, &c.Position, &c.Span); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
