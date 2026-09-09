package frontend

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/web"
	"github.com/starfederation/datastar-go/datastar"
)

// HabitService keeps calendar-backed habits separate from the undated plan and Jotpad.
type HabitService interface {
	List(context.Context, string) ([]habit.Habit, error)
	Snapshot(ctx context.Context, owner, timezone string) (*habit.Snapshot, error)
	MonthSnapshot(ctx context.Context, owner, timezone, month string) (*habit.Snapshot, error)
	Create(ctx context.Context, owner, name, startDate, timezone string) ([]habit.Habit, error)
	SetCheckIn(ctx context.Context, owner string, habitID int64, date string, checked bool, timezone string) (*habit.Snapshot, error)
}

type habitSignals struct {
	Name     string `json:"habitname"`
	Start    string `json:"habitstart"`
	Timezone string `json:"timezone"`
	Month    string `json:"habitmonth"`
	Refresh  string `json:"habitrefresh"`
	View     uint64 `json:"habitview"`
	HabitID  int64  `json:"habitid"`
	Date     string `json:"habitdate"`
	Checked  *bool  `json:"habitchecked"`
}

func validHabitRefresh(token string) bool {
	if len(token) != 32 {
		return false
	}
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// HabitMonthHandler renders one per-view month. The selector only matches while
// that month is still selected, so a delayed response cannot replace a newer view.
func HabitMonthHandler(svc HabitService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig habitSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals", http.StatusBadRequest)
			return
		}
		if !validHabitRefresh(sig.Refresh) {
			http.Error(w, "invalid refresh token", http.StatusBadRequest)
			return
		}
		owner := web.OwnerFrom(r.Context())
		snap, err := svc.MonthSnapshot(r.Context(), owner, sig.Timezone, sig.Month)
		if habit.IsRejection(err) {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err != nil {
			log.Printf("habit month: %v", err)
			http.Error(w, "Unable to load habits. Please try again.", http.StatusInternalServerError)
			return
		}
		sse := datastar.NewSSE(w, r)
		selector := `#habit-grid[data-month="` + snap.Month.Key + `"][data-refresh="` + sig.Refresh + `"][data-view="` + strconv.FormatUint(sig.View, 10) + `"]`
		if err := sse.PatchElementTempl(components.HabitGrid(snap.Habits, snap.Month, sig.Refresh, sig.View), datastar.WithSelector(selector)); err != nil {
			log.Printf("habit month: %v", err)
		}
	})
}

func HabitCreateHandler(svc HabitService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig habitSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}
		_, err := svc.Create(r.Context(), web.OwnerFrom(r.Context()), sig.Name, sig.Start, sig.Timezone)
		if habit.IsRejection(err) {
			sse := datastar.NewSSE(w, r)
			if err := sse.PatchElementTempl(components.HabitFeedback(err.Error())); err != nil {
				log.Printf("habit feedback: %v", err)
			}
			return
		}
		if err != nil {
			log.Printf("habit create: %v", err)
			http.Error(w, "Unable to create habit. Please try again.", http.StatusInternalServerError)
			return
		}
		// The owner stream is the sole ordered path for habit-grid HTML.
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.HabitFeedback("Habit created.")); err != nil {
			log.Printf("habit feedback: %v", err)
		}
	})
}

func patchHabitCheckInFeedback(sse *datastar.ServerSentEventGenerator, message, result, date, refresh string, view uint64) error {
	opts := make([]datastar.PatchElementOption, 0, 1)
	if validHabitRefresh(refresh) {
		month := ""
		if parsed, err := time.Parse(time.DateOnly, date); err == nil && parsed.Format(time.DateOnly) == date {
			month = `[data-month="` + date[:7] + `"]`
		}
		opts = append(opts, datastar.WithSelector(`#habit-grid`+month+`[data-refresh="`+refresh+`"][data-view="`+strconv.FormatUint(view, 10)+`"] #habit-checkin-feedback`))
	} else if refresh != "" {
		return nil
	}
	return sse.PatchElementTempl(components.HabitCheckInFeedback(message, result), opts...)
}

func HabitCheckInHandler(svc HabitService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig habitSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}
		if sig.Checked == nil {
			http.Error(w, "habitchecked is required", http.StatusBadRequest)
			return
		}
		owner := web.OwnerFrom(r.Context())
		_, err := svc.SetCheckIn(r.Context(), owner, sig.HabitID, sig.Date, *sig.Checked, sig.Timezone)
		if habit.IsRejection(err) {
			sse := datastar.NewSSE(w, r)
			if patchErr := patchHabitCheckInFeedback(sse, err.Error(), "rejected", sig.Date, sig.Refresh, sig.View); patchErr != nil {
				log.Printf("habit check-in feedback: %v", patchErr)
			}
			return
		}
		if err != nil {
			log.Printf("habit check-in: %v", err)
			http.Error(w, "Unable to save check-in. Please try again.", http.StatusInternalServerError)
			return
		}
		// The owner stream serializes committed snapshots; keeping grid HTML off
		// this response prevents a delayed mutation response overwriting a newer write.
		sse := datastar.NewSSE(w, r)
		if err := patchHabitCheckInFeedback(sse, "Saved.", "committed", sig.Date, sig.Refresh, sig.View); err != nil {
			log.Printf("habit check-in feedback: %v", err)
		}
	})
}
