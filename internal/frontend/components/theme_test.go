// Render tests pinning the theme picker's native radio groups and the
// colourscheme family + Light/Dark mode axis.
package components_test

import (
	"strings"
	"testing"
)

// Every rendered picker is an independent native group. The dialog and Guide
// bind those groups to the same signals without merging their arrow-key scope.
func TestThemePickersRenderIndependentBoundRadioGroups(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	groups := map[string]int{
		"theme-feeling":      3,
		"theme-colourscheme": 3,
		"theme-colormode":    2,
		"guide-colourscheme": 3,
		"guide-colormode":    2,
		"guide-feeling":      3,
	}
	for name, want := range groups {
		if got := strings.Count(body, `type="radio" name="`+name+`"`); got != want {
			t.Errorf("radio group %q has %d options, want %d", name, got, want)
		}
	}
	for signal, want := range map[string]int{
		"_colorscheme": 6,
		"_colormode":    4,
		"_feeling":      6,
	} {
		if got := strings.Count(body, `data-bind:`+signal); got != want {
			t.Errorf("signal %q has %d bound radios, want %d", signal, got, want)
		}
	}
	for _, gone := range []string{
		`data-on:click="$_colorscheme =`,
		`data-on:click="$_colormode =`,
		`data-on:click="$_feeling =`,
		`data-class:active="$_colorscheme`,
		`data-class:active="$_colormode`,
		`data-class:active="$_feeling`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("page retains manual selection plumbing %q", gone)
		}
	}
}

// The feeling picker offers "Mono" — the JetBrains-Mono + Octicons feeling —
// and the retired "business" token appears nowhere in the rendered page.
func TestThemePickerOffersMonoFeeling(t *testing.T) {
	body := renderPage(t, threeBlocks(), testBounds)

	for _, want := range []string{
		`data-bind:_feeling value="mono"`,
		`Mono</label>`,
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
		`data-bind:_colorscheme value="solarized"`,
		`Solarized</label>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing Solarized family %q; body:\n%s", want, body)
		}
	}
	for _, gone := range []string{
		`value="solarized-light"`,
		`value="solarized-osaka"`,
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

	for _, family := range []string{"solarized", "nord", "catppuccin"} {
		if want := `data-bind:_colorscheme value="` + family + `"`; !strings.Contains(body, want) {
			t.Errorf("page missing %s family binding %q", family, want)
		}
	}
	for _, gone := range []string{"catppuccin-mocha", "rose-pine-dawn", "rose-pine"} {
		if strings.Contains(body, `value="`+gone+`"`) {
			t.Errorf("page still offers legacy %q colorscheme token", gone)
		}
	}
	for _, want := range []string{
		`"catppuccin-mocha": ["catppuccin", "dark"]`,
		`"rose-pine-dawn": ["catppuccin", "light"]`,
		`"rose-pine": ["catppuccin", null]`,
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
		`data-bind:_colormode value="light"`,
		`data-bind:_colormode value="dark"`,
		`Light</label>`,
		`Dark</label>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing Light/Dark mode toggle %q; body:\n%s", want, body)
		}
	}
}
