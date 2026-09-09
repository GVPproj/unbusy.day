package frontend

import (
	"context"
	"log"
	"net/http"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/web"
	"github.com/starfederation/datastar-go/datastar"
)

// HabitService keeps calendar-backed habits separate from the undated plan and Jotpad.
type HabitService interface {
	List(context.Context, string) ([]habit.Habit, error)
	Snapshot(ctx context.Context, owner, timezone string) (*habit.Snapshot, error)
	Create(ctx context.Context, owner, name, startDate, timezone string) ([]habit.Habit, error)
	SetCheckIn(ctx context.Context, owner string, habitID int64, date string, checked bool, timezone string) (*habit.Snapshot, error)
}

type habitSignals struct {
	Name     string `json:"habitname"`
	Start    string `json:"habitstart"`
	Timezone string `json:"timezone"`
	HabitID  int64  `json:"habitid"`
	Date     string `json:"habitdate"`
	Checked  *bool  `json:"habitchecked"`
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
		if _, err := svc.Snapshot(r.Context(), owner, sig.Timezone); err != nil {
			if habit.IsRejection(err) {
				sse := datastar.NewSSE(w, r)
				if patchErr := sse.PatchElementTempl(components.HabitCheckInFeedback(err.Error(), "rejected")); patchErr != nil {
					log.Printf("habit check-in feedback: %v", patchErr)
				}
			} else {
				log.Printf("habit check-in preflight: %v", err)
				http.Error(w, "Unable to save check-in. Please try again.", http.StatusInternalServerError)
			}
			return
		}
		_, err := svc.SetCheckIn(r.Context(), owner, sig.HabitID, sig.Date, *sig.Checked, sig.Timezone)
		if habit.IsRejection(err) {
			sse := datastar.NewSSE(w, r)
			if patchErr := sse.PatchElementTempl(components.HabitCheckInFeedback(err.Error(), "rejected")); patchErr != nil {
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
		if err := sse.PatchElementTempl(components.HabitCheckInFeedback("Saved.", "committed")); err != nil {
			log.Printf("habit check-in feedback: %v", err)
		}
	})
}

func patchHabits(sse *datastar.ServerSentEventGenerator, r *http.Request, svc HabitService, timezone string) error {
	snap, err := svc.Snapshot(r.Context(), web.OwnerFrom(r.Context()), timezone)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(components.HabitGrid(snap.Habits, snap.Month))
}
