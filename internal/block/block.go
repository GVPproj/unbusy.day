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

// BlockType is a Block's tag: deep/shallow work, break, or appointment (a
// fixed-time commitment). A flat peer value — still movable. Immutable after creation.
type BlockType string

const (
	BlockDeep        BlockType = "deep"
	BlockShallow     BlockType = "shallow"
	BlockBreak       BlockType = "break"
	BlockAppointment BlockType = "appointment"
)

func (t BlockType) Valid() bool {
	switch t {
	case BlockDeep, BlockShallow, BlockBreak, BlockAppointment:
		return true
	}
	return false
}

type Block struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Position Slot      `json:"position"`
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

// IsRejection reports whether err is the User's doing (a rejected placement,
// a blank label, an unknown id) rather than a fault. Rejections surface as 200
// + a re-render of the authoritative column; every Err* in the package belongs here.
func IsRejection(err error) bool {
	for _, r := range []error{
		ErrNotSameBlocks, ErrOutOfBounds, ErrOverlap, ErrInvalidSpan,
		ErrInvalidBounds, ErrBoundsOccupied,
		ErrEmptyLabel, ErrBlockNotFound, ErrInvalidBlockType,
	} {
		if errors.Is(err, r) {
			return true
		}
	}
	return false
}

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

func (s *Service) Bounds(ctx context.Context, owner string) (Bounds, error) {
	return queryBounds(ctx, s.db, owner)
}

// Snapshot is the committed post-mutation state every mutator returns — the
// same state the Event fans out.
type Snapshot struct {
	Blocks []Block `json:"blocks"`
	Bounds Bounds  `json:"bounds"`
}

// commit runs fn inside the write transaction, reads the committed state back,
// and fans it out post-commit. fn sees the pre-mutation state for validation.
// _txlock=immediate takes the write lock at BeginTx, so reads inside the tx
// can't be invalidated by a concurrent writer before commit (ADRs 0005/0007).
func (s *Service) commit(
	ctx context.Context,
	owner string,
	fn func(ctx context.Context, tx *sql.Tx, bounds Bounds, current []Block) error,
) (*Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	bounds, err := queryBounds(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	current, err := queryBlocks(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := fn(ctx, tx, bounds, current); err != nil {
		return nil, err
	}

	snap := &Snapshot{}
	if snap.Blocks, err = queryBlocks(ctx, tx, owner); err != nil {
		return nil, err
	}
	if snap.Bounds, err = queryBounds(ctx, tx, owner); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.pub != nil {
		s.pub.Publish(Event{Owner: owner, Blocks: snap.Blocks, Bounds: snap.Bounds})
	}
	return snap, nil
}

// Create inserts a new span-1 block at slot; rejects a blank label, a slot
// outside bounds, or an occupied slot.
func (s *Service) Create(ctx context.Context, owner, label string, slot Slot, typ BlockType) (*Snapshot, error) {
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

	return s.commit(ctx, owner, func(ctx context.Context, tx *sql.Tx, bounds Bounds, current []Block) error {
		if !bounds.Contains(slot, 1) {
			return ErrOutOfBounds
		}
		if OccupiedSlots(current)[slot] {
			return ErrOverlap
		}
		id, err := newID()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO block (id, label, position, span, type, owner_id) VALUES (?, ?, ?, 1, ?, ?)`,
			id, label, slot, string(typ), owner)
		return err
	})
}

// Delete removes the owner's block by id; ErrBlockNotFound if they don't own it.
func (s *Service) Delete(ctx context.Context, owner, id string) (*Snapshot, error) {
	return s.commit(ctx, owner, func(ctx context.Context, tx *sql.Tx, _ Bounds, _ []Block) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM block WHERE id = ? AND owner_id = ?`, id, owner)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrBlockNotFound
		}
		return nil
	})
}

// Clear removes all the owner's blocks; bounds are untouched and an
// already-empty day is a harmless no-op.
func (s *Service) Clear(ctx context.Context, owner string) (*Snapshot, error) {
	return s.commit(ctx, owner, func(ctx context.Context, tx *sql.Tx, _ Bounds, _ []Block) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM block WHERE owner_id = ?`, owner)
		return err
	})
}

// Rename changes the owner's block label.
func (s *Service) Rename(ctx context.Context, owner, id, label string) (*Snapshot, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, ErrEmptyLabel
	}

	return s.commit(ctx, owner, func(ctx context.Context, tx *sql.Tx, _ Bounds, _ []Block) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE block SET label = ? WHERE id = ? AND owner_id = ?`, label, id, owner)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrBlockNotFound
		}
		return nil
	})
}

// SetLayout replaces the owner's whole layout in one mutation (ADR 0005): the
// client computes the push, the server enforces the invariants.
func (s *Service) SetLayout(ctx context.Context, owner string, layout []Placement) (*Snapshot, error) {
	return s.commit(ctx, owner, func(ctx context.Context, tx *sql.Tx, bounds Bounds, current []Block) error {
		if err := ValidateLayout(bounds, current, layout); err != nil {
			return err
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
		_, err := tx.ExecContext(ctx, b.String(), args...)
		return err
	})
}

// SetBounds edits the owner's day extent; a shrink onto an occupied slot
// rejects whole.
func (s *Service) SetBounds(ctx context.Context, owner string, start, end Slot) (*Snapshot, error) {
	next := Bounds{Start: start, End: end}
	if !next.Valid() {
		return nil, ErrInvalidBounds
	}

	return s.commit(ctx, owner, func(ctx context.Context, tx *sql.Tx, _ Bounds, current []Block) error {
		for _, c := range current {
			if !next.Contains(c.Position, c.Span) {
				return ErrBoundsOccupied
			}
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE "user" SET day_start = ?, day_end = ? WHERE id = ?`, start, end, owner)
		return err
	})
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
