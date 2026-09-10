package habit

import (
	"time"
	_ "time/tzdata" // Production's scratch image has no system zoneinfo.
)

type Month struct {
	Today    string
	Key      string
	Label    string
	Dates    []string
	Previous string
	Next     string
	Current  bool
}

// Calendar returns the current civil month in the browser's IANA timezone.
func Calendar(timezone string, now time.Time) (Month, error) {
	local, err := localTime(timezone, now)
	if err != nil {
		return Month{}, err
	}
	return calendarMonth(local, local.Format("2006-01")), nil
}

// CalendarMonth returns a selected month no later than the browser's local month.
func CalendarMonth(timezone, key string, now time.Time) (Month, error) {
	local, err := localTime(timezone, now)
	if err != nil {
		return Month{}, err
	}
	selected, err := time.Parse("2006-01", key)
	if err != nil || selected.Format("2006-01") != key {
		return Month{}, rejection("Choose a valid month.")
	}
	if key > local.Format("2006-01") {
		return Month{}, rejection("Cannot browse beyond the current month.")
	}
	return calendarMonth(local, key), nil
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

func calendarMonth(local time.Time, key string) Month {
	first, _ := time.Parse("2006-01", key)
	currentKey := local.Format("2006-01")
	m := Month{
		Today:    local.Format(time.DateOnly),
		Key:      key,
		Label:    first.Format("January 2006"),
		Dates:    make([]string, 0, 31),
		Previous: first.AddDate(0, -1, 0).Format("2006-01"),
		Current:  key == currentKey,
	}
	if !m.Current {
		m.Next = first.AddDate(0, 1, 0).Format("2006-01")
	}
	// Enumerate in UTC so DST and skipped local midnights cannot shift civil dates.
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		m.Dates = append(m.Dates, d.Format(time.DateOnly))
	}
	return m
}
