// Package jot is the transport-agnostic Jotpad service: one perpetual
// free-text scratchpad per user, owner-scoped like everything else (ADR 0003).
// It deliberately shares nothing with block — a Jotpad has no overlap, no push
// cascade, and no validation beyond a length cap. Concurrent writers are
// reconciled by a version counter: a write from the current version replaces,
// a stale write is three-way merged against the base it was typed on.
package jot

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// MaxLen caps a Jotpad at 100,000 characters. Enforced here and by maxlength
// on the textarea; a browser cannot exceed it, only a crafted client can.
const MaxLen = 100_000

var ErrTooLong = errors.New("jot exceeds the length cap")

// Pad is the committed state: the text and the version that stored it.
type Pad struct {
	Text    string `json:"text"`
	Version int64  `json:"version"`
}

// Event is a full post-commit snapshot, routed by Owner (ADR 0003).
type Event struct {
	Owner   string `json:"owner"`
	Version int64  `json:"version"`
	Text    string `json:"text"`
}

// Publisher is the pub/sub seam; nil skips fan-out. The Service publishes
// post-commit only, so subscribers never see uncommitted state.
type Publisher interface {
	PublishJot(Event)
}

type Service struct {
	db  *sql.DB
	pub Publisher
}

func NewService(db *sql.DB, pub Publisher) *Service {
	return &Service{db: db, pub: pub}
}

// Get returns the owner's Jotpad. A Session cannot exist without a User row, so
// the row always exists and an empty scratchpad is the column default — there
// is no "not created yet" case to branch on.
func (s *Service) Get(ctx context.Context, owner string) (Pad, error) {
	var p Pad
	err := s.db.QueryRowContext(ctx,
		`SELECT jot, jot_version FROM "user" WHERE id = ?`, owner).
		Scan(&p.Text, &p.Version)
	return p, err
}

// Set stores the owner's Jotpad — exactly the characters given, no trimming —
// and returns the committed state. base is the version the client's text was
// typed on: when it matches the stored version the text replaces wholesale;
// when it is stale the client's edit is three-way merged into the stored text
// (base = the shadowed previous revision) so neither side's typing is lost.
// Over-cap writes and over-cap merge results are rejected whole with
// ErrTooLong; the returned Pad is then the untouched authoritative state.
func (s *Service) Set(ctx context.Context, owner, text string, base int64) (Pad, error) {
	if utf8.RuneCountInString(text) > MaxLen {
		p, err := s.Get(ctx, owner)
		if err != nil {
			return Pad{}, err
		}
		return p, ErrTooLong
	}

	// _txlock=immediate takes the write lock at BeginTx, so the read below
	// can't be invalidated by a concurrent writer before commit (ADR 0007).
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Pad{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var stored Pad
	if err := tx.QueryRowContext(ctx,
		`SELECT jot, jot_version FROM "user" WHERE id = ?`, owner).
		Scan(&stored.Text, &stored.Version); err != nil {
		return Pad{}, err
	}

	next := text
	if base != stored.Version {
		merged, err := s.merge(ctx, tx, owner, stored, text, base)
		if err != nil {
			return Pad{}, err
		}
		if utf8.RuneCountInString(merged) > MaxLen {
			return stored, ErrTooLong
		}
		next = merged
	}

	committed := Pad{Text: next, Version: stored.Version + 1}
	if _, err := tx.ExecContext(ctx,
		`UPDATE "user" SET jot = ?, jot_version = ? WHERE id = ?`,
		committed.Text, committed.Version, owner); err != nil {
		return Pad{}, err
	}
	// Shadow the revision just replaced — the merge base for the next stale
	// writer. Previous revision only, never history.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO jot_base (owner, version, text) VALUES (?, ?, ?)
		 ON CONFLICT (owner) DO UPDATE SET version = excluded.version, text = excluded.text`,
		owner, stored.Version, stored.Text); err != nil {
		return Pad{}, err
	}
	if err := tx.Commit(); err != nil {
		return Pad{}, err
	}

	if s.pub != nil {
		s.pub.PublishJot(Event{Owner: owner, Version: committed.Version, Text: committed.Text})
	}
	return committed, nil
}

// merge reconciles a stale write: diff the client's text against the base it
// was typed on, then fuzzy-apply those patches to the stored text. When the
// client's base is older than the one shadow revision, the shadow is still the
// closest base we have — diff-match-patch is built for fuzzy application, and
// an unpatchable hunk keeps the stored text (the response then visibly snaps
// the client to the authoritative result).
func (s *Service) merge(ctx context.Context, tx *sql.Tx, owner string, stored Pad, theirs string, base int64) (string, error) {
	var baseText string
	var baseVersion int64
	err := tx.QueryRowContext(ctx,
		`SELECT version, text FROM jot_base WHERE owner = ?`, owner).
		Scan(&baseVersion, &baseText)
	if errors.Is(err, sql.ErrNoRows) {
		// No shadow yet: the only text ever stored is the current one.
		baseText, baseVersion = stored.Text, stored.Version
	} else if err != nil {
		return "", err
	}
	if base != baseVersion {
		// Instrumented per PRD: does one shadow revision suffice in practice?
		log.Printf("jot merge fallback: client base v%d, shadow v%d (owner %s)", base, baseVersion, owner)
	}

	dmp := diffmatchpatch.New()
	patches := dmp.PatchMake(baseText, theirs)
	merged, _ := dmp.PatchApply(patches, stored.Text)
	return merged, nil
}
