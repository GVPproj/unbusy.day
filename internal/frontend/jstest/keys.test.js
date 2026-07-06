// Unit tests for the pure keyboard decision reducer.
// Run: node --test internal/frontend/jstest
import test from "node:test";
import assert from "node:assert/strict";
import { keyboardLayout } from "../static/keys.js";
import { pushLayout } from "../static/push.js";

const bounds = { start: 18, end: 34 }; // 9:00–17:00, end-exclusive

test("ArrowDown on a grabbed block moves it one slot later, cascading like pushLayout", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 19, span: 1 },
	];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "move" }, "ArrowDown");
	assert.equal(out.kind, "moved");
	assert.deepEqual(out.layout, pushLayout(bounds, current, { id: "a", slot: 19, span: 1 }));
});

test("ArrowUp on a grabbed block moves it one slot earlier", () => {
	const current = [
		{ id: "a", slot: 20, span: 1 },
		{ id: "b", slot: 19, span: 1 },
	];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "move" }, "ArrowUp");
	assert.equal(out.kind, "moved");
	assert.deepEqual(out.layout, pushLayout(bounds, current, { id: "a", slot: 19, span: 1 }));
});

test("ArrowUp at the first slot of the day is blocked, layout unchanged", () => {
	const current = [{ id: "a", slot: 18, span: 1 }];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "move" }, "ArrowUp");
	assert.equal(out.kind, "blocked");
	assert.deepEqual(out.layout, current);
});

test("ArrowDown at the last slot of the day is blocked, layout unchanged", () => {
	const current = [{ id: "a", slot: 33, span: 1 }]; // 33 is the last slot (end 34 exclusive)
	const out = keyboardLayout(bounds, current, { id: "a", mode: "move" }, "ArrowDown");
	assert.equal(out.kind, "blocked");
	assert.deepEqual(out.layout, current);
});

test("ArrowDown skips past a taller block below instead of stalling on an infeasible step", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 }, // span-1 block against the top edge
		{ id: "b", slot: 19, span: 2 }, // taller block touching directly below
	];
	// Stepping a→19 is infeasible (b can't clear the top edge), so the move
	// skips to slot 20 rather than freezing on the blocked single step.
	assert.equal(pushLayout(bounds, current, { id: "a", slot: 19, span: 1 }), null);
	const out = keyboardLayout(bounds, current, { id: "a", mode: "move" }, "ArrowDown");
	assert.equal(out.kind, "moved");
	assert.deepEqual(out.layout, pushLayout(bounds, current, { id: "a", slot: 20, span: 1 }));
});

test("ArrowUp skips past a taller block above instead of stalling", () => {
	// An up-move displaces blocks downward, so its symmetric stall is against
	// the BOTTOM edge.
	const tight = { start: 18, end: 21 };
	const current = [
		{ id: "b", slot: 18, span: 2 }, // taller block at the top
		{ id: "a", slot: 20, span: 1 }, // span-1 block at the bottom edge, touching b
	];
	assert.equal(pushLayout(tight, current, { id: "a", slot: 19, span: 1 }), null);
	const out = keyboardLayout(tight, current, { id: "a", mode: "move" }, "ArrowUp");
	assert.equal(out.kind, "moved");
	assert.deepEqual(out.layout, pushLayout(tight, current, { id: "a", slot: 18, span: 1 }));
});

test("walking a span-1 block up past two tall blocks matches a single pointer drop (no accumulated shove)", () => {
	// Each press recomputes the cascade from the grab-START layout against the
	// running target (grabbed.slot), so displaced blocks slide by the mover's
	// span — never accumulating into a double shove + gaps.
	const start = [
		{ id: "x", slot: 18, span: 2 }, // 9:00–10:00
		{ id: "y", slot: 20, span: 2 }, // 10:00–11:00
		{ id: "a", slot: 22, span: 1 }, // the mover, directly below both
	];
	let slot = 22;
	let out;
	for (let i = 0; i < 4; i++) {
		out = keyboardLayout(bounds, start, { id: "a", mode: "move", slot }, "ArrowUp");
		assert.equal(out.kind, "moved");
		slot = out.slot; // thread the running target back in, as the glue does
	}
	assert.equal(slot, 18); // reached the top edge
	// Identical to one pointer drop of `a` at the top.
	assert.deepEqual(out.layout, pushLayout(bounds, start, { id: "a", slot: 18, span: 1 }));
	assert.deepEqual(out.layout, [
		{ id: "x", slot: 19, span: 2 },
		{ id: "y", slot: 21, span: 2 },
		{ id: "a", slot: 18, span: 1 },
	]);
});

test("ArrowDown on the resize handle grows the span by one, compressing below like pushLayout", () => {
	const current = [
		{ id: "a", slot: 20, span: 1 },
		{ id: "b", slot: 21, span: 1 },
	];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "ArrowDown");
	assert.equal(out.kind, "resized");
	assert.deepEqual(
		out.layout,
		pushLayout(bounds, current, { id: "a", slot: 20, span: 2 }, { compress: true }),
	);
});

