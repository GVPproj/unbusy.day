// Unit tests for the pure push cascade (ADR 0005). Run: node --test frontend/jstest
import test from "node:test";
import assert from "node:assert/strict";
import { pushLayout } from "../static/push.js";

const bounds = { start: 18, end: 34 }; // 9:00–17:00, end-exclusive

const byId = (layout, id) => layout.find((p) => p.id === id);

test("move into empty slots places the card, others untouched", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 19, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 25, span: 1 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 25, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 19, span: 1 });
	assert.equal(out.length, 2);
});

test("drop onto an occupied slot pushes the overlapped card down", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 20, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 21, span: 1 });
});

test("a gap absorbs the push so cards past the gap stay put", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 20, span: 1 },
		{ id: "c", slot: 23, span: 1 }, // gap at 21–22 absorbs b's displacement
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 21, span: 1 });
	assert.deepEqual(byId(out, "c"), { id: "c", slot: 23, span: 1 });
});

test("a push that would force a card past the day end rejects whole", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 32, span: 2 }, // flush against end 34
	];
	assert.equal(pushLayout(bounds, current, { id: "a", slot: 33, span: 1 }), null);
});

test("the moved card's own run outside the day rejects", () => {
	const current = [{ id: "a", slot: 18, span: 2 }];
	assert.equal(pushLayout(bounds, current, { id: "a", slot: 17, span: 2 }), null);
	assert.equal(pushLayout(bounds, current, { id: "a", slot: 33, span: 2 }), null);
});

test("contiguous cards chain-push together", () => {
	const current = [
		{ id: "a", slot: 18, span: 1 },
		{ id: "b", slot: 20, span: 2 },
		{ id: "c", slot: 22, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 21, span: 1 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 22, span: 2 });
	assert.deepEqual(byId(out, "c"), { id: "c", slot: 24, span: 1 });
});

test("growing a span pushes the card below, same as a drag", () => {
	const current = [
		{ id: "a", slot: 20, span: 1 },
		{ id: "b", slot: 21, span: 1 },
	];
	const out = pushLayout(bounds, current, { id: "a", slot: 20, span: 2 });
	assert.deepEqual(byId(out, "a"), { id: "a", slot: 20, span: 2 });
	assert.deepEqual(byId(out, "b"), { id: "b", slot: 22, span: 1 });
});

test("a card straddling the drop run is pushed fully below it", () => {
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
