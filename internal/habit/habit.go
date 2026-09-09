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

// CurrentCalendar uses the service clock and browser timezone, without storage reads.
func (s *Service) CurrentCalendar(timezone string) (Month, error) {
	return Calendar(timezone, s.now())
}

// MonthSnapshot reads one view-selected month without storing that selection.
func (s *Service) MonthSnapshot(ctx context.Context, owner, timezone, key string) (*Snapshot, error) {
	month, err := CalendarMonth(timezone, key, s.now())
	if err != nil {
		return nil, err
	}
	habits, err := listMonth(ctx, s.db, owner, month)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Habits: habits, Month: month}, nil
}

func (s *Service) List(ctx context.Context, owner string) ([]Habit, error) {
	rows, err := s.db.QueryContext(ctx, `
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

func listMonth(ctx context.Context, db *sql.DB, owner string, month Month) ([]Habit, error) {
	first, last := month.Dates[0], month.Dates[len(month.Dates)-1]
	rows, err := db.QueryContext(ctx, `
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
func (s *Service) SetCheckIn(ctx context.Context, owner string, habitID int64, date string, checked bool, timezone string) error {
	current, err := s.CurrentCalendar(timezone)
	if err != nil {
		return err
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return rejection("Choose a valid check-in date.")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var startDate string
	if err := tx.QueryRowContext(ctx, `SELECT start_date FROM habit WHERE id = ? AND owner_id = ?`, habitID, owner).Scan(&startDate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rejection("Habit not found.")
		}
		return err
	}
	if date < startDate {
		return rejection("Check-in date cannot be before the habit started.")
	}
	if date > current.Today {
		return rejection("Check-in date cannot be after today.")
	}
	if checked {
		_, err = tx.ExecContext(ctx, `INSERT INTO habit_checkin (habit_id, date) VALUES (?, ?) ON CONFLICT (habit_id, date) DO NOTHING`, habitID, date)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM habit_checkin WHERE habit_id = ? AND date = ?`, habitID, date)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.pub != nil {
		s.pub.PublishHabit(Event{Owner: owner})
	}
	return nil
}

func (s *Service) validateDefinition(name, startDate, timezone string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", rejection("Enter a habit name.")
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 {
		return "", rejection("Habit names must contain at most 80 Unicode characters.")
	}
	month, err := s.CurrentCalendar(timezone)
	if err != nil {
		return "", err
	}
	if parsed, err := time.Parse(time.DateOnly, startDate); err != nil || parsed.Format(time.DateOnly) != startDate {
		return "", rejection("Enter a valid start date (yyyy-mm-dd).")
	}
	if startDate > month.Today {
		return "", rejection("Start date cannot be after today.")
	}
	return name, nil
}

func (s *Service) Create(ctx context.Context, owner, name, startDate, timezone string) error {
	name, err := s.validateDefinition(name, startDate, timezone)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Keep the high-water mark even when the maximum or last habit is deleted.
	var id int64
	if err := tx.QueryRowContext(ctx, `UPDATE habit_id_allocator SET last_id = MAX(last_id, COALESCE((SELECT MAX(id) FROM habit), 0)) + 1 WHERE singleton = 1 RETURNING last_id`).Scan(&id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO habit (id,owner_id,name,name_key,start_date) VALUES (?,?,?,?,?) ON CONFLICT (owner_id,name_key) DO NOTHING`, id, owner, name, foldKey(name), startDate)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		return rejection("A habit with this name already exists.")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.pub != nil {
		s.pub.PublishHabit(Event{Owner: owner})
	}
	return nil
}

// Delete permanently removes an owned habit and its check-ins, including on retries.
func (s *Service) Delete(ctx context.Context, owner string, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM habit WHERE id = ? AND owner_id = ?`, id, owner); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Missing IDs still invalidate the caller's stale view, never another owner's.
	if s.pub != nil {
		s.pub.PublishHabit(Event{Owner: owner})
	}
	return nil
}

// Edit updates one owned habit without changing its identity or check-ins.
func (s *Service) Edit(ctx context.Context, owner string, habitID int64, name, startDate, timezone string) error {
	name, err := s.validateDefinition(name, startDate, timezone)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var earliest sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT MIN(c.date)
		FROM habit h
		LEFT JOIN habit_checkin c ON c.habit_id = h.id
		WHERE h.id = ? AND h.owner_id = ?
		GROUP BY h.id`, habitID, owner).Scan(&earliest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rejection("Habit not found.")
		}
		return err
	}
	if earliest.Valid && startDate > earliest.String {
		return rejection("Start date cannot be after an existing check-in.")
	}
	var duplicate int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM habit WHERE owner_id = ? AND name_key = ? AND id != ?`, owner, foldKey(name), habitID).Scan(&duplicate)
	if err == nil {
		return rejection("A habit with this name already exists.")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE habit SET name = ?, name_key = ?, start_date = ? WHERE id = ? AND owner_id = ?`, name, foldKey(name), startDate, habitID, owner); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.pub != nil {
		s.pub.PublishHabit(Event{Owner: owner})
	}
	return nil
}
