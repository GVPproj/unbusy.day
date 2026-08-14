// Render tests pinning the theme picker's feeling options and the colourscheme
// family + Light/Dark mode axis. The live font/icon swap and the live colormode
// swap are Datastar mirroring signals into data-* attributes and are verified
// manually.
package components_test

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

// colourscheme is now a family axis; Solarized is one family whose Light/Dark
// variant is chosen by the modeOption toggle. The layout <html> mirrors
// $_colormode into data-colormode and persists it alongside the family, and the
// pre-paint script sets data-colormode before first paint so a dark choice
// doesn't flash the light tokens until Datastar hydrates.
func TestLayoutMirrorsColormodeSignal(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	for _, want := range []string{
		`data-signals:_colormode=`,
		`data-attr:data-colormode="$_colormode"`,
		`localStorage.setItem('colormode'`,
		`localStorage.getItem("colormode")`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing colormode binding %q; body:\n%s", want, body)
		}
	}
}

// The Colours picker offers a single Solarized family row; its Light/Dark
// variant is chosen by the modeOption toggle below it. The legacy
// 'solarized-light' / 'solarized-osaka' tokens are gone from every colorscheme
// click target (picker and guide miniature alike).
func TestThemePickerOffersSolarizedFamily(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	for _, want := range []string{
		`data-on:click="$_colorscheme = &#39;solarized&#39;"`,
		`data-class:active="$_colorscheme === &#39;solarized&#39;"`,
		`Solarized</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing Solarized family %q; body:\n%s", want, body)
		}
	}
	for _, gone := range []string{
		`$_colorscheme = &#39;solarized-light&#39;`,
		`$_colorscheme = &#39;solarized-osaka&#39;`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("page still references legacy %q colorscheme token; body:\n%s", gone, body)
		}
	}
}

// Every colourscheme is a family with both variants, so the picker offers the
// three family tokens and none of the legacy single-variant ones — and the
// pre-paint script carries each legacy value to its family + mode pair.
func TestThemePickerOffersFamiliesOnly(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	for _, family := range []string{"solarized", "nord", "rose-pine"} {
		for _, want := range []string{
			`data-on:click="$_colorscheme = &#39;` + family + `&#39;"`,
			`data-class:active="$_colorscheme === &#39;` + family + `&#39;"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("page missing %s family binding %q", family, want)
			}
		}
	}
	for _, gone := range []string{"catppuccin-mocha", "catppuccin", "rose-pine-dawn"} {
		if strings.Contains(body, `$_colorscheme = &#39;`+gone+`&#39;`) {
			t.Errorf("page still writes legacy %q colorscheme token", gone)
		}
	}
	for _, want := range []string{
		`"catppuccin-mocha": ["nord", "dark"]`,
		`"catppuccin": ["nord", null]`,
		`"rose-pine-dawn": ["rose-pine", "light"]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("pre-paint script missing legacy migration %q", want)
		}
	}
}

// The Light/Dark toggle writes $_colormode; both rows are present so the
// colourscheme family and its variant are independent, persistent axes.
func TestThemePickerOffersLightDarkModeToggle(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	for _, want := range []string{
		`data-on:click="$_colormode = &#39;light&#39;"`,
		`data-on:click="$_colormode = &#39;dark&#39;"`,
		`data-class:active="$_colormode === &#39;light&#39;"`,
		`data-class:active="$_colormode === &#39;dark&#39;"`,
		`Light</button>`,
		`Dark</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing Light/Dark mode toggle %q; body:\n%s", want, body)
		}
	}
}
