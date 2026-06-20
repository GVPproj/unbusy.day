// Bounds-editing UI tests: a native <dialog> (theme-modal house style) whose
// selects cover the hard 5:00–18:00 limits and whose Save posts the chosen
// extent to /blocks/bounds as Datastar signals. Options outside the occupied
// envelope disable reactively so an impossible window is never offered.
package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/frontend/components/modals"
)

// testEnvelope is the occupied extent of threeBlocks() (18,19,20 span-1):
// first occupied slot 18, last occupied end 21.
var testEnvelope = block.Envelope{FirstSlot: 18, LastEnd: 21}

// The modal offers every legal half-hour boundary (start 5:00–17:30, end
// 5:30–18:00) and pre-selects the owner's current bounds, so opening it shows
// the day as configured.
func TestHoursModalOffersLegalRangeWithCurrentSelected(t *testing.T) {
	var b strings.Builder
	if err := modals.HoursModal(testBounds, testEnvelope).Render(context.Background(), &b); err != nil {
		t.Fatalf("render bounds modal: %v", err)
	}
	body := b.String()

	if !strings.Contains(body, `id="bounds-modal"`) {
		t.Errorf("missing #bounds-modal dialog anchor; body:\n%s", body)
	}
	// Current bounds (18/34 = 9:00/17:00) come pre-selected.
	for _, want := range []string{
		`<option value="18" selected`,
		`<option value="34" selected`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing pre-selected option %q; body:\n%s", want, body)
		}
	}
	// Hard limits: start options span 5:00–17:30 (10–35), end 5:30–18:00 (11–36).
	for _, want := range []string{
		`<option value="10"`, `5:00</option>`,
		`<option value="35"`, `17:30</option>`,
		`<option value="11"`, `5:30</option>`,
		`<option value="36"`, `18:00</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing range marker %q; body:\n%s", want, body)
		}
	}
	if got, want := strings.Count(body, "<select"), 2; got != want {
		t.Errorf("want %d selects, got %d; body:\n%s", want, got, body)
	}
}

// Each option's disabled state is bound reactively to the live envelope
// signals: a start slot past firstOccupiedSlot disables (raising start there
// would strand the earliest block), an end slot below lastOccupiedEnd disables.
// The bindings reference the signals, not baked-in constants, so the modal
// rendered once in the shell stays correct as blocks change.
func TestHoursModalBindsDisabledToEnvelopeSignals(t *testing.T) {
	var b strings.Builder
	if err := modals.HoursModal(testBounds, testEnvelope).Render(context.Background(), &b); err != nil {
		t.Fatalf("render bounds modal: %v", err)
	}
	body := b.String()

	// Envelope seeded as an object-literal signal (the keyed colon form can't
	// carry camelCase names — the browser lowercases attribute names), so the
	// disabled-bindings' $firstOccupiedSlot/$lastOccupiedEnd resolve. >/< escape
	// to &gt;/&lt; in the attribute value.
	for _, want := range []string{
		`firstOccupiedSlot: 18`,
		`lastOccupiedEnd: 21`,
		`<option value="20" data-attr:disabled="20 &gt; $firstOccupiedSlot"`,
		`<option value="20" data-attr:disabled="20 &lt; $lastOccupiedEnd"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("modal missing %q; body:\n%s", want, body)
		}
	}
}

// With no blocks the envelope collapses to its sentinels, so every option's
// binding evaluates false and the full 5:00–18:00 range stays pickable.
func TestHoursModalEmptyDayOffersFullRange(t *testing.T) {
	var b strings.Builder
	if err := modals.HoursModal(testBounds, block.OccupiedEnvelope(nil)).Render(context.Background(), &b); err != nil {
		t.Fatalf("render bounds modal: %v", err)
	}
	body := b.String()

	for _, want := range []string{
		`firstOccupiedSlot: 36`, // MaxDayEnd: no start slot exceeds it
		`lastOccupiedEnd: 8`,    // MinDayStart: no end slot is below it
	} {
		if !strings.Contains(body, want) {
			t.Errorf("empty-day modal missing %q; body:\n%s", want, body)
		}
	}
}

// The modal's Datastar wiring: $start/$end seed from the current bounds, the
// selects write them back as numbers (select values are strings; BoundsHandler
// decodes ints), and Save @posts /blocks/bounds. Keyed attributes must use the
// verified colon form — the dash form is a silent no-op (see the column test).
func TestHoursModalWiresSignalsToBoundsEndpoint(t *testing.T) {
	var b strings.Builder
	if err := modals.HoursModal(testBounds, testEnvelope).Render(context.Background(), &b); err != nil {
		t.Fatalf("render bounds modal: %v", err)
	}
	body := b.String()

	for _, want := range []string{
		`start: 18`,
		`end: 34`,
		`data-on:change="$start = +evt.target.value"`,
		`data-on:change="$end = +evt.target.value"`,
		`data-on:click="@post('/blocks/bounds')"`,
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

// GET / hosts the bounds modal seeded with the owner's bounds and occupied
// envelope, plus a declarative opener (command/commandfor — no open signal,
// house style).
func TestPageHostsHoursModalWithOpener(t *testing.T) {
	svc := &fakeService{blocks: threeBlocks()}

	req := authedRequest(http.MethodGet, "/", "")
	rec := httptest.NewRecorder()
	PageHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="bounds-modal"`,
		`start: 18`,
		`firstOccupiedSlot: 18`,
		`lastOccupiedEnd: 21`,
		`commandfor="bounds-modal" command="show-modal"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q; body:\n%s", want, body)
		}
	}
}
