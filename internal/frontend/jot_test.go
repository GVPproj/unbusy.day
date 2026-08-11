package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/jot"
)

// fakeJotService implements JotService in memory, keyed by owner so the
// scoping assertions are real rather than recorded. Set mimics the real
// service's contract: CAS is invisible here (no merge), but over-cap and
// storage failures return the authoritative pad alongside the error.
type fakeJotService struct {
	pads   map[string]jot.Pad
	getErr error
	setErr error

	gotOwner string
	gotBase  int64
}

func newFakeJot() *fakeJotService {
	return &fakeJotService{pads: map[string]jot.Pad{}}
}

func (f *fakeJotService) Get(ctx context.Context, owner string) (jot.Pad, error) {
	f.gotOwner = owner
	if f.getErr != nil {
		return jot.Pad{}, f.getErr
	}
	return f.pads[owner], nil
}

func (f *fakeJotService) Set(ctx context.Context, owner, text string, base int64) (jot.Pad, error) {
	f.gotOwner = owner
	f.gotBase = base
	if f.setErr != nil {
		return f.pads[owner], f.setErr
	}
	p := jot.Pad{Text: text, Version: f.pads[owner].Version + 1}
	f.pads[owner] = p
	return p, nil
}

func decodeJotResponse(t *testing.T, rec *httptest.ResponseRecorder) (int64, *string) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type: want application/json, got %q", ct)
	}
	var resp struct {
		Version int64   `json:"version"`
		Text    *string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return resp.Version, resp.Text
}

// The write path answers plain JSON, never an element patch: the committed
// version, and no text when the store took exactly what the client sent.
func TestJotHandlerStoresAndAnswersTheCommittedVersion(t *testing.T) {
	svc := newFakeJot()
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot",
		`{"_jot":"buy milk\nsee a friend","_jotVersion":0}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	version, text := decodeJotResponse(t, rec)
	if version != 1 {
		t.Errorf("version: want 1, got %d", version)
	}
	if text != nil {
		t.Errorf("text must be omitted when the write was taken verbatim; got %q", *text)
	}
	if svc.gotOwner != testOwner || svc.gotBase != 0 {
		t.Errorf("owner/base: want %q/0, got %q/%d", testOwner, svc.gotOwner, svc.gotBase)
	}
	if got := svc.pads[testOwner].Text; got != "buy milk\nsee a friend" {
		t.Errorf("stored text: got %q", got)
	}
}

// Clearing the Jotpad is a normal write, not an empty-body no-op.
func TestJotHandlerStoresAnEmptyJot(t *testing.T) {
	svc := newFakeJot()
	svc.pads[testOwner] = jot.Pad{Text: "old notes", Version: 1}
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot", `{"_jot":"","_jotVersion":1}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if got := svc.pads[testOwner].Text; got != "" {
		t.Errorf("stored text: want empty, got %q", got)
	}
}

// When the service stores something other than what the client sent (a merge),
// the response carries the authoritative text so the client snaps to it.
func TestJotHandlerReturnsTheMergedTextWhenTheStoreDiffers(t *testing.T) {
	svc := &mergingJot{merged: jot.Pad{Text: "merged result", Version: 5}}
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot",
		`{"_jot":"stale text","_jotVersion":2}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	version, text := decodeJotResponse(t, rec)
	if version != 5 {
		t.Errorf("version: want 5, got %d", version)
	}
	if text == nil || *text != "merged result" {
		t.Errorf("merged response must carry the authoritative text; got %v", text)
	}
}

type mergingJot struct{ merged jot.Pad }

func (m *mergingJot) Get(ctx context.Context, owner string) (jot.Pad, error) { return m.merged, nil }
func (m *mergingJot) Set(ctx context.Context, owner, text string, base int64) (jot.Pad, error) {
	return m.merged, nil
}

// Over-cap keeps its log-and-drop behavior, but the response now carries the
// authoritative state — the client must stop believing its over-cap doc is
// saved (the indicator would otherwise lie). Still 200, still no element patch.
func TestJotHandlerAnswersTheAuthoritativeStateForAnOverCapWrite(t *testing.T) {
	svc := newFakeJot()
	svc.pads[testOwner] = jot.Pad{Text: "keep me", Version: 3}
	svc.setErr = jot.ErrTooLong
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot",
		`{"_jot":"way too long","_jotVersion":3}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	version, text := decodeJotResponse(t, rec)
	if version != 3 {
		t.Errorf("version: want the untouched 3, got %d", version)
	}
	if text == nil || *text != "keep me" {
		t.Errorf("rejection must return the authoritative text; got %v", text)
	}
}

func TestJotHandlerRejectsAMalformedSignalsBody(t *testing.T) {
	svc := newFakeJot()
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot", `not json`))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", rec.Code)
	}
}

