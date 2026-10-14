// Package jot manages three perpetual notes per owner (ADR 0003).
// Each note independently reconciles stale writes against one shadow revision.
package jot

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const (
	Daily     = "daily"
	Weekly    = "weekly"
	Quarterly = "quarterly"
)

// MaxLen caps each Note at 100,000 characters. Enforced here and by the
// editor's transaction filter; only a crafted client can exceed it.
const MaxLen = 100_000

var (
	ErrTooLong     = errors.New("jot exceeds the length cap")
	ErrInvalidNote = errors.New("unknown jot note")
)

// ValidNote reports whether note names one of the fixed planning horizons.
func ValidNote(note string) bool {
	return note == Daily || note == Weekly || note == Quarterly
}

// Pad is the committed state: the text and the version that stored it.
type Pad struct {
	Text    string `json:"text"`
	Version int64  `json:"version"`
}

// Event is a full post-commit snapshot, routed by Owner (ADR 0003).
type Event struct {
	Owner   string `json:"owner"`
	Note    string `json:"note"`
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

// Get returns a Note; unwritten notes are empty at v0, but unknown owners error.
func (s *Service) Get(ctx context.Context, owner, note string) (Pad, error) {
	if !ValidNote(note) {
		return Pad{}, ErrInvalidNote
	}
	var p Pad
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(n.text, ''), COALESCE(n.version, 0)
		 FROM "user" u LEFT JOIN jot_note n ON n.owner = u.id AND n.note = ?
		 WHERE u.id = ?`, note, owner).
		Scan(&p.Text, &p.Version)
	return p, err
}

// Set stores the owner's Note — exactly the characters given, no trimming —
// and returns the committed state. base is the version the client's text was
// typed on: when it matches the stored version the text replaces wholesale;
// when it is stale the client's edit is three-way merged into the stored text
// (base = the shadowed previous revision) so neither side's typing is lost.
// Over-cap writes and over-cap merge results are rejected whole with
// ErrTooLong; the returned Pad is then the untouched authoritative state.
func (s *Service) Set(ctx context.Context, owner, note, text string, base int64) (Pad, error) {
	if !ValidNote(note) {
		return Pad{}, ErrInvalidNote
	}
	if utf8.RuneCountInString(text) > MaxLen {
		p, err := s.Get(ctx, owner, note)
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
		`SELECT COALESCE(n.text, ''), COALESCE(n.version, 0)
		 FROM "user" u LEFT JOIN jot_note n ON n.owner = u.id AND n.note = ?
		 WHERE u.id = ?`, note, owner).
		Scan(&stored.Text, &stored.Version); err != nil {
		return Pad{}, err
	}

	next := text
	if base != stored.Version {
		merged, err := s.merge(ctx, tx, owner, note, stored, text, base)
		if err != nil {
			return Pad{}, err
		}
		if utf8.RuneCountInString(merged) > MaxLen {
			return stored, ErrTooLong
		}
		next = merged
	}

	committed := Pad{Text: next, Version: stored.Version + 1}
	// Store the new revision and its single merge base atomically.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO jot_note (owner, note, text, version, base_text, base_version)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (owner, note) DO UPDATE SET text = excluded.text,
		 version = excluded.version, base_text = excluded.base_text, base_version = excluded.base_version`,
		owner, note, committed.Text, committed.Version, stored.Text, stored.Version); err != nil {
		return Pad{}, err
	}
	if err := tx.Commit(); err != nil {
		return Pad{}, err
	}

	if s.pub != nil {
		s.pub.PublishJot(Event{Owner: owner, Note: note, Version: committed.Version, Text: committed.Text})
	}
	return committed, nil
}

// merge reconciles a stale write: diff the client's text against the base it
// was typed on, then fuzzy-apply those patches to the stored text. When the
// client's base is older than the one shadow revision, the shadow is still the
// closest base we have — diff-match-patch is built for fuzzy application, and
// an unpatchable hunk keeps the stored text (the response then visibly snaps
// the client to the authoritative result).
func (s *Service) merge(ctx context.Context, tx *sql.Tx, owner, note string, stored Pad, theirs string, base int64) (string, error) {
	var baseText string
	var baseVersion int64
	err := tx.QueryRowContext(ctx,
		`SELECT base_version, base_text FROM jot_note
		 WHERE owner = ? AND note = ? AND base_version IS NOT NULL`, owner, note).
		Scan(&baseVersion, &baseText)
	if errors.Is(err, sql.ErrNoRows) {
		// No shadow yet: the only text ever stored is the current one.
		baseText, baseVersion = stored.Text, stored.Version
	} else if err != nil {
		return "", err
	}
	if base != baseVersion {
		// Instrumented to answer whether one shadow revision suffices in practice.
		log.Printf("jot merge fallback: client base v%d, shadow v%d (owner %s, note %s)", base, baseVersion, owner, note)
	}

	dmp := diffmatchpatch.New()
	patches := dmp.PatchMake(baseText, theirs)
	merged, _ := dmp.PatchApply(patches, stored.Text)
	return merged, nil
}
