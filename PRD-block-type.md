# PRD — Block Type

Status: **Code complete — core, DB, adapter, frontend views & theming done; manual contrast eyeballing remains**
Branch: `feat/blockType`
Last updated: 2026-06-17

## Summary

Add a **Block Type** to every Block: a flat three-way tag — `deep`, `shallow`,
or `break`. The User picks the type when creating a Block (a colorscheme-aware
swatch radio in the create modal), and Blocks are color-coded by type across the
Day Plan, drawing fill/ink colors from each canonical colorscheme's palette.

See `CONTEXT.md` › **Block Type** for the glossary entry.

## Goals

- Every Block carries exactly one Block Type, chosen at creation.
- Create modal offers a native, colorscheme-respecting swatch radio (deep/shallow/break).
- Blocks render with a full background fill keyed on type, legible in all three colorschemes.

## Non-goals

- **Editing a Block's type after creation.** Type is immutable; changing it means
  delete + recreate. An edit path is a possible future issue, out of scope here.
- Any change to the layout/drag wire — type is not spatial and never enters the
  `{id, slot, span}` payload.
- Analytics, filtering, or Templates keyed on type.

## Settled design decisions

| # | Decision | Choice |
|---|----------|--------|
| 1 | Concept & name | Flat three-way **Block Type**: `deep`/`shallow`/`break` (deep & shallow are work grades, break is rest; treated as peer values) |
| 2 | Default | `shallow` is the universal default — DB column default, seeded starters, first + pre-checked radio |
| 3 | Representation & validation | Named `BlockType` string + constants; `ErrInvalidBlockType` rejection at the service; belt-and-suspenders DB `CHECK` |
| 4 | Mutability | Create-only; immutable after creation |
| 5 | Visual treatment | Full background fill per type (contrast iterated as needed) |
| 6 | Tokens | Paired `--type-{deep,shallow,break}` + `-ink` per colorscheme; deep→blue/violet, break→green from canonical palettes; **shallow = `--surface`/`--ink`** so the default type keeps the pre-feature block look |
| 7 | Create UI | Native `<fieldset>` radios bound to `addtype` signal (default `'shallow'`, listed first), styled as color swatches; reset on submit |

**No ADR** — additive column following the established `span` (`CHECK`) and
sentinel-rejection patterns; reversible and unsurprising.

## Implementation steps

Tracked as a checklist. Mark `[x]` when done; update **Status** above as phases complete.

### Core — `internal/block/`
- [x] Add `type BlockType string` with `BlockDeep`/`BlockShallow`/`BlockBreak` constants and a validity check
- [x] Add `Type BlockType` field to the `Block` struct (`json:"type"`)
- [x] Add `ErrInvalidBlockType` sentinel
- [x] Extend `Create` signature to `Create(ctx, owner, label, slot, typ)`; trim/validate type, reject invalid (blank → `deep`)
- [x] Add `type` to the explicit column list in `queryBlocks` and the scan in `scanBlocks`
- [x] Name `type` in `Create`'s `INSERT`; confirm `Seed` is unchanged (relies on DB default)
- [x] Unit test: invalid type rejects; valid type round-trips; blank defaults to deep

### Database — `internal/migrate/migrations/`
- [x] New timestamp-versioned migration: `ALTER TABLE block ADD COLUMN type TEXT NOT NULL DEFAULT 'deep' CHECK (type IN ('deep','shallow','break'))`
- [x] Verify boot migrate backfills existing rows to `deep` (covered by `TestCreate_BlankTypeDefaultsToDeep`: seeded starters read back `deep`)

### Frontend adapter — `internal/frontend/adapter.go`
- [x] Add `Type string` to `createSignals` (`json:"addtype"`)
- [x] Pass type into `svc.Create`; add `ErrInvalidBlockType` arm to the 200-rejection `switch` in `CreateHandler`
- [x] Update the `BlockService` interface `Create` signature

### Frontend views — `internal/frontend/components/`
- [x] `create.templ`: native `<fieldset>` of three radios bound to `addtype`, default `'deep'` (`data-signals:addtype`), deep pre-checked; reset `$addtype = 'deep'` on submit
- [x] `create.templ`: style options as color swatches using the `--type-*` tokens; `accent-color: var(--accent)` fallback (in new `CreateStyles`, wired into the blocks route's Layout)
- [x] `column_block.templ`: add `data-type={ string(c.Type) }` to the block `<li>` (placed after `data-slot` so the `data-id/span/slot` group the tests assert stays contiguous)
- [x] `column.templ` `ColumnStyles`: `.block[data-type="…"]` sets `background: var(--type-…)` and `color: var(--type-…-ink)`

### Theming — `internal/frontend/layouts/layout.templ`
- [x] Add paired `--type-{deep,shallow,break}` + `-ink` tokens to each `:root[data-colorscheme=…]` block (solarized-light, solarized-osaka, catppuccin-mocha), hues from each canonical palette
- [ ] Eyeball contrast in all three schemes; iterate fills/inks where text is unreadable (starting values chosen; needs a human pass)

### Verification
- [x] `task templ` regenerates; `task test` passes (and `go test -race ./...` clean); added `TestColumnRendersBlockType`
- [ ] Manual: create a block of each type, confirm color + readable label in all three colorschemes
- [ ] Manual: drag/stretch a typed block — `data-type` survives the morph; live SSE re-render keeps the color
- [ ] `git commit -s` (DCO)

## Open questions / iteration notes

- Exact per-scheme hex/hsl values for the 6 type tokens × 3 schemes — to be tuned during the theming step.
- If full-fill contrast proves stubborn for a specific type/scheme, the paired `-ink` token is the lever (no structural change needed).
