// Package block is the transport-agnostic core service: the Datastar/SSE
// frontend drives one Service, so day-plan layout logic lives once.
// Every query is owner-scoped (ADR 0003): each User privately owns their Blocks.
package block

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
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
	db  *sql.DB
	pub Publisher
}

// NewService wires the core service over a SQLite handle and a Publisher. pub
// may be nil (e.g. unit tests with no bus): Reorder then skips fan-out.
func NewService(db *sql.DB, pub Publisher) *Service {
	return &Service{db: db, pub: pub}
}

func (s *Service) List(ctx context.Context, owner string) ([]Block, error) {
	return queryBlocks(ctx, s.db, owner)
}

// Seed gives a new User their starter blocks on first login (ADR 0003) so the
// day plan is populated before any create-block UI exists. No-op if the owner
// already has blocks; ids are generated, never hand-picked.
func (s *Service) Seed(ctx context.Context, owner string) error {
	labels := []string{"Alpha", "Bravo", "Charlie"}
	var b strings.Builder
	// Starter blocks take the first slots after the owner's day start, span 1.
	// SQLite has no column-alias on a VALUES subquery, so the row set is a CTE.
	b.WriteString(`WITH v(id, label, pos) AS (VALUES `)
	args := make([]any, 0, len(labels)*3+3)
	for i, label := range labels {
		id, err := newID()
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(?, ?, ?)")
		args = append(args, id, label, i)
	}
	b.WriteString(`) INSERT INTO block (id, label, position, owner_id) SELECT v.id, v.label, u.day_start + v.pos, ? FROM v, "user" u WHERE u.id = ? AND NOT EXISTS (SELECT 1 FROM block WHERE owner_id = ?)`)
	args = append(args, owner, owner, owner)
	_, err := s.db.ExecContext(ctx, b.String(), args...)
	return err
}

// LayoutResult is the post-mutation column returned to the caller.
type LayoutResult struct {
	Blocks []Block `json:"blocks"`
}

// SetLayout replaces the owner's whole layout in one mutation (ADR 0005): the
// client computes the push, the server enforces the invariants via
// ValidateLayout inside the write transaction (SQLite serializes writes).
func (s *Service) SetLayout(ctx context.Context, owner string, layout []Placement) (*LayoutResult, error) {
	// _txlock=immediate takes the write lock at BeginTx, so these reads can't be
	// invalidated by a concurrent writer before we validate and commit.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	bounds, err := queryBounds(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	current, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := ValidateLayout(bounds, current, layout); err != nil {
		return nil, err
	}

	// Single bulk UPDATE. SQLite serializes writes at the database level, so no
	// row-level locking is needed; intermediate overlaps never surface.
	var b strings.Builder
	b.WriteString(`WITH v(id, slot, span) AS (VALUES `)
	args := make([]any, 0, len(layout)*3+1)
	for i, p := range layout {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(?, ?, ?)")
		args = append(args, p.ID, p.Slot, p.Span)
	}
	b.WriteString(`) UPDATE block AS c SET position = v.slot, span = v.span FROM v WHERE c.id = v.id AND c.owner_id = ?`)
	args = append(args, owner)
	if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
		return nil, err
	}

	bs, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
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
	return queryBounds(ctx, s.db, owner)
}

// SetBounds edits the owner's day extent. Hard limits 5:00–18:00, end after
// start; the day may only shrink into empty slots — a shrink onto an occupied
// slot rejects whole, same shape as a layout rejection.
func (s *Service) SetBounds(ctx context.Context, owner string, start, end int) error {
	if start < MinDayStart || end > MaxDayEnd || end <= start {
		return ErrInvalidBounds
	}

	// _txlock=immediate takes the write lock at BeginTx, so a concurrent layout
	// mutation can't slip a block outside the new bounds between this check and
	// commit.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	bs, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return err
	}
	for _, c := range bs {
		if c.Position < start || c.Position+c.Span > end {
			return ErrBoundsOccupied
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE "user" SET day_start = ?, day_end = ? WHERE id = ?`, start, end, owner); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// Fan out post-commit so live tabs re-render the grid at its new extent.
	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: Bounds{Start: start, End: end}})
	}
	return nil
}

// querier is the read surface shared by *sql.DB and *sql.Tx, so the same query
// helpers run on the render path (db) and inside a mutation (tx).
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// queryBlocks reads the owner's blocks in position order. Runs on both the
// render path (db) and inside a mutation tx. The explicit column list lives
// here once — keep it in sync with scanBlocks.
func queryBlocks(ctx context.Context, q querier, owner string) ([]Block, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, label, position, span FROM block WHERE owner_id = ? ORDER BY position`, owner)
	if err != nil {
		return nil, err
	}
	return scanBlocks(rows)
}

// queryBounds reads the owner's day extent.
func queryBounds(ctx context.Context, q querier, owner string) (Bounds, error) {
	var b Bounds
	err := q.QueryRowContext(ctx,
		`SELECT day_start, day_end FROM "user" WHERE id = ?`, owner).Scan(&b.Start, &b.End)
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

func scanBlocks(rows *sql.Rows) ([]Block, error) {
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
