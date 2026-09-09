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
	ID           int64
	Name         string
	StartDate    string
	CheckedDates []string
}

// Event invalidates the owner's habit list; readers fetch current state.
type Event struct{ Owner string }

type Publisher interface{ PublishHabit(Event) }

type Service struct {
	db  *sql.DB
	pub Publisher
	now func() time.Time
}

type Option func(*Service)

// WithClock makes civil-date behavior deterministic at the service boundary.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

func NewService(db *sql.DB, pub Publisher, opts ...Option) *Service {
	s := &Service{db: db, pub: pub, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type Snapshot struct {
	Habits []Habit
	Month  Month
}

func (s *Service) Snapshot(ctx context.Context, owner, timezone string) (*Snapshot, error) {
	month, err := Calendar(timezone, s.now())
	if err != nil {
		return nil, err
	}
	return s.snapshotMonth(ctx, owner, month)
}

// MonthSnapshot reads one view-selected month without storing that selection.
func (s *Service) MonthSnapshot(ctx context.Context, owner, timezone, key string) (*Snapshot, error) {
	month, err := CalendarMonth(timezone, key, s.now())
	if err != nil {
		return nil, err
	}
	return s.snapshotMonth(ctx, owner, month)
}

func (s *Service) snapshotMonth(ctx context.Context, owner string, month Month) (*Snapshot, error) {
	habits, err := listMonth(ctx, s.db, owner, month)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Habits: habits, Month: month}, nil
}

func (s *Service) List(ctx context.Context, owner string) ([]Habit, error) {
	return list(ctx, s.db, owner)
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func list(ctx context.Context, q querier, owner string) ([]Habit, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT h.id, h.name, h.start_date, c.date
		FROM habit h
		LEFT JOIN habit_checkin c ON c.habit_id = h.id
		WHERE h.owner_id = ?
		ORDER BY h.id, c.date`, owner)
	if err != nil {
		return nil, err
	}
	return scanHabits(rows)
}

func listMonth(ctx context.Context, q querier, owner string, month Month) ([]Habit, error) {
	first, last := month.Dates[0], month.Dates[len(month.Dates)-1]
	rows, err := q.QueryContext(ctx, `
		SELECT h.id, h.name, h.start_date, c.date
		FROM habit h
		LEFT JOIN habit_checkin c ON c.habit_id = h.id AND c.date >= ? AND c.date <= ?
		WHERE h.owner_id = ? AND h.start_date <= ?
		ORDER BY h.id, c.date`, first, last, owner, last)
	if err != nil {
		return nil, err
	}
	return scanHabits(rows)
}

func scanHabits(rows *sql.Rows) ([]Habit, error) {
	defer rows.Close()
	habits := make([]Habit, 0)
	for rows.Next() {
		var id int64
		var name, startDate string
		var checkedDate sql.NullString
		if err := rows.Scan(&id, &name, &startDate, &checkedDate); err != nil {
			return nil, err
		}
		if len(habits) == 0 || habits[len(habits)-1].ID != id {
			habits = append(habits, Habit{ID: id, Name: name, StartDate: startDate})
		}
		if checkedDate.Valid {
			h := &habits[len(habits)-1]
			h.CheckedDates = append(h.CheckedDates, checkedDate.String)
		}
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

// SetCheckIn records an explicit checked state for one owned habit and civil date.
func (s *Service) SetCheckIn(ctx context.Context, owner string, habitID int64, date string, checked bool, timezone string) (*Snapshot, error) {
	now := s.now()
	current, err := Calendar(timezone, now)
	if err != nil {
		return nil, err
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return nil, rejection("Choose a valid check-in date.")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var startDate string
	if err := tx.QueryRowContext(ctx, `SELECT start_date FROM habit WHERE id = ? AND owner_id = ?`, habitID, owner).Scan(&startDate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, rejection("Habit not found.")
		}
		return nil, err
	}
	if date < startDate {
		return nil, rejection("Check-in date cannot be before the habit started.")
	}
	if date > current.Today {
		return nil, rejection("Check-in date cannot be after today.")
	}
	month, err := CalendarMonth(timezone, date[:7], now)
	if err != nil {
		return nil, err
	}
	if checked {
		_, err = tx.ExecContext(ctx, `INSERT INTO habit_checkin (habit_id, date) VALUES (?, ?) ON CONFLICT (habit_id, date) DO NOTHING`, habitID, date)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM habit_checkin WHERE habit_id = ? AND date = ?`, habitID, date)
	}
	if err != nil {
		return nil, err
	}
	habits, err := listMonth(ctx, tx, owner, month)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if s.pub != nil {
		s.pub.PublishHabit(Event{Owner: owner})
	}
	return &Snapshot{Habits: habits, Month: month}, nil
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
