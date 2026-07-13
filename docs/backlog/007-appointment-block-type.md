# 007 — Add `appointment` as a 4th Block Type

Status: spec / ready to build
Date: 2026-07-13

## What

Add a fourth `BlockType`, **`appointment`**, alongside `deep` / `shallow` /
`break` — the fixed-time commitment (dentist, standup, school pickup) a user
adds first, before building the rest of the day. It is a peer tag value: a
new label + color, nothing more. See "Scope / non-goals" before building.

Colors were picked from a throwaway prototype
(`internal/frontend/static/prototype-appointment-colors.html` — delete it as
part of this work). Chosen `--type-appointment` / `--type-appointment-ink`
pairs, drawn from each scheme's canonical palette:

| Colorscheme        | Option           | `--type-appointment` (fill) | `--type-appointment-ink` |
| ------------------ | ---------------- | --------------------------- | ------------------------ |
| solarized-light    | A — Yellow (Solarized `#b58900`) | `hsl(45, 90%, 80%)`  | `hsl(45, 100%, 25%)` |
| solarized-osaka    | A — Yellow (Osaka yellow hue 45) | `hsl(45, 55%, 22%)`  | `hsl(45, 85%, 75%)`  |
| catppuccin-mocha   | C — Maroon (Catppuccin `#eba0ac`)| `hsl(350, 40%, 26%)` | `hsl(350, 65%, 77%)` |

