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
	CurrentCalendar(timezone string) (habit.Month, error)
	MonthSnapshot(ctx context.Context, owner, timezone, month string) (*habit.Snapshot, error)
	Create(ctx context.Context, owner, name, startDate, timezone string) error
	Edit(ctx context.Context, owner string, habitID int64, name, startDate, timezone string) error
	Delete(ctx context.Context, owner string, habitID int64) error
	SetCheckIn(ctx context.Context, owner string, habitID int64, date string, checked bool, timezone string) error
}

type habitSignals struct {
	Name       string `json:"habitname"`
	Start      string `json:"habitstart"`
	CreateView uint64 `json:"habitcreateview"`
	Timezone   string `json:"timezone"`
	Month      string `json:"habitmonth"`
	Refresh    string `json:"habitrefresh"`
	View       uint64 `json:"habitview"`
	HabitID    int64  `json:"habitid"`
	EditID     int64  `json:"habiteditid"`
	EditName   string `json:"habiteditname"`
	EditStart  string `json:"habiteditstart"`
	EditView   uint64 `json:"habiteditview"`
	DeleteID   int64  `json:"habitdeleteid"`
	DeleteView uint64 `json:"habitdeleteview"`
	Date       string `json:"habitdate"`
	Checked    *bool  `json:"habitchecked"`

	CheckInAttempt uint64 `json:"habitcheckinattempt"`
	Read           uint64 `json:"habitread"`
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
		selector := `#habit-grid[data-month="` + snap.Month.Key + `"][data-refresh="` + sig.Refresh + `"][data-view="` + strconv.FormatUint(sig.View, 10) + `"][data-read-request="` + strconv.FormatUint(sig.Read, 10) + `"]`
		if err := sse.PatchElementTempl(components.HabitGridRead(snap.Habits, snap.Month, sig.Refresh, sig.View, sig.Read), datastar.WithSelector(selector)); err != nil {
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
		err := svc.Create(r.Context(), web.OwnerFrom(r.Context()), sig.Name, sig.Start, sig.Timezone)
		if habit.IsRejection(err) {
			sse := datastar.NewSSE(w, r)
			if err := sse.MarshalAndPatchSignals(struct {
				Message string `json:"_habitcreateerror"`
				View    uint64 `json:"_habitcreateerrorview"`
			}{err.Error(), sig.CreateView}); err != nil {
				log.Printf("habit create feedback: %v", err)
			}
			return
		}
		if err != nil {
			log.Printf("habit create: %v", err)
			http.Error(w, "Unable to create habit. Please try again.", http.StatusInternalServerError)
			return
		}
		// Only acknowledge this opening; grid HTML stays on the ordered owner stream.
		sse := datastar.NewSSE(w, r)
		if err := sse.MarshalAndPatchSignals(struct {
			View uint64 `json:"_habitcreatesavedview"`
		}{sig.CreateView}); err != nil {
			log.Printf("habit create acknowledgement: %v", err)
		}
	})
}

func HabitEditHandler(svc HabitService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig habitSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}
		err := svc.Edit(r.Context(), web.OwnerFrom(r.Context()), sig.EditID, sig.EditName, sig.EditStart, sig.Timezone)
		if habit.IsRejection(err) {
			sse := datastar.NewSSE(w, r)
			if err := sse.MarshalAndPatchSignals(struct {
				Message string `json:"_habitediterror"`
				View    uint64 `json:"_habitediterrorview"`
			}{err.Error(), sig.EditView}); err != nil {
				log.Printf("habit edit feedback: %v", err)
			}
			return
		}
		if err != nil {
			log.Printf("habit edit: %v", err)
			http.Error(w, "Unable to edit habit. Please try again.", http.StatusInternalServerError)
			return
		}
		// The private acknowledgement closes only the submitting view's dialog;
		// grid HTML stays on the ordered owner stream.
		sse := datastar.NewSSE(w, r)
		if err := sse.MarshalAndPatchSignals(struct {
			View uint64 `json:"_habiteditsavedview"`
		}{sig.EditView}); err != nil {
			log.Printf("habit edit acknowledgement: %v", err)
		}
	})
}

func HabitDeleteHandler(svc HabitService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig habitSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}
		if err := svc.Delete(r.Context(), web.OwnerFrom(r.Context()), sig.DeleteID); err != nil {
			log.Printf("habit delete: %v", err)
			http.Error(w, "Unable to delete habit. Please try again.", http.StatusInternalServerError)
			return
		}
		// Only acknowledge this confirmation; authoritative grid reads use the owner stream.
		sse := datastar.NewSSE(w, r)
		if err := sse.MarshalAndPatchSignals(struct {
			View uint64 `json:"_habitdeletesavedview"`
		}{sig.DeleteView}); err != nil {
			log.Printf("habit delete acknowledgement: %v", err)
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

// A scalar keeps the attempt/result atomic in Datastar's changed-path signal events.
func patchHabitCheckInReceipt(sse *datastar.ServerSentEventGenerator, attempt uint64, result string) error {
	return sse.MarshalAndPatchSignals(struct {
		Receipt string `json:"_habitcheckinack"`
	}{strconv.FormatUint(attempt, 10) + ":" + result})
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
		err := svc.SetCheckIn(r.Context(), owner, sig.HabitID, sig.Date, *sig.Checked, sig.Timezone)
		if habit.IsRejection(err) {
			sse := datastar.NewSSE(w, r)
			if patchErr := patchHabitCheckInFeedback(sse, err.Error(), "rejected", sig.Date, sig.Refresh, sig.View); patchErr != nil {
				log.Printf("habit check-in feedback: %v", patchErr)
			}
			if patchErr := patchHabitCheckInReceipt(sse, sig.CheckInAttempt, "rejected"); patchErr != nil {
				log.Printf("habit check-in receipt: %v", patchErr)
			}
			return
		}
		if err != nil {
			log.Printf("habit check-in: %v", err)
			http.Error(w, "Unable to save check-in. Please try again.", http.StatusInternalServerError)
			return
		}
		// The private receipt triggers a fenced month read, never a mutation snapshot.
		// A later write may supersede this value before that authoritative read.
		sse := datastar.NewSSE(w, r)
		if err := patchHabitCheckInFeedback(sse, "", "committed", sig.Date, sig.Refresh, sig.View); err != nil {
			log.Printf("habit check-in feedback: %v", err)
		}
		if err := patchHabitCheckInReceipt(sse, sig.CheckInAttempt, "committed"); err != nil {
			log.Printf("habit check-in receipt: %v", err)
		}
	})
}
