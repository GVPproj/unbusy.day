package habit

import (
	"time"
	_ "time/tzdata" // Production's scratch image has no system zoneinfo.
)

type Month struct {
	Today string
	Label string
	Dates []string
}

// Calendar returns civil dates in the browser's IANA timezone, not UTC days.
func Calendar(timezone string, now time.Time) (Month, error) {
	if timezone == "" || timezone == "Local" {
		return Month{}, rejection("Choose a valid timezone.")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Month{}, rejection("Choose a valid timezone.")
	}
	local := now.In(loc)
	m := Month{Today: local.Format(time.DateOnly), Label: local.Format("January 2006"), Dates: make([]string, 0, 31)}
	// Enumerate civil dates in UTC so DST and skipped local midnights cannot shift them.
	first := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC)
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		m.Dates = append(m.Dates, d.Format(time.DateOnly))
	}
	return m, nil
}
