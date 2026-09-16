# 002 — Colorscheme licenses and "using them by name" in a commercial app

Status: complete — recommendation implemented
Research date: 2026-07-15
Outcome: approved for commercial use; original notices recorded in `THIRD_PARTY_LICENSES.md`

The current theme model has two independent color axes, implemented in
`internal/frontend/static/css/app.css` and selected in
`internal/frontend/components/modals/theme.templ`:

- `data-colorscheme` selects the family: `catppuccin`, `solarized`, or `nord`.
- `data-colormode` selects `light` or `dark` within that family.

That produces Catppuccin Latte/Mocha, Solarized Light/Solarized Dark Osaka,
and light/dark Nord variants based on One Nord and Nord. Catppuccin and Nord
therefore coexist; neither replaces the other.

The original point-in-time research covered Solarized Light, Solarized Dark
Osaka, and Catppuccin Mocha. Its question was twofold: (a) what licenses govern
them, and (b) whether their names can be used in a commercial product. The
answer remains that the palette sources use permissive, commercial-use-friendly
licenses and that the originally reviewed names can be used descriptively to
identify the selected palette. Nord's MIT licensing is recorded below; this
update did not repeat the original trademark-policy search for Nord/One Nord.
The implemented outcome was to keep the recognizable names and add a root
`THIRD_PARTY_LICENSES.md` notice file.

---

## A. The licenses (verified from primary sources)

Sections 1–3 preserve the upstream LICENSE/README findings fetched on
2026-07-15. Section 4 records the MIT licensing relevant to the subsequently
added Nord family.

### 1. Solarized — MIT

- Source: `github.com/altercation/solarized` LICENSE file
  (`https://raw.githubusercontent.com/altercation/solarized/master/LICENSE`).
- Text: "Copyright (c) 2011 Ethan Schoonover" + standard MIT terms.
- GitHub repo metadata (`api.github.com/repos/altercation/solarized`) reports
  `license.name = "MIT License"`, `spdx_id = "MIT"`.
- README author line: "Solarized … author: Ethan Schoonover, created 2011 Mar 15".

### 2. Solarized Osaka — Apache License 2.0

- Source: `github.com/craftzdog/solarized-osaka.nvim` LICENSE file.
- Boilerplate: "Copyright 2024 Takuya Matsuyama … Licensed under the Apache
  License, Version 2.0".
