// Render tests pinning the Guide modal's server-rendered structure and its two
// invoker mount points. The step-through interaction is Datastar's own behavior
// and is verified manually (see SPEC-guide-modal.md).
package components_test

import (
	"context"
	"strconv"
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

// Pane 3's column is a live demo: guide/demo.js is mounted, exactly one column
// carries the gc-demo hook, and its blocks carry the placement data the script
// reads (data-id/data-slot/data-span) plus a grip handle.
func TestGuideModalDemoColumnWiring(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	if !strings.Contains(body, "/static/js/guide/demo.js") {
		t.Errorf("page missing the guide/demo.js module; body:\n%s", body)
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

// The touch carousel (app.css lays .guide-body out as a scroll-snap row) needs
// three things in the markup: guide/swipe.js mounted, the body listening for the
// `guidestep` event the script dispatches on settle, and every pane carrying an
// inert binding — off-screen panes are on screen in the carousel, so inert is
// what keeps them out of the tab order and the a11y tree.
func TestGuideModalSwipeWiring(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	if !strings.Contains(body, "/static/js/guide/swipe.js") {
		t.Errorf("page missing the guide/swipe.js module; body:\n%s", body)
	}
	if !strings.Contains(body, `data-on:guidestep="$_guidestep = evt.detail.step"`) {
		t.Errorf("guide body does not consume the guidestep event; body:\n%s", body)
	}
	for i := 1; i <= 4; i++ {
		want := `data-attr:inert="$_guidestep !== ` + strconv.Itoa(i) + `"`
		if !strings.Contains(body, want) {
			t.Errorf("pane %d missing its inert binding %q; body:\n%s", i, want, body)
		}
	}
}

func TestLoginPageLoadsLandingDemo(t *testing.T) {
	body := renderLogin(t)

	for _, required := range []string{`class="guide-figure guide-column gc-demo"`, "/static/js/guide/demo.js", "/static/js/invoker-fallback.js"} {
		if !strings.Contains(body, required) {
			t.Errorf("login page missing %q", required)
		}
	}
}

func TestLoginPageOmitsGuideModalAndButton(t *testing.T) {
	body := renderLogin(t)

	for _, unwanted := range []string{`id="guide-modal"`, `commandfor="guide-modal"`, "What is it?"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("login page contains %q; body:\n%s", unwanted, body)
		}
	}
}
