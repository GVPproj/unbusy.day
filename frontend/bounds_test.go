// Bounds-editing UI tests: a native <dialog> (theme-modal house style) whose
// selects cover the hard 5:00–18:00 limits and whose Save posts the chosen
// extent to /cards/bounds as Datastar signals.
package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/frontend/components"
)

// The modal offers every legal half-hour boundary (start 5:00–17:30, end
// 5:30–18:00) and pre-selects the owner's current bounds, so opening it shows
// the day as configured.
func TestBoundsModalOffersLegalRangeWithCurrentSelected(t *testing.T) {
	var b strings.Builder
	if err := components.BoundsModal(testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render bounds modal: %v", err)
	}
	body := b.String()

	if !strings.Contains(body, `id="bounds-modal"`) {
		t.Errorf("missing #bounds-modal dialog anchor; body:\n%s", body)
	}
	// Current bounds (18/34 = 9:00/17:00) come pre-selected.
	for _, want := range []string{
		`<option value="18" selected>9:00</option>`,
		`<option value="34" selected>17:00</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing pre-selected option %q; body:\n%s", want, body)
		}
	}
	// Hard limits: start options span 5:00–17:30 (10–35), end 5:30–18:00 (11–36).
	for _, want := range []string{
		`<option value="10">5:00</option>`,
		`<option value="35">17:30</option>`,
		`<option value="11">5:30</option>`,
		`<option value="36">18:00</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing range option %q; body:\n%s", want, body)
		}
	}
	if got, want := strings.Count(body, "<select"), 2; got != want {
		t.Errorf("want %d selects, got %d; body:\n%s", want, got, body)
	}
}

// The modal's Datastar wiring: $start/$end seed from the current bounds, the
// selects write them back as numbers (select values are strings; BoundsHandler
// decodes ints), and Save @posts /cards/bounds. Keyed attributes must use the
// verified colon form — the dash form is a silent no-op (see the column test).
func TestBoundsModalWiresSignalsToBoundsEndpoint(t *testing.T) {
	var b strings.Builder
	if err := components.BoundsModal(testBounds).Render(context.Background(), &b); err != nil {
		t.Fatalf("render bounds modal: %v", err)
	}
	body := b.String()

	for _, want := range []string{
		`data-signals:start="18"`,
		`data-signals:end="34"`,
		`data-on:change="$start = +evt.target.value"`,
		`data-on:change="$end = +evt.target.value"`,
		`data-on:click="@post('/cards/bounds')"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("modal missing %q; body:\n%s", want, body)
		}
	}
	for _, stale := range []string{`data-signals-start`, `data-signals-end`, `data-on-change`, `data-on-click`} {
		if strings.Contains(body, stale) {
			t.Errorf("modal carries dash-form attribute %q — a silent no-op on Datastar v1.0.2; body:\n%s", stale, body)
		}
	}
}

// GET / hosts the bounds modal seeded with the owner's bounds, plus a
// declarative opener (command/commandfor — no open signal, house style).
func TestPageHostsBoundsModalWithOpener(t *testing.T) {
	svc := &fakeService{cards: threeCards()}

	req := authedRequest(http.MethodGet, "/", "")
	rec := httptest.NewRecorder()
	PageHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="bounds-modal"`,
		`data-signals:start="18"`,
		`commandfor="bounds-modal" command="show-modal"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q; body:\n%s", want, body)
		}
	}
}
