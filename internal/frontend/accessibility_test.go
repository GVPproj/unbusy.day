// Render tests pinning the server-rendered accessibility semantics; the
// keyboard glue itself (drag.js) is verified manually.
package frontend

import (
	"context"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/frontend/routes"
)

func renderPage(t *testing.T, cs []block.Block, b block.Bounds) string {
	t.Helper()
	var sb strings.Builder
	if err := routes.BlocksPage(cs, b).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render page: %v", err)
	}
	return sb.String()
}

// blockListElement returns the <ul id="block-list">…</ul> substring; it nests
// no inner <ul>, so the first </ul> closes it.
func blockListElement(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `id="block-list"`)
	if i < 0 {
		t.Fatalf("body has no #block-list; body:\n%s", body)
	}
	open := strings.LastIndex(body[:i], "<ul")
	end := strings.Index(body[i:], "</ul>")
	if open < 0 || end < 0 {
		t.Fatalf("could not bound #block-list element; body:\n%s", body)
	}
	return body[open : i+end+len("</ul>")]
}

// The live-region and instructions nodes must live OUTSIDE #block-list so SSE
// morphs leave them intact (queued announcer text, aria-describedby targets).
func TestPageRendersLiveRegionAndInstructionsOutsidePatchTarget(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)
	list := blockListElement(t, body)

	for _, want := range []string{`id="sr-announce"`, `aria-live="assertive"`, `id="dnd-instructions"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q; body:\n%s", want, body)
		}
		if strings.Contains(list, want) {
			t.Errorf("%q is inside #block-list — it would be wiped on every SSE morph; must live outside the patch target", want)
		}
	}
}

// blockOpenTag returns the opening <li …> tag of the block with the given data-id.
func blockOpenTag(t *testing.T, body, id string) string {
	t.Helper()
	marker := `data-id="` + id + `"`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("body has no block %s; body:\n%s", id, body)
	}
	open := strings.LastIndex(body[:i], "<li")
	close := strings.Index(body[i:], ">")
	if open < 0 || close < 0 {
		t.Fatalf("could not bound block %s open tag; body:\n%s", id, body)
	}
	return body[open : i+close+1]
}

// gripOpenTag returns the opening <span …> tag of the (single) resize grip.
func gripOpenTag(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `class="grip`)
	if i < 0 {
		t.Fatalf("body has no grip; body:\n%s", body)
	}
	open := strings.LastIndex(body[:i], "<span")
	close := strings.Index(body[i:], ">")
	if open < 0 || close < 0 {
		t.Fatalf("could not bound grip open tag; body:\n%s", body)
	}
	return body[open : i+close+1]
}

// The grip is an APG Window Splitter: a focusable role="separator" reporting
// span via aria-value*; it must not be aria-hidden, and aria-controls must resolve.
func TestGripIsResizeSeparator(t *testing.T) {
	cs := []block.Block{{ID: "a", Label: "Deep Work", Position: 20, Span: 2}}
	var sb strings.Builder
	if err := components.BlockColumn(cs, testBounds).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render column: %v", err)
	}
	body := sb.String()

	if tag := blockOpenTag(t, body, "a"); !strings.Contains(tag, `id="a"`) {
		t.Errorf("block <li> missing id=\"a\" (aria-controls target); open tag:\n%s", tag)
	}

	grip := gripOpenTag(t, body)
	for _, want := range []string{
		`role="separator"`,
		`tabindex="0"`,
		`aria-controls="a"`,
		`aria-label="Resize Deep Work"`,
		`aria-valuemin="1"`,
		`aria-valuemax="14"`, // day end (34) − block start (20)
		`aria-valuenow="2"`,
		`aria-valuetext="10:00 to 11:00"`,
	} {
		if !strings.Contains(grip, want) {
			t.Errorf("grip missing %q; open tag:\n%s", want, grip)
		}
	}
	if strings.Contains(grip, `aria-hidden="true"`) {
		t.Errorf("grip is still aria-hidden — it must be reachable as a separator; open tag:\n%s", grip)
	}
}

func TestBlockIsFocusableAndDescribed(t *testing.T) {
	var sb strings.Builder
	if err := components.BlockColumn(threeBlocks(), testBounds).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render column: %v", err)
	}
	tag := blockOpenTag(t, sb.String(), "a")
	for _, want := range []string{
		`tabindex="0"`,
		`aria-roledescription="schedule block"`,
		`aria-describedby="dnd-instructions"`,
	} {
		if !strings.Contains(tag, want) {
			t.Errorf("block <li> missing %q; open tag:\n%s", want, tag)
		}
	}
}
