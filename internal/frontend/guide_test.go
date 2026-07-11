// Render tests pinning the Guide modal's server-rendered structure and its two
// invoker mount points. The step-through interaction is Datastar's own behavior
// and is verified manually (see SPEC-guide-modal.md).
package frontend

import (
	"context"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/frontend/routes"
)

func renderLogin(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	if err := routes.LoginPage("").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render login: %v", err)
	}
	return sb.String()
}

// loginFormElement returns the <form id="login-form">…</form> substring; the
// form nests no inner <form>, so the first </form> closes it.
func loginFormElement(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `id="login-form"`)
	if i < 0 {
		t.Fatalf("body has no #login-form; body:\n%s", body)
	}
	open := strings.LastIndex(body[:i], "<form")
	end := strings.Index(body[i:], "</form>")
	if open < 0 || end < 0 {
		t.Fatalf("could not bound #login-form element; body:\n%s", body)
	}
	return body[open : i+end+len("</form>")]
}

// The app page mounts the dialog once and offers the nav invoker. Pane count,
// copy, and the step-toggle mechanism churn as the guide iterates and are left
// to /verify — this pins only the mount + wiring.
func TestBlocksPageRendersGuideModalAndNavInvoker(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	if n := strings.Count(body, `id="guide-modal"`); n != 1 {
		t.Errorf("want exactly one guide-modal dialog, got %d; body:\n%s", n, body)
	}
	// Nav invoker: opens the modal and closes the mobile drawer.
	for _, want := range []string{
		`commandfor="guide-modal" command="show-modal"`,
		`>Guide</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing nav Guide invoker %q; body:\n%s", want, body)
		}
	}
}

// Pane 3's column is a live demo: guide-demo.js is mounted, exactly one column
// carries the gc-demo hook, and its blocks carry the placement data the script
// reads (data-id/data-slot/data-span) plus a grip handle.
func TestGuideModalDemoColumnWiring(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	if !strings.Contains(body, "guide-demo.js") {
		t.Errorf("page missing the guide-demo.js module; body:\n%s", body)
	}
	if n := strings.Count(body, "gc-demo"); n != 1 {
		t.Errorf("want exactly one gc-demo column, got %d; body:\n%s", n, body)
	}
	for _, want := range []string{
		`data-id="deep-work" data-slot="1" data-span="2"`,
		`data-id="email" data-slot="3" data-span="1"`,
		`data-id="lunch" data-slot="4" data-span="2"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("demo column missing block placement %q; body:\n%s", want, body)
		}
	}
	if n := strings.Count(body, `class="gc-grip"`); n != 3 {
		t.Errorf("want a grip handle on each of the 3 demo blocks, got %d; body:\n%s", n, body)
	}
	if n := strings.Count(body, `class="gc-cue"`); n != 3 {
		t.Errorf("want a resize cue on each of the 3 demo blocks, got %d; body:\n%s", n, body)
	}
}

// The login page mounts the same dialog, an outlined non-submitting invoker, and
// the invoker-command fallback loader. The invoker's label is left to churn; what
// matters is type="button" (it must not submit the email form) and that it opens
// the guide.
func TestLoginPageRendersGuideModalAndWhyButton(t *testing.T) {
	body := renderLogin(t)

	if !strings.Contains(body, `id="guide-modal"`) {
		t.Errorf("login page missing guide-modal; body:\n%s", body)
	}
	// The guide invoker must not submit the email form.
	if !strings.Contains(body, `<button type="button" class="outline-btn" commandfor="guide-modal" command="show-modal">`) {
		t.Errorf("login page missing the outlined type=button guide invoker; body:\n%s", body)
	}
	// DialogInit's fallback loader must be present on login too.
	if !strings.Contains(body, "invoker-fallback.js") {
		t.Errorf("login page missing DialogInit (invoker-fallback.js); body:\n%s", body)
	}
	// The dialog must sit OUTSIDE #login-form so the email→code SSE morph never
	// wipes it. The form may carry the Why? invoker (commandfor), but not the
	// dialog element itself.
	if form := loginFormElement(t, body); strings.Contains(form, `id="guide-modal"`) {
		t.Errorf("guide-modal is inside #login-form — an SSE morph would wipe it; must live outside the patch target; form:\n%s", form)
	}
}
