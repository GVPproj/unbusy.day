package components

import "strconv"

// HabitView correlates a rendered grid with the view's refresh and read request.
// Week keys and refresh tokens must be validated before building selectors.
type HabitView struct {
	Refresh string
	View    uint64
	Read    uint64
}

func (v HabitView) GridSelector(week string) string {
	selector := "#habit-grid"
	if week != "" {
		selector += `[data-week="` + week + `"]`
	}
	return selector + `[data-refresh="` + v.Refresh + `"][data-view="` + strconv.FormatUint(v.View, 10) + `"]`
}

func (v HabitView) ReadSelector(week string) string {
	return v.GridSelector(week) + `[data-read-request="` + strconv.FormatUint(v.Read, 10) + `"]`
}