test("ArrowUp on the resize handle shrinks the span by one", () => {
	const current = [{ id: "a", slot: 20, span: 2 }];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "ArrowUp");
	assert.equal(out.kind, "resized");
	assert.deepEqual(
		out.layout,
		pushLayout(bounds, current, { id: "a", slot: 20, span: 1 }, { compress: true }),
	);
});

test("ArrowUp on a span-1 block is blocked at the one-slot floor", () => {
	const current = [{ id: "a", slot: 20, span: 1 }];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "ArrowUp");
	assert.equal(out.kind, "blocked");
	assert.deepEqual(out.layout, current);
});

test("Home on the resize handle jumps to the minimum one-slot span", () => {
	const current = [{ id: "a", slot: 20, span: 3 }];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "Home");
	assert.equal(out.kind, "resized");
	assert.equal(out.span, 1);
	assert.deepEqual(
		out.layout,
		pushLayout(bounds, current, { id: "a", slot: 20, span: 1 }, { compress: true }),
	);
});

test("End on the resize handle jumps to the maximum legal span, compressing below", () => {
	const current = [
		{ id: "a", slot: 30, span: 1 },
		{ id: "b", slot: 32, span: 1 }, // one block below, floors at span 1
	];
	// Largest span that still fits: a→30–32 (span 3), b compressed to slot 33.
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "End");
	assert.equal(out.kind, "resized");
	assert.equal(out.span, 3);
	assert.deepEqual(
		out.layout,
		pushLayout(bounds, current, { id: "a", slot: 30, span: 3 }, { compress: true }),
	);
});

test("End with no room to grow stays at the current span", () => {
	// a is span 1 at the last slot; the only legal span is 1, so End is a no-op.
	const current = [{ id: "a", slot: 33, span: 1 }];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "End");
	assert.deepEqual(out.layout, current);
});

test("a grow the floors below can't absorb is blocked, layout unchanged", () => {
	const current = [
		{ id: "a", slot: 32, span: 1 },
		{ id: "b", slot: 33, span: 1 }, // already at the last slot, nothing to give
	];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "ArrowDown");
	assert.equal(out.kind, "blocked");
	assert.deepEqual(out.layout, current);
});

test("a resize step reports the resulting span so the glue can thread it back", () => {
	const current = [{ id: "a", slot: 20, span: 1 }];
	const out = keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "ArrowDown");
	assert.equal(out.span, 2);
});

test("ArrowDown grows from the running span cursor, not the start layout's span", () => {
	// The running target span threads through grabbed.span (as move threads
	// grabbed.slot): a second grow must go 2→3, not 1→2 again.
	const start = [{ id: "a", slot: 20, span: 1 }];
	const out = keyboardLayout(bounds, start, { id: "a", mode: "resize", span: 2 }, "ArrowDown");
	assert.equal(out.span, 3);
	assert.deepEqual(
		out.layout,
		pushLayout(bounds, start, { id: "a", slot: 20, span: 3 }, { compress: true }),
	);
});

test("shrinking after a compressing grow restores the neighbour's span (recomputed from start)", () => {
	// Shrinking must UNDO the grow's compression of `b`. Recomputing from the
	// running (already compressed) layout would never let b grow back — compress
	// only shrinks.
	const tight = { start: 18, end: 24 };
	const start = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 19, span: 3 },
	];
	let span;
	let out;
	for (let i = 0; i < 3; i++) {
		out = keyboardLayout(tight, start, { id: "a", mode: "resize", span }, "ArrowDown");
		assert.equal(out.kind, "resized");
		span = out.span;
	}
	assert.equal(span, 4); // 1→2→3→4
	assert.equal(out.layout.find((p) => p.id === "b").span, 2); // b compressed by the grow
	out = keyboardLayout(tight, start, { id: "a", mode: "resize", span }, "ArrowUp");
	assert.equal(out.span, 3);
	assert.equal(out.layout.find((p) => p.id === "b").span, 3);
	assert.deepEqual(
		out.layout,
		pushLayout(tight, start, { id: "a", slot: 18, span: 3 }, { compress: true }),
	);
});

// null means "not my key": the glue won't preventDefault, so the browser keeps
// its native behaviour (Tab focus, page scroll).
test("an unhandled key returns null", () => {
	const current = [{ id: "a", slot: 20, span: 1 }];
	assert.equal(keyboardLayout(bounds, current, { id: "a", mode: "move" }, "Tab"), null);
	assert.equal(keyboardLayout(bounds, current, { id: "a", mode: "move" }, "Home"), null); // Home is resize-only
	assert.equal(keyboardLayout(bounds, current, { id: "a", mode: "resize" }, "Enter"), null);
});

test("an unknown grabbed id returns null", () => {
	const current = [{ id: "a", slot: 20, span: 1 }];
	assert.equal(keyboardLayout(bounds, current, { id: "zzz", mode: "move" }, "ArrowDown"), null);
});
