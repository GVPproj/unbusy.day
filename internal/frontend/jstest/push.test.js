// Unit tests for the pure push cascade (ADR 0005). Run: node --test frontend/jstest
import test from "node:test";
import assert from "node:assert/strict";
import { pushLayout } from "../static/push.js";

const bounds = { start: 18, end: 34 }; // 9:00–17:00, end-exclusive

const byId = (layout, id) => layout.find((p) => p.id === id);

test("move into empty slots places the block, others untouched", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 19, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 25, span: 1 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 25, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 19, span: 1 });
	assert.equal(out.length, 2);
});

test("dragging down onto an occupied slot slides that block up behind it", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 20, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 19, span: 1 });
});

test("a gap absorbs the push so blocks past the gap stay put", () => {
	// Up-drag: a lands on b, b is pushed down into the gap at 21–22, so c stays.
	const current = [
		{ id: "a", slot: 26, span: 1 },
		{ id: "b", slot: 20, span: 1 },
		{ id: "c", slot: 23, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 21, span: 1 });
	assert.deepEqual(byId(out, "c"), { id: "c", slot: 23, span: 1 });
});

test("a grow that would force a block past the day end rejects whole", () => {
	// A drag can always slide displaced blocks into the slot it vacated, so only
	// a resize (which frees nothing) can still overflow an edge and reject.
	const current = [
		{ id: "a", slot: 30, span: 1 },
		{ id: "b", slot: 32, span: 2 }, // flush against end 34
	];
	assert.equal(pushLayout(bounds, current, { id: "a", slot: 30, span: 3 }), null);
});

test("the moved block's own run outside the day rejects", () => {
	const current = [{ id: "a", slot: 18, span: 2 }];
	assert.equal(pushLayout(bounds, current, { id: "a", slot: 17, span: 2 }), null);
	assert.equal(pushLayout(bounds, current, { id: "a", slot: 33, span: 2 }), null);
});

test("contiguous blocks chain-push together, up behind a down-drag", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 19, span: 1 },
		{ id: "c", slot: 20, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 18, span: 1 });
	assert.deepEqual(byId(out, "c"), { id: "c", slot: 19, span: 1 });
});

test("growing a span pushes the block below, same as a drag", () => {
	const current = [
		{ id: "a", slot: 20, span: 1 },
		{ id: "b", slot: 21, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 2 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 20, span: 2 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 22, span: 1 });
});

test("an up-drag pushes a straddling block fully below the drop run", () => {
	const current = [
		{ id: "a", slot: 30, span: 1 },
		{ id: "b", slot: 19, span: 2 }, // occupies 19–20; drop at 20 straddles
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 21, span: 2 });
});

test("an exact fit flush against the day end is accepted", () => {
	const current = [{ id: "a", slot: 18, span: 2 }];
	const out = pushLayout(bounds, current, { id: "a", slot: 32, span: 2 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 32, span: 2 });
});

test("identity placement is a no-op layout", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 20, span: 2 },
	];
	const out = pushLayout(bounds, current, { id: "b", slot: 20, span: 2 });
	assert.deepEqual(out, current);
});

test("dragging the top block to the bottom of a full column reorders it", () => {
	const full = { start: 0, end: 3 };
	const current = [
		{ id: "a", slot: 0, span: 1 },
		{ id: "b", slot: 1, span: 1 },
		{ id: "c", slot: 2, span: 1 },
	];
	const out = pushLayout(full, current, { id: "a", slot: 2, span: 1 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 2, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 0, span: 1 });
	assert.deepEqual(byId(out, "c"), { id: "c", slot: 1, span: 1 });
});

test("dragging the bottom block to the top of a full column reorders it", () => {
	const full = { start: 0, end: 3 };
	const current = [
		{ id: "a", slot: 0, span: 1 },
		{ id: "b", slot: 1, span: 1 },
		{ id: "c", slot: 2, span: 1 },
	];
	const out = pushLayout(full, current, { id: "c", slot: 0, span: 1 });
	assert.deepEqual(byId(out, "c"), { id: "c", slot: 0, span: 1 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 1, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 2, span: 1 });
});
