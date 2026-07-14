// Render tests pinning the server-rendered wiring for the keyboard accelerators
// and the shortcuts reference (UNB-25). The keyboard glue itself (block-gestures.js,
// the global `?` handler) is DOM/native-dialog behaviour verified manually; these
// assert the static attributes and accessible text a user or AT depends on.
package frontend

import (
	"context"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/frontend/components/modals"
)

// The New Block dialog submits on Cmd/Ctrl-Enter by triggering the one existing
// Create control (id=create-submit) — no second submission path. The handler
// sits on the dialog so it fires from the name field or a type option alike.
func TestCreateModalWiresCmdEnterToSubmit(t *testing.T) {
	var b strings.Builder
	if err := modals.CreateModal().Render(context.Background(), &b); err != nil {
		t.Fatalf("render create modal: %v", err)
	}
	body := b.String()

	if !strings.Contains(body, `id="create-submit"`) {
		t.Errorf("Create button missing id=create-submit (the Cmd/Ctrl-Enter target); body:\n%s", body)
	}
	for _, want := range []string{
		`data-on:keydown="`,
		`evt.metaKey || evt.ctrlKey`,
		`evt.key === 'Enter'`,
		`document.getElementById('create-submit').click()`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("create dialog missing Cmd/Ctrl-Enter wiring %q; body:\n%s", want, body)
		}
	}
	// Keyed colon form only — the dash form is a silent no-op on Datastar v1.0.2.
	if strings.Contains(body, "data-on-keydown") {
		t.Errorf("create dialog uses dash-form data-on-keydown — a silent no-op; body:\n%s", body)
	}
}

// The block-list carries the delete CustomEvent seam, wired the same way as its
// layout/rename events, so keyboard delete reaches the existing endpoint.
func TestBlockListWiresDeleteEvent(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)
	list := blockListElement(t, body)

	if !strings.Contains(list, `data-on:delete="$deleteid = evt.detail.id; @post('/blocks/delete')"`) {
		t.Errorf("#block-list missing data-on:delete wiring; list:\n%s", list)
	}
}

// The shortcuts reference is mounted once on the app page, opened by the sidenav
// invoker and the global `?` module, listing the whole scheme grouped by focus
// context.
func TestPageRendersShortcutsModalAndInvokers(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	if n := strings.Count(body, `id="shortcuts-modal"`); n != 1 {
		t.Errorf("want exactly one shortcuts-modal dialog, got %d; body:\n%s", n, body)
	}
	// Native light-dismiss dialog, like the other modals.
	if !strings.Contains(body, `id="shortcuts-modal"`) || !strings.Contains(body, `class="app-dialog shortcuts-dialog"`) {
		t.Errorf("shortcuts modal isn't the app-dialog native-dialog exemplar; body:\n%s", body)
	}
	// Sidenav invoker (opens the modal and closes the mobile drawer) + the `?`
	// module loaded on the app page.
	for _, want := range []string{
		`commandfor="shortcuts-modal" command="show-modal"`,
		`>Shortcuts</span>`,
		"/static/shortcuts.js",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing shortcuts wiring %q; body:\n%s", want, body)
		}
	}
	// The reference lists each shortcut with a description; spot-check the tiers.
	for _, want := range []string{
		"Keyboard Shortcuts",
		"On a focused block",
		"On a resize grip",
		"<kbd>Enter</kbd>",
		"<kbd>j</kbd>",
		"<kbd>Delete</kbd>",
		"<kbd>?</kbd>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shortcuts modal missing %q; body:\n%s", want, body)
		}
	}
}

// The block's accessible instructions must mention the new keys so AT users
// hear an accurate description (WCAG: the aria-describedby target stays true).
func TestDndInstructionsMentionNewKeys(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	i := strings.Index(body, `id="dnd-instructions"`)
	if i < 0 {
		t.Fatalf("page has no #dnd-instructions; body:\n%s", body)
	}
	instr := body[i:]
	if end := strings.Index(instr, "</div>"); end >= 0 {
		instr = instr[:end]
	}
	for _, want := range []string{"j and k", "rename", "Delete", "question mark"} {
		if !strings.Contains(instr, want) {
			t.Errorf("#dnd-instructions missing %q; text:\n%s", want, instr)
		}
	}
}
