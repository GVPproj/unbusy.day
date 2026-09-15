package habit

import (
	"time"
	_ "time/tzdata" // Production's scratch image has no system zoneinfo.
)

type Week struct {
	Today    string
	Key      string
	Label    string
	Dates    []string
	Previous string
	Next     string
	Current  bool
}

// CurrentWeek returns the Sunday-to-Saturday week in the browser's timezone.
func CurrentWeek(timezone string, now time.Time) (Week, error) {
	local, err := localTime(timezone, now)
	if err != nil {
		return Week{}, err
	}
	return calendarWeek(local, weekStart(local).Format(time.DateOnly)), nil
}

// CalendarWeek returns a selected week no later than the browser's local week.
func CalendarWeek(timezone, key string, now time.Time) (Week, error) {
	local, err := localTime(timezone, now)
	if err != nil {
		return Week{}, err
	}
	selected, err := time.Parse(time.DateOnly, key)
	if err != nil || selected.Format(time.DateOnly) != key || selected.Weekday() != time.Sunday {
		return Week{}, rejection("Choose a valid week.")
	}
	if key > weekStart(local).Format(time.DateOnly) {
		return Week{}, rejection("Cannot browse beyond the current week.")
	}
	return calendarWeek(local, key), nil
}

// Canonical dates sort chronologically in SQLite and in the rendered calendar.
func validCivilDate(date string) bool {
	parsed, err := time.Parse(time.DateOnly, date)
	return err == nil && parsed.Format(time.DateOnly) == date
}

func localTime(timezone string, now time.Time) (time.Time, error) {
	if timezone == "" || timezone == "Local" {
		return time.Time{}, rejection("Choose a valid timezone.")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, rejection("Choose a valid timezone.")
	}
	return now.In(loc), nil
}

func weekStart(local time.Time) time.Time {
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	return date.AddDate(0, 0, -int(date.Weekday()))
}

func calendarWeek(local time.Time, key string) Week {
	first, _ := time.Parse(time.DateOnly, key)
	last := first.AddDate(0, 0, 6)
	currentKey := weekStart(local).Format(time.DateOnly)
	week := Week{
		Today:    local.Format(time.DateOnly),
		Key:      key,
		Label:    weekLabel(first, last),
		Dates:    make([]string, 0, 7),
		Previous: first.AddDate(0, 0, -7).Format(time.DateOnly),
		Current:  key == currentKey,
	}
	if !week.Current {
		week.Next = first.AddDate(0, 0, 7).Format(time.DateOnly)
	}
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		week.Dates = append(week.Dates, day.Format(time.DateOnly))
	}
	return week
}

func weekLabel(first, last time.Time) string {
	if first.Year() != last.Year() {
		return first.Format("January 2, 2006") + "–" + last.Format("January 2, 2006")
	}
	if first.Month() != last.Month() {
		return first.Format("January 2") + "–" + last.Format("January 2, 2006")
	}
	return first.Format("January 2") + "–" + last.Format("2, 2006")
}
