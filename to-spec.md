# to-spec — UNB-26 review follow-ups

Checklist of changes/improvements surfaced by the two-axis review of `refactor/block-gestures` against UNB-26. None are merge blockers — the refactor is behaviour-preserving and standards-clean. Ordered roughly by value.

## Fix before merge

- [x] **Resolve the doc/test contradiction.** Softened the privacy comment in `block-gestures.js` to sanction the one co-located contract test (`jstest/block-gestures.test.js`) importing `gestures/keyboard.js` directly, with the reason spelled out (the entry can't load under `node --test` because `pointer.js` pulls Motion from a CDN). The return-the-handles alternative was rejected: routing the test through `initBlockGestures` would trigger the lazy `import("./gestures/pointer.js")` and its unresolvable Motion CDN import under node.

## Standards cleanups (judgement calls)

- [x] **De-dupe teardown in `gestures/pointer.js`.** Extracted `tearDown(g, kind, layout)` (detach → `teardownSibs` → clear inline style + active class → guarded `writeLayout`) and called it from all four sites — `cancel()`'s drag and resize branches and the `settleDrag`/`settleResize` `finally` blocks. The `writeLayout` guard is now uniform (`cancel` was previously unguarded, but the block is always present during a synchronous supersede, so behaviour is unchanged).
- [x] **Reorder `enterEdit` params in `gestures/rename.js`.** Now `enterEdit(list, label, x, y)` — required `list` leads, optional coordinates trail. Both call sites updated; keyboard entry drops to `enterEdit(list, label)` (undefined coords → `byKeyboard`).
- [~] **(optional) Rename `keyboard-reducer.js`** — deferred. No clearly-better name presented itself (it genuinely owns a reducer; the formatting helpers read as part of the same pure keyboard tier), and a rename ripples through `keyboard.js`, `keyboard-reducer.test.js`, and `CLAUDE.md`. Left as-is per the item's own "only if a better name presents itself" guidance.

## Spec accuracy nits

- [x] **Fix the `cancel()` comment in `gestures/pointer.js`.** Rewritten to say it is *not* pointercancel: pointercancel springs back via `settleDrag/settleResize(e, false)` while `cancel()` reverts synchronously with no spring.
- [x] **Decide the fate of `arb.pointer.cancel()`.** Kept, with a note added to its comment: it's part of the symmetric `{ isActive, cancel }` handle `bindArb` enforces but currently unreferenced (the keyboard path bails on `isActive` rather than cancelling the pointer), retained so the contract holds if that policy flips.
- [x] **(doc only) Reconcile the "entry owns arbitration" wording.** Updated the `block-gestures.js` module header: the entry *wires the shared `arb` registry* (hands each path a live handle to the other), and each path *enforces the policy itself* — "the entry owns the wiring; the paths own the checks."

## Verification still outstanding (from the ticket)

- [ ] **`/verify` Playwright pass** — pointer drag, stretch, keyboard grab→arrow→drop, splitter resize, F2 rename, keyboard delete, SSE-morph survival. (`task test` is already green.)