Note Mocha uses Maroon, not a yellow — Catppuccin Yellow reads too close to
its Peach, and Peach is already `--nav-icon`. The prototype's `.opt-amber`
class name is a misnomer for the Mocha row (it's rose/maroon); ignore the
class name, use the hsl pairs above.

## Scope / non-goals

`appointment` is a **flat peer value of the existing Block Type tag** — a
color and a label. It does **not** make a block time-anchored, pinned, or
immovable; appointment blocks still drag, stretch, and get pushed by the
layout engine exactly like every other block. If we later want genuinely
locked-to-the-clock blocks, that is a separate, much larger feature (new
invariant in `block/`, push-cascade changes, ADR) and is explicitly out of
scope here. This is why the CONTEXT.md model still holds: "peer values of one
attribute."

## Files to change

### 1. Domain — `internal/block/block.go`

- Add the const: `BlockAppointment BlockType = "appointment"` (block.go:17–21).
- Add `BlockAppointment` to the `Valid()` switch (block.go:23–29).
- Update the `BlockType` doc comment (block.go:14) — it currently enumerates
  "deep/shallow work or break".

Nothing else in `block/` needs to change: `Create` passes the type straight
through `Valid()`, and `Seed` still seeds shallow starters (the default is
unchanged). `adapter.go` is already type-agnostic (`block.BlockType(sig.Type)`).

### 2. Migration — new file in `internal/migrate/migrations/`

The `type` column carries a CHECK: `CHECK (type IN ('deep','shallow','break'))`
(`20260617000000_block_type.sql:6`). SQLite can't `ALTER` a CHECK in place, so
widen it with a forward-only table rebuild. Nothing has a foreign key **to**
`block` (only `block → user`), so no `PRAGMA foreign_keys` toggle is needed —
and `PRAGMA foreign_keys` is a no-op inside goose's transaction anyway.

New file `20260713000000_block_type_appointment.sql` (pick a timestamp after
`20260623120000`):

```sql
-- +goose Up
-- Widen the block.type CHECK to admit 'appointment' (the 4th Block Type).
-- SQLite can't alter a CHECK in place, so rebuild the table verbatim with the
-- expanded IN list. Nothing references block by FK, so no foreign_keys toggle.
CREATE TABLE block_new (
  id         TEXT    PRIMARY KEY,
  label      TEXT    NOT NULL,
  position   INTEGER NOT NULL,
  span       INTEGER NOT NULL DEFAULT 1 CHECK (span >= 1),
  created_at TEXT    NOT NULL DEFAULT (datetime('now')),
  owner_id   TEXT    REFERENCES "user"(id) ON DELETE CASCADE,
  type       TEXT    NOT NULL DEFAULT 'shallow'
                     CHECK (type IN ('deep', 'shallow', 'break', 'appointment'))
);
INSERT INTO block_new (id, label, position, span, created_at, owner_id, type)
  SELECT id, label, position, span, created_at, owner_id, type FROM block;
DROP TABLE block;
ALTER TABLE block_new RENAME TO block;
```

Column order in `block_new` matches the post-`block_type`-migration shape
(`type` last, as `ALTER TABLE ADD COLUMN` appended it). No indexes on `block`
to recreate. Forward-only, no Down (ADR 0004).

### 3. Styles — `internal/frontend/static/app.css`

**Token definitions** — add `--type-appointment` + `--type-appointment-ink`
to each colorscheme block, beside the existing `--type-*` tokens:

- Default / `solarized-light` (shared `:root` block, app.css:172–177):
  `--type-appointment: hsl(45, 90%, 80%); --type-appointment-ink: hsl(45, 100%, 25%);`
- `solarized-osaka` (app.css:210–215):
  `--type-appointment: hsl(45, 55%, 22%); --type-appointment-ink: hsl(45, 85%, 75%);`
- `catppuccin-mocha` (app.css:248–253):
  `--type-appointment: hsl(350, 40%, 26%); --type-appointment-ink: hsl(350, 65%, 77%);`

**Render sites** — add an `&[data-type="appointment"]` branch to each of the
four selector groups that currently list deep/shallow/break. Match each site's
existing shape (swatches set `background` only; blocks set `background` +
`color`):

- `.swatch` in the create modal — app.css:732–740 (background only).
- `.guide-swatch` in the guide — app.css:812–820 (background only).
- `.gc-block` in the guide mini-column — app.css:863–875 (background + color).
- `.block-item` in the real day grid — app.css:1548–1559 (background + color).

Cosmetic: the "three type swatches" comment at app.css:787 → "four".

### 4. Create modal — `internal/frontend/components/modals/create.templ`

- Add one option to `.type-options` (create.templ:30–34):
  `@createTypeOption("appointment", "Appointment")`.
- Default (`data-signals:addtype="'shallow'"`) and the reset in the Create
  click handler stay `shallow` — do not change them.
- **Ordering decision (needs a call):** current order is Shallow, Deep, Break.
  Recommend appending **Appointment last** — keeps the default (shallow) first
  and is the least disruptive. Keep create.templ and the guide list (below) in
  the same order.

### 5. Guide modal — `internal/frontend/components/modals/guide.templ`

- "Give it one of three types:" → "four types" (guide.templ:45).
- Add a 4th `<li>` to `.guide-types` (guide.templ:46–50), same order as create:
  `<li><span class="guide-swatch" data-type="appointment" aria-hidden="true"></span><span><strong>Appointment</strong> — a fixed-time commitment</span></li>`
- Optional: seed an appointment block into the pane-3 demo column
  (guide.templ:55–65) so the new color appears in the interactive figure. Not
  required for correctness.

### 6. Regenerate templ + verify

- `task templ` to regenerate `create_templ.go` / `guide_templ.go`. **Not while
  `task dev` is running** (deletes the watch session's literal cache → 500s).
- `task build` / `go vet` to confirm the new const compiles through.

### 7. Tests — `internal/block/` and `internal/frontend/`

- `internal/block/block_test.go`:
  - `TestBlockType_Valid` (block_test.go:715–722) — add `BlockAppointment` to
    the valid set. Leave the invalid cases (`"focus"`, `"DEEP"`, …) as-is.
  - Optional: a `Create` test with `block.BlockAppointment` mirroring
    `TestCreate` (block_test.go:551), asserting it persists and publishes.
- `internal/frontend/adapter_test.go`:
  - Optional: extend `TestColumnRendersBlockType` (adapter_test.go:596) with an
    appointment block, asserting `data-type="appointment"` renders.
- Migration sanity: `task nuke && <boot>` (or `task migrate`) to confirm the
  rebuild applies clean and existing rows survive. Insert an `appointment` row
  to confirm the widened CHECK accepts it.
- `/verify` skill for a visual pass: the new swatch in the create modal, the
  guide legend row, and an appointment block on the grid, across all three
  colorschemes.

### 8. Domain + marketing copy (docs — do together, lower urgency)

- `CONTEXT.md` "Block Type" (CONTEXT.md:16–21) — currently "A flat three-way
  tag … deep, shallow, break." Update to four and add the appointment gloss
  ("a fixed-time commitment"), keeping the "peer values of one attribute /
  still movable" framing from Scope above.
- `README.md:26` "one of three types" + the alt text on README.md:29.
- Marketing image: `docs/readme-types.html` → `.github/readme-types.png` is a
  rendered asset showing the three types; regenerate it with the 4th row if we
  refresh README imagery. (Asset regen, not code — can trail the feature.)
- `docs/research/001-…md:179` describes the "flat three-way tag" — leave as a
  historical research note or add a one-line addendum; not load-bearing.

### 9. Cleanup

- Delete `internal/frontend/static/prototype-appointment-colors.html` (throwaway,
  not linked or embedded).

## Order to build

1. `block.go` const + `Valid()` → 2. migration → 3. `task templ`-consuming
templ edits (create + guide) → 4. app.css tokens + render sites → 5. `task
templ` + build + tests → 6. docs/copy + delete prototype.

## Open decisions

- **Type order** in the create modal / guide legend (recommend: append
  Appointment last).
- **Guide demo block** — seed an appointment into the pane-3 interactive column
  (optional).
- **Marketing image** regen timing — with this PR or a follow-up.

## Related

- CONTEXT.md "Block Type" — the domain definition being extended.
- `20260617000000_block_type.sql` — the migration this one widens; ADR 0004
  (forward-only goose migrations) governs the rebuild approach.
- ADR 0011 (plain CSS, tokens) — new colors must be `--type-*` tokens per
  scheme, never hardcoded.
- `prototype-appointment-colors.html` — where the color options were chosen.