// The page load renders the stored jot; live updates ride SSE signal patches.
// The text travels as a JSON script (never element text), so whitespace-hostile
// content — leading blank lines, HTML-ish fragments — survives every reload.
func TestPageRendersTheStoredJotInTheEditorPayload(t *testing.T) {
	jots := newFakeJot()
	jots.pads[testOwner] = jot.Pad{Text: "\n\n- milk & eggs <b>", Version: 4}
	rec := httptest.NewRecorder()

	PageHandler(&fakeService{blocks: threeBlocks()}, jots).
		ServeHTTP(rec, authedRequest(http.MethodGet, "/", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if jots.gotOwner != testOwner {
		t.Errorf("owner: want %q, got %q", testOwner, jots.gotOwner)
	}
	if got := jotPayload(t, rec.Body.String()); got != "\n\n- milk & eggs <b>" {
		t.Errorf("jot payload: got %q", got)
	}
	// The version the client's first CAS write must carry.
	if !strings.Contains(rec.Body.String(), `data-jot-version="4"`) {
		t.Errorf("body missing data-jot-version; body:\n%s", rec.Body.String())
	}
}

// Two panels cannot both be <main>. The Day Plan keeps <main> and the page's
// <h1>; the Jotpad is a named <aside> landmark with an <h2>.
func TestPageRendersTheJotpadAsANamedAsideLandmark(t *testing.T) {
	body := renderPageWithJot(t, "")

	if n := strings.Count(body, "<main"); n != 1 {
		t.Errorf("want exactly one <main>, got %d; body:\n%s", n, body)
	}
	for _, want := range []string{
		`<aside`,
		`class="column jotpad"`,
		`id="jot-heading"`,
		`aria-labelledby="jot-heading"`,
		`<h2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<h1") && strings.Index(body, "<h1") > strings.Index(body, "<aside") {
		t.Errorf("the <h1> must stay in the Day Plan panel, ahead of the Jotpad")
	}
}

// The write path, pinned as markup: jot-cm.js owns saving (Datastar's @post
// can't read /jot's JSON response), the version and length cap ride data
// attributes, and the save-state indicator is a live region next to the heading.
func TestPageRendersTheJotWriteWiring(t *testing.T) {
	body := renderPageWithJot(t, "")

	for _, want := range []string{
		`data-jot-version="0"`,
		`data-maxlen="100000"`,
		`/static/jot-cm.js`,
		`id="jot-status"`,
		`data-state="saved"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body:\n%s", want, body)
		}
	}
	// The old Datastar save path must be gone: it can't read the JSON reply.
	for _, gone := range []string{`data-bind:_jot`, `@post('/jot'`} {
		if strings.Contains(body, gone) {
			t.Errorf("body still carries the retired Datastar save wiring %q", gone)
		}
	}
}

// Remote jot state arrives as _jotv/_jott signal patches on /events; the shell
// hands them to the sync driver via data-on-signal-patch. Never element patches.
func TestPageRendersTheJotRemoteListener(t *testing.T) {
	body := renderPageWithJot(t, "")

	for _, want := range []string{
		`data-signals:_jotv="0"`,
		`data-on-signal-patch="window.__jotRemote && window.__jotRemote($_jotv, $_jott)"`,
		`data-on-signal-patch-filter="{include: /^_jotv$/}"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body:\n%s", want, body)
		}
	}
}

// Panel visibility is client-only state: $_jotopen never reaches the server,
// and data-class (not data-show) keeps the toggle inert above 52rem.
func TestPageTogglesPanelsWithAClientOnlySignal(t *testing.T) {
	body := renderPageWithJot(t, "")

	for _, want := range []string{
		`data-signals:_jotopen="false"`,
		`data-class:jot-open="$_jotopen"`,
		`class="panels"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body:\n%s", want, body)
		}
	}
	// data-show sets display unconditionally, which would hide a panel above
	// 52rem too; the panels must be switched by a class the media query owns.
	for _, tag := range []string{openTag(t, body, `class="panels"`), openTag(t, body, `class="column jotpad"`)} {
		if strings.Contains(tag, "data-show") {
			t.Errorf("panel must not use data-show; got tag: %s", tag)
		}
	}
}

// openTag returns the element start tag containing marker.
func openTag(t *testing.T, body, marker string) string {
	t.Helper()
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("body has no element matching %q; body:\n%s", marker, body)
	}
	open := strings.LastIndex(body[:i], "<")
	end := strings.Index(body[i:], ">")
	if open < 0 || end < 0 {
		t.Fatalf("could not bound the tag for %q", marker)
	}
	return body[open : i+end+1]
}

func TestSideNavCarriesTheJotpadToggle(t *testing.T) {
	body := renderPageWithJot(t, "")

	for _, want := range []string{"View Jotpad", "View Plan", `$_jotopen = !$_jotopen`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body:\n%s", want, body)
		}
	}
}

// The guide is the app's narrative onboarding; without a mention there, the
// second panel is unexplained.
func TestGuideModalIntroducesTheJotpad(t *testing.T) {
	if body := renderPageWithJot(t, ""); !strings.Contains(body, "Jotpad") ||
		!strings.Contains(body, "scratchpad for everything that isn't a block") {
		t.Errorf("guide modal must introduce the Jotpad; body:\n%s", body)
	}
}

func renderPageWithJot(t *testing.T, text string) string {
	t.Helper()
	jots := newFakeJot()
	jots.pads[testOwner] = jot.Pad{Text: text}
	rec := httptest.NewRecorder()
	PageHandler(&fakeService{blocks: threeBlocks()}, jots).
		ServeHTTP(rec, authedRequest(http.MethodGet, "/", ""))
	return rec.Body.String()
}

// jotPayload decodes the #jot-cm-text JSON script — the text exactly as
// jot-cm.js will hand it to the editor.
func jotPayload(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `id="jot-cm-text"`)
	if i < 0 {
		t.Fatalf("body has no #jot-cm-text script; body:\n%s", body)
	}
	open := strings.Index(body[i:], ">")
	end := strings.Index(body[i:], "</script>")
	if open < 0 || end < 0 {
		t.Fatalf("could not bound the #jot-cm-text script; body:\n%s", body)
	}
	var text string
	if err := json.Unmarshal([]byte(body[i+open+1:i+end]), &text); err != nil {
		t.Fatalf("decode jot payload: %v", err)
	}
	return text
}

func TestJotHandler500sOnAStorageFailure(t *testing.T) {
	svc := newFakeJot()
	svc.setErr = context.DeadlineExceeded
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot", `{"_jot":"x","_jotVersion":0}`))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", rec.Code)
	}
}
