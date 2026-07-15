# 002 — Colorscheme licenses and "using them by name" in a commercial app

Status: research (no code committed)
Date: 2026-07-15

The app ships three colorschemes, selected via `data-colorscheme` and defined
in `internal/frontend/static/app.css` + `internal/frontend/components/modals/theme.templ`:

1. `solarized-light`
2. `solarized-osaka` (Solarized Dark Osaka)
3. `catppuccin-mocha`

The question is twofold: (a) what licenses govern them, and (b) what is the
situation around **using them by name** in a quasi-commercial product. The
short answer: all three are **permissive, commercial-use-friendly** licenses
(MIT ×2, Apache-2.0 ×1), and the names carry **no registered trademark
restriction** — using them by name to identify which palette you're offering is
exactly the intended use. The one real obligation is **attribution**.

---

## A. The licenses (verified from primary sources)

Every claim below is from the upstream repo's own LICENSE/README, fetched
2026-07-15.

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

All three are **OSI-approved permissive** licenses: no copyleft, no
non-commercial clause, no field-of-use restriction, no revenue cap. None has an
attached NOTICE file, so Apache-2.0 §4(d) adds nothing beyond MIT-style
attribution.

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
each palette as hand-written CSS custom properties with our own HSL values
(`app.css` `:root[data-colorscheme=...]` blocks). Even in the absence of any
license, copying a palette's color values is not copyright infringement. The
licenses matter for courtesy/attribution and for the (small) implementation we
did inherit conceptually, not because the palette values are owned.

### B2. Trademark — neither name is restricted

I found **no trademark policy or assertion** for either name:

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

So displaying the labels "Solarized Light", "Solarized Dark Osaka", and
"Catppuccin Mocha" in the theme picker is **nominative/descriptive use** —
identifying the palette the user is selecting — which is both the accepted
community norm and squarely protected (and in any event unchallenged here).

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

Because we are not redistributing their source files verbatim, the clean way to
satisfy attribution is a **THIRD_PARTY_LICENSES** (a.k.a. "open-source
notices" / credits) file — and optionally an in-app "About / Credits" entry —
reproducing each project's copyright line and license. Apache-2.0 in particular
also gently wants modified-file notices if you ever did ship their files, which
we don't.

---

## D. Recommendation (concrete)

1. **Add `THIRD_PARTY_LICENSES.md`** at the repo root (or link it from an
   About modal) reproducing verbatim:

   - Solarized — "Copyright (c) 2011 Ethan Schoonover" + MIT text.
   - Solarized Osaka — "Copyright 2024 Takuya Matsuyama" + Apache-2.0 text.
   - Catppuccin — "Copyright (c) 2021 Catppuccin" + MIT text.

   Link each to its upstream repo. This single step satisfies every license
   obligation above.

2. **Keep the human-readable names** "Solarized Light", "Solarized Dark Osaka",
   "Catppuccin Mocha" in the theme picker — they're descriptive/nominative use
   of unasserted names, consistent with both projects' ecosystems. No change
   needed.

3. **No need to rename, dual-license, or restrict** any colorscheme for the
   commercial path. None of the three has a non-commercial, branding, or
   field-of-use string attached.

4. (Optional belt-and-suspenders) A one-time USPTO TESS search for "Solarized"
   and "Catppuccin" in class 9 to confirm no surprise registration. Expected
   result: none of concern.

---

## E. Sources

- Solarized LICENSE — https://raw.githubusercontent.com/altercation/solarized/master/LICENSE
- Solarized repo metadata (MIT confirmation) — https://api.github.com/repos/altercation/solarized
- Solarized Osaka LICENSE — https://raw.githubusercontent.com/craftzdog/solarized-osaka.nvim/main/LICENSE
- Solarized Osaka README — https://raw.githubusercontent.com/craftzdog/solarized-osaka.nvim/main/README.md
- Catppuccin LICENSE — https://raw.githubusercontent.com/catppuccin/catppuccin/main/LICENSE
- Catppuccin README (commercial-use confirmation) — https://raw.githubusercontent.com/catppuccin/catppuccin/main/README.md
- US Copyright Office, Compendium of Practices §313.4(K) (colors not copyrightable) — https://www.copyright.gov/comp3/chap300/ch300-copyrightable-authorship.pdf
