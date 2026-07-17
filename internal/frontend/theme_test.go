// Render tests pinning the theme picker's feeling options. The live font/icon
// swap is Datastar mirroring $_feeling into data-feeling and is verified manually.
package frontend

import (
	"strings"
	"testing"
)

// The feeling picker offers "Mono" — the JetBrains-Mono + Octicons feeling —
// which writes the 'mono' token, and the retired "business" token appears nowhere
// in the rendered page (picker, guide preview, or data-feeling bindings).
func TestThemePickerOffersMonoFeeling(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	// templ HTML-escapes the single quotes in the Datastar expressions to &#39;.
	for _, want := range []string{
		`data-on:click="$_feeling = &#39;mono&#39;"`,
		`data-class:active="$_feeling === &#39;mono&#39;"`,
		`Mono</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing Mono feeling %q; body:\n%s", want, body)
		}
	}
	for _, gone := range []string{"business", "Business"} {
		if strings.Contains(body, gone) {
			t.Errorf("page still references retired %q feeling token; body:\n%s", gone, body)
		}
	}
}
