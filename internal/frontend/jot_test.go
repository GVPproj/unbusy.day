package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/jot"
)

// fakeJotService implements JotService in memory, keyed by owner so the
// scoping assertions are real rather than recorded.
type fakeJotService struct {
	texts  map[string]string
	getErr error
	setErr error

	gotOwner string
}

func newFakeJot() *fakeJotService {
	return &fakeJotService{texts: map[string]string{}}
}

func (f *fakeJotService) Get(ctx context.Context, owner string) (string, error) {
	f.gotOwner = owner
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.texts[owner], nil
}

func (f *fakeJotService) Set(ctx context.Context, owner, text string) error {
	f.gotOwner = owner
	if f.setErr != nil {
		return f.setErr
	}
	f.texts[owner] = text
	return nil
}

// 204, no patch: the DOM is already correct, and patching a textarea the user
// is typing into is exactly the thing to avoid.
func TestJotHandlerWritesTheSignalAndAnswers204WithNoBody(t *testing.T) {
	svc := newFakeJot()
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot", `{"_jot":"buy milk\nsee a friend"}`))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("204 must carry no patch; got body:\n%s", body)
	}
	if svc.gotOwner != testOwner {
		t.Errorf("owner: want %q, got %q", testOwner, svc.gotOwner)
	}
	if got := svc.texts[testOwner]; got != "buy milk\nsee a friend" {
		t.Errorf("stored text: got %q", got)
	}
}

// Clearing the Jotpad is a normal write, not an empty-body no-op.
func TestJotHandlerStoresAnEmptyJot(t *testing.T) {
	svc := newFakeJot()
	svc.texts[testOwner] = "old notes"
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot", `{"_jot":""}`))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", rec.Code)
	}
	if got := svc.texts[testOwner]; got != "" {
		t.Errorf("stored text: want empty, got %q", got)
	}
}

// The one place the house "domain rejection → 200 + re-render" convention is
// deliberately not followed: re-rendering would destroy prose the user typed.
// Over-cap is rejected and logged, with no UI.
func TestJotHandlerRejectsAnOverCapWriteWithoutPatchingTheTextarea(t *testing.T) {
	svc := newFakeJot()
	svc.texts[testOwner] = "keep me"
	svc.setErr = jot.ErrTooLong
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot", `{"_jot":"way too long"}`))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: want 204 (no snap-back render), got %d", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "textarea") {
		t.Errorf("a rejected jot write must never patch the textarea; got body:\n%s", body)
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

// The Jotpad is read back on every page load; nothing else re-renders it.
func TestPageRendersTheStoredJotInsideTheTextarea(t *testing.T) {
	jots := newFakeJot()
	jots.texts[testOwner] = "- milk\n- eggs"
	rec := httptest.NewRecorder()

	PageHandler(&fakeService{blocks: threeBlocks()}, jots).
		ServeHTTP(rec, authedRequest(http.MethodGet, "/", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if jots.gotOwner != testOwner {
		t.Errorf("owner: want %q, got %q", testOwner, jots.gotOwner)
	}
	if got := parsedTextareaValue(t, rec.Body.String()); got != "- milk\n- eggs" {
		t.Errorf("textarea value: got %q", got)
	}
}

// An HTML parser drops a newline immediately after the <textarea> start tag,
// so the render emits a sacrificial one; without it a jot beginning with a
// blank line loses it on every reload.
func TestPageKeepsALeadingNewlineInTheStoredJot(t *testing.T) {
	jots := newFakeJot()
	jots.texts[testOwner] = "\n\nafter two blank lines"
	rec := httptest.NewRecorder()

	PageHandler(&fakeService{}, jots).ServeHTTP(rec, authedRequest(http.MethodGet, "/", ""))

	if raw := textareaContent(t, rec.Body.String()); !strings.HasPrefix(raw, "\n") {
		t.Fatalf("render must emit a sacrificial leading newline; got %q", raw)
	}
	if got := parsedTextareaValue(t, rec.Body.String()); got != "\n\nafter two blank lines" {
		t.Errorf("parsed textarea value: got %q", got)
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

// The write path, pinned as markup: the bind, the 1s debounce, the cap, and
// the filterSignals pair that lets one endpoint carry an underscore signal.
func TestPageRendersTheJotWriteWiring(t *testing.T) {
	body := renderPageWithJot(t, "")

	for _, want := range []string{
		`data-bind:_jot`,
		`data-on:input__debounce.1s`,
		`@post('/jot'`,
		`include: /^_jot$/`,
		`exclude: /^$/`,
		`maxlength="100000"`,
		`/static/jot.js`,
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
	jots.texts[testOwner] = text
	rec := httptest.NewRecorder()
	PageHandler(&fakeService{blocks: threeBlocks()}, jots).
		ServeHTTP(rec, authedRequest(http.MethodGet, "/", ""))
	return rec.Body.String()
}

// textareaContent returns the raw (still-escaped-decoded) body of #jot.
func textareaContent(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `id="jot"`)
	if i < 0 {
		t.Fatalf("body has no #jot textarea; body:\n%s", body)
	}
	open := strings.Index(body[i:], ">")
	end := strings.Index(body[i:], "</textarea>")
	if open < 0 || end < 0 {
		t.Fatalf("could not bound the #jot textarea; body:\n%s", body)
	}
	raw := body[i+open+1 : i+end]
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'").Replace(raw)
}

// parsedTextareaValue is what a browser would take as #jot's value: the raw
// content minus the newline an HTML parser drops after the start tag.
func parsedTextareaValue(t *testing.T, body string) string {
	t.Helper()
	return strings.TrimPrefix(textareaContent(t, body), "\n")
}

func TestJotHandler500sOnAStorageFailure(t *testing.T) {
	svc := newFakeJot()
	svc.setErr = context.DeadlineExceeded
	rec := httptest.NewRecorder()

	JotHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/jot", `{"_jot":"x"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", rec.Code)
	}
}
