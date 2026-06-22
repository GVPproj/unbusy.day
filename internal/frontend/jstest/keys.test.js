// Unit tests for the pure keyboard decision reducer (PRD: keyboard-accessible
// blocks). Mirrors push.test.js. Run: node --test internal/frontend/jstest
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

// The glue relies on null to mean "not my key" — so it won't preventDefault and
// the browser keeps its native behaviour (Tab to move focus, page scroll, etc.).
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
