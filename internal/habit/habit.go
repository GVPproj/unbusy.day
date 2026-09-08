// Package habit stores a user's perpetual habits, independent of the displayed month.
package habit

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type rejection string

func (e rejection) Error() string { return string(e) }

// IsRejection distinguishes invalid input from infrastructure failures.
func IsRejection(err error) bool {
	var target rejection
	return errors.As(err, &target)
}

type Habit struct {
	ID        int64
	Name      string
	StartDate string
}

// Event invalidates the owner's habit list; readers fetch current state.
type Event struct{ Owner string }

type Publisher interface{ PublishHabit(Event) }

type Service struct {
	db  *sql.DB
	pub Publisher
	now func() time.Time
}

func NewService(db *sql.DB, pub Publisher) *Service { return &Service{db: db, pub: pub, now: time.Now} }

func (s *Service) List(ctx context.Context, owner string) ([]Habit, error) {
	return list(ctx, s.db, owner)
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func list(ctx context.Context, q querier, owner string) ([]Habit, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, start_date FROM habit WHERE owner_id = ? ORDER BY id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	habits := make([]Habit, 0)
	for rows.Next() {
		var h Habit
		if err := rows.Scan(&h.ID, &h.Name, &h.StartDate); err != nil {
			return nil, err
		}
		habits = append(habits, h)
	}
	return habits, rows.Err()
}

// foldKey chooses one representative per Unicode simple-fold cycle (EqualFold semantics).
func foldKey(name string) string {
	return strings.Map(func(r rune) rune {
		key := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < key {
				key = next
			}
		}
		return key
	}, name)
}

func (s *Service) Create(ctx context.Context, owner, name, startDate, timezone string) ([]Habit, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, rejection("Enter a habit name.")
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 {
		return nil, rejection("Habit names must contain at most 80 Unicode characters.")
	}
	month, err := Calendar(timezone, s.now())
	if err != nil {
		return nil, err
	}
	if _, err := time.Parse(time.DateOnly, startDate); err != nil {
		return nil, rejection("Enter a valid start date (yyyy-mm-dd).")
	}
	if startDate > month.Today {
		return nil, rejection("Start date cannot be after today.")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO habit (owner_id,name,name_key,start_date) VALUES (?,?,?,?) ON CONFLICT (owner_id,name_key) DO NOTHING`, owner, name, foldKey(name), startDate)
	if err != nil {
		return nil, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if inserted == 0 {
		return nil, rejection("A habit with this name already exists.")
	}
	habits, err := list(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if s.pub != nil {
		s.pub.PublishHabit(Event{Owner: owner})
	}
	return habits, nil
}
