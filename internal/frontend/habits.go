package frontend

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/web"
	"github.com/starfederation/datastar-go/datastar"
)

// HabitService keeps calendar-backed habits separate from the undated plan and Jotpad.
type HabitService interface {
	List(context.Context, string) ([]habit.Habit, error)
	Create(ctx context.Context, owner, name, startDate, timezone string) ([]habit.Habit, error)
}

type habitSignals struct {
	Name     string `json:"habitname"`
	Start    string `json:"habitstart"`
	Timezone string `json:"timezone"`
}

func HabitCreateHandler(svc HabitService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig habitSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}
		habits, err := svc.Create(r.Context(), web.OwnerFrom(r.Context()), sig.Name, sig.Start, sig.Timezone)
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
		month, err := habit.Calendar(sig.Timezone, time.Now())
		if err != nil {
			http.Error(w, "invalid timezone", http.StatusBadRequest)
			return
		}
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.Habits(habits, month)); err != nil {
			log.Printf("habit patch: %v", err)
			return
		}
		if err := sse.PatchElementTempl(components.HabitFeedback("Habit created.")); err != nil {
			log.Printf("habit feedback: %v", err)
		}
	})
}

func patchHabits(sse *datastar.ServerSentEventGenerator, r *http.Request, svc HabitService, timezone string) error {
	habits, err := svc.List(r.Context(), web.OwnerFrom(r.Context()))
	if err != nil {
		return err
	}
	month, err := habit.Calendar(timezone, time.Now())
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(components.Habits(habits, month))
}