- README describes it as "One of the
  [Solarized](https://ethanschoonover.com/solarized/)-inspired dark theme[s]"
  that "improves upon this by introducing additional colors." I.e. a derivative
  of MIT-licensed Solarized — license-compatible (MIT is compatible with
  Apache-2.0).

### 3. Catppuccin — MIT

- Source: `github.com/catppuccin/catppuccin` LICENSE file.
- Text: "Copyright (c) 2021 Catppuccin" + standard MIT terms.
- README explicitly enumerates the permissions MIT grants, **listing
  "Commercial use"** first — a deliberate, written confirmation that commercial
  use is intended and allowed.

### 4. Nord / One Nord — MIT

The current Nord family draws from two permissively licensed sources:

- `github.com/rmehri01/onenord.nvim` — MIT, copyright 2021 Ryan Mehri. The
  light palette is based on its `onenordlight` palette, and the dark palette
  uses its accents.
- `github.com/nordtheme/nord` — MIT, copyright 2016–present Sven Greb. Its
  Polar Night colors inform the dark palette.

All of these are **OSI-approved permissive** licenses: no copyleft,
non-commercial clause, field-of-use restriction, or revenue cap. The original
three projects had no attached NOTICE file, so Apache-2.0 §4(d) added nothing
beyond retaining the applicable license and attribution notices when
redistributing covered material.

---

## B. Why "using them by name" is fine — copyright vs. trademark

The worry behind "using them by name" conflates two separate bodies of law.

### B1. Copyright does not protect the colors themselves

A color, and a simple palette of colors, is **not copyrightable** in the US.
The Copyright Office Compendium of Practices §313.4(K) states the Office will
not register claims limited to colors. What *is* copyrightable in these
projects is the **specific code/implementation** — the vim color files, the
Lua plugin, the generated CSS — not the 16 hex values.

Concretely: unbusy.day does **not** copy any upstream code file. It re-expresses
each palette as hand-written CSS custom properties with HSL values in
`internal/frontend/static/css/app.css`
(`:root[data-colorscheme=...][data-colormode=...]` blocks). Even in the absence
of any license, copying a palette's color values is not copyright infringement.
The licenses matter for provenance and courtesy attribution, not because the
palette values themselves are owned.

### B2. Trademark — no naming restriction was found

The 2026-07-15 review found **no trademark policy or assertion** for Solarized
or Catppuccin:

- No `TRADEMARK.md` / `.github/TRADEMARK.md` / trademark page in
  `catppuccin/catppuccin` or the catppuccin `.github` org profile; the
  `catppuccin.com` homepage carries no trademark notice.
- The Solarized LICENSE and README make no trademark claim; Ethan Schoonover
  has never asserted control over the "Solarized" name.

More tellingly, **both projects' own ecosystems are built on third parties using
the name**:

- Solarized's own README links dozens of community ports all named
  "…-solarized" (`iterm2-colors-solarized`, `vim-colors-solarized`, …). The
  name is used purely to identify the palette.
- Catppuccin is an org of **hundreds** of community ports literally titled
  "Catppuccin for X"; using the name to identify the palette is the intended
  and universal pattern.

The current picker uses the family labels "Catppuccin", "Solarized", and
"Nord", with a separate Light/Dark choice. They are descriptive labels for the
selected palette family. The detailed policy search above covers Solarized and
Catppuccin; make an equivalent Nord/One Nord check before treating the absence
of a Nord naming restriction as independently verified by this document.

> Standard disclaimer: this is engineering research, not legal advice. If a
> registered word mark turned up later, descriptive use of a name to identify a
> theming palette would still be the safe norm — but a quick USPTO TESS search
> for "Solarized" and "Catppuccin" (class 9 software) is the belt-and-suspenders
> check if you want one.

---

## C. What MIT / Apache-2.0 actually require of a commercial app

Both licenses are unambiguously commercial-use-friendly; there is nothing
"quasi" to worry about. The only affirmative obligation is **attribution**:

- **MIT**: "The above copyright notice and this permission notice shall be
  included in all copies or substantial portions of the Software."
- **Apache-2.0**: retain copyright/license/NOTICE notices; if you modify a
  file, "cause … modified files to carry prominent notices stating that You
  changed the files." (No NOTICE file exists upstream, so §4(d) adds nothing.)

Because no upstream palette source files are redistributed verbatim, the clean
way to record provenance and conservatively satisfy any applicable attribution
is a **THIRD_PARTY_LICENSES** (a.k.a. "open-source notices" / credits) file.
That file now exists at the repository root and includes the notices from the
original research. Apache-2.0 would additionally require modified-file notices
if the app shipped modified Solarized Osaka files, which it does not.

---

## D. Recommendation — completed

1. **Create `THIRD_PARTY_LICENSES.md` at the repository root.** Done. It records
   the original Solarized, Solarized Osaka, and Catppuccin copyright and license
   notices and links to their upstream repositories.
2. **Keep recognizable, human-readable palette names.** Done. The picker now
   presents Catppuccin, Solarized, and Nord as families, with Light/Dark as an
   independent choice.
3. **Do not rename, dual-license, or restrict the palettes for commercial use.**
   Done. The current palette sources are MIT or Apache-2.0 licensed and have no
   non-commercial or field-of-use condition.

The optional trademark search noted in the original recommendation was a
belt-and-suspenders check, not a blocker to implementation.

---

## E. Sources

- Solarized LICENSE — https://raw.githubusercontent.com/altercation/solarized/master/LICENSE
- Solarized repo metadata (MIT confirmation) — https://api.github.com/repos/altercation/solarized
- Solarized Osaka LICENSE — https://raw.githubusercontent.com/craftzdog/solarized-osaka.nvim/main/LICENSE
- Solarized Osaka README — https://raw.githubusercontent.com/craftzdog/solarized-osaka.nvim/main/README.md
- Catppuccin LICENSE — https://raw.githubusercontent.com/catppuccin/catppuccin/main/LICENSE
- Catppuccin README (commercial-use confirmation) — https://raw.githubusercontent.com/catppuccin/catppuccin/main/README.md
- One Nord LICENSE — https://raw.githubusercontent.com/rmehri01/onenord.nvim/main/LICENSE
- Nord LICENSE — https://raw.githubusercontent.com/nordtheme/nord/develop/license
- US Copyright Office, Compendium of Practices §313.4(K) (colors not copyrightable) — https://www.copyright.gov/comp3/chap300/ch300-copyrightable-authorship.pdf

---

## Implementation update

Nord / One Nord was added after the original research and was briefly described
as a replacement for Catppuccin. That description is no longer true. The
current picker and stylesheet ship **all three** families — Catppuccin,
Solarized, and Nord — and apply the independent `light` / `dark` colormode to
each. One Nord and upstream Nord are both MIT licensed, so adding Nord does not
change the original commercial-use conclusion.
