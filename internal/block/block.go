// Package block is the transport-agnostic core service. All mutation logic
// lives here once; every query is owner-scoped (ADR 0003).
package block

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// BlockType is a Block's tag: deep/shallow work or break. Immutable after creation.
type BlockType string

const (
	BlockDeep    BlockType = "deep"
	BlockShallow BlockType = "shallow"
	BlockBreak   BlockType = "break"
)

func (t BlockType) Valid() bool {
	switch t {
	case BlockDeep, BlockShallow, BlockBreak:
		return true
	}
	return false
}

type Block struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Position int       `json:"position"`
	Span     int       `json:"span"` // height in slots (≥1)
	Type     BlockType `json:"type"`
}

// Event is a full post-mutation snapshot, routed by Owner (ADR 0003).
type Event struct {
	Owner  string  `json:"owner"`
	Blocks []Block `json:"blocks"`
	Bounds Bounds  `json:"bounds"`
}

// Publisher is the pub/sub seam; nil skips fan-out. The Service publishes
// post-commit only, so subscribers never see uncommitted state.
type Publisher interface {
	Publish(Event)
}

var ErrInvalidSpan = errors.New("span must be at least 1")
var ErrEmptyLabel = errors.New("block label is required")
var ErrBlockNotFound = errors.New("block not found")
var ErrInvalidBlockType = errors.New("invalid block type")

type Service struct {
	db  *sql.DB
	pub Publisher
}

func NewService(db *sql.DB, pub Publisher) *Service {
	return &Service{db: db, pub: pub}
}

func (s *Service) List(ctx context.Context, owner string) ([]Block, error) {
	return queryBlocks(ctx, s.db, owner)
}

// Seed inserts starter blocks on first login; no-op if the owner has blocks.
func (s *Service) Seed(ctx context.Context, owner string) error {
	labels := []string{"Alpha", "Bravo", "Charlie"}
	var b strings.Builder
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

type LayoutResult struct {
	Blocks []Block `json:"blocks"`
}

type CreateResult struct {
	Blocks []Block `json:"blocks"`
}

// Create inserts a new span-1 block at slot; rejects a blank label, a slot
// outside bounds, or an occupied slot.
func (s *Service) Create(ctx context.Context, owner, label string, slot int, typ BlockType) (*CreateResult, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, ErrEmptyLabel
	}
	typ = BlockType(strings.TrimSpace(string(typ)))
	if typ == "" {
		typ = BlockShallow
	}
	if !typ.Valid() {
		return nil, ErrInvalidBlockType
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	bounds, err := queryBounds(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if slot < bounds.Start || slot >= bounds.End {
		return nil, ErrOutOfBounds
	}
	current, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if OccupiedSlots(current)[slot] {
		return nil, ErrOverlap
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO block (id, label, position, span, type, owner_id) VALUES (?, ?, ?, 1, ?, ?)`,
		id, label, slot, string(typ), owner); err != nil {
		return nil, err
	}

	bs, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: bounds})
	}
	return &CreateResult{Blocks: bs}, nil
}

type DeleteResult struct {
	Blocks []Block `json:"blocks"`
}

// Delete removes the owner's block by id; ErrBlockNotFound if they don't own it.
func (s *Service) Delete(ctx context.Context, owner, id string) (*DeleteResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	bounds, err := queryBounds(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx,
		`DELETE FROM block WHERE id = ? AND owner_id = ?`, id, owner)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, ErrBlockNotFound
	}

	bs, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: bounds})
	}
	return &DeleteResult{Blocks: bs}, nil
}

type ClearResult struct {
	Blocks []Block `json:"blocks"`
}

// Clear removes all the owner's blocks; bounds are untouched and an
// already-empty day is a harmless no-op.
func (s *Service) Clear(ctx context.Context, owner string) (*ClearResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	bounds, err := queryBounds(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM block WHERE owner_id = ?`, owner); err != nil {
		return nil, err
	}

	bs, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: bounds})
	}
	return &ClearResult{Blocks: bs}, nil
}

type RenameResult struct {
	Blocks []Block `json:"blocks"`
}

// Rename changes the owner's block label.
func (s *Service) Rename(ctx context.Context, owner, id, label string) (*RenameResult, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, ErrEmptyLabel
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	bounds, err := queryBounds(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE block SET label = ? WHERE id = ? AND owner_id = ?`, label, id, owner)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, ErrBlockNotFound
	}

	bs, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: bounds})
	}
	return &RenameResult{Blocks: bs}, nil
}

// SetLayout replaces the owner's whole layout in one mutation (ADR 0005): the
// client computes the push, the server enforces the invariants.
// _txlock=immediate takes the write lock at BeginTx, so reads inside the tx
// can't be invalidated by a concurrent writer before commit.
func (s *Service) SetLayout(ctx context.Context, owner string, layout []Placement) (*LayoutResult, error) {
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

	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: bounds})
	}
	return &LayoutResult{Blocks: bs}, nil
}

func (s *Service) Bounds(ctx context.Context, owner string) (Bounds, error) {
	return queryBounds(ctx, s.db, owner)
}

// SetBounds edits the owner's day extent; a shrink onto an occupied slot
// rejects whole.
func (s *Service) SetBounds(ctx context.Context, owner string, start, end int) error {
	if start < MinDayStart || end > MaxDayEnd || end <= start {
		return ErrInvalidBounds
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

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

	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: bs, Bounds: Bounds{Start: start, End: end}})
	}
	return nil
}

// querier is the read surface shared by *sql.DB and *sql.Tx.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func queryBlocks(ctx context.Context, q querier, owner string) ([]Block, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, label, position, span, type FROM block WHERE owner_id = ? ORDER BY position`, owner)
	if err != nil {
		return nil, err
	}
	return scanBlocks(rows)
}

func queryBounds(ctx context.Context, q querier, owner string) (Bounds, error) {
	var b Bounds
	err := q.QueryRowContext(ctx,
		`SELECT day_start, day_end FROM "user" WHERE id = ?`, owner).Scan(&b.Start, &b.End)
	return b, err
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func scanBlocks(rows *sql.Rows) ([]Block, error) {
	defer func() { _ = rows.Close() }()
	var out []Block
	for rows.Next() {
		var c Block
		if err := rows.Scan(&c.ID, &c.Label, &c.Position, &c.Span, &c.Type); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
