// Unit tests for the shared gesture commit path (gestures/commit.js): the
// pointer and keyboard paths both finish through commitGesture, so its guard
// order — same? → in-list? → write → mirrors → announce → dispatch — is the
// contract. Run: node --test internal/frontend/jstest
import test from "node:test";
import assert from "node:assert/strict";
import { commitGesture } from "../static/js/blocks/commit.js";
import { sameLayout } from "../static/js/blocks/grid.js";

// Minimal stand-ins for the few DOM surfaces commitGesture touches.
const blockEl = (id, slot, span) => ({
	classList: { contains: (c) => c === "block-item" },
	dataset: { id, slot: String(slot), span: String(span) },
	style: { setProperty(k, v) { this[k] = v; } },
	parentElement: null,
	grip: { attrs: {}, setAttribute(k, v) { this.attrs[k] = v; } },
	sr: { textContent: "" },
	querySelector(sel) {
		if (sel === ".grip") return this.grip;
		if (sel === ".sr-only") return this.sr;
		return null;
	},
});

const listOf = (...els) => {
	const list = {
		children: els,
		events: [],
		dispatchEvent(e) { this.events.push(e); },
	};
	for (const el of els) el.parentElement = list;
	return list;
};

const bounds = { start: 18, end: 34 };

test("an unchanged layout dispatches nothing and stays silent", () => {
	const a = blockEl("a", 18, 1);
	const list = listOf(a);
	const layout = [{ id: "a", slot: 18, span: 1 }];
	const said = [];
	const dispatched = commitGesture(list, {
		el: a, id: "a", from: layout, to: [{ ...layout[0] }], bounds,
		announce: (m) => said.push(m), say: "Dropped.", refocus: null,
	});
	assert.equal(dispatched, false);
	assert.equal(list.events.length, 0);
	assert.deepEqual(said, [], "a no-op gesture announces nothing");
});

test("a block that left the list dispatches nothing — the server won", () => {
	const a = blockEl("a", 18, 1);
	const list = listOf(a);
	a.parentElement = null; // foreign morph replaced it mid-gesture
	const dispatched = commitGesture(list, {
		el: a, id: "a",
		from: [{ id: "a", slot: 18, span: 1 }],
		to: [{ id: "a", slot: 20, span: 1 }],
		bounds, announce() {}, say: "Dropped.", refocus: null,
	});
	assert.equal(dispatched, false);
	assert.equal(list.events.length, 0);
});

// The old one-directional sameLayout returned true when `to` lost a block (and
// vacuously for []), silently suppressing the dispatch.
test("a layout that lost a block still dispatches", () => {
	const a = blockEl("a", 18, 1);
	const b = blockEl("b", 19, 1);
	const list = listOf(a, b);
	const dispatched = commitGesture(list, {
		el: a, id: "a",
		from: [{ id: "a", slot: 18, span: 1 }, { id: "b", slot: 19, span: 1 }],
		to: [{ id: "a", slot: 18, span: 1 }],
		bounds, announce() {}, say: null, refocus: null,
	});
	assert.equal(dispatched, true);
	assert.equal(list.events.length, 1);
});

test("a layout that gained a block still dispatches", () => {
	const a = blockEl("a", 18, 1);
	const list = listOf(a);
	const dispatched = commitGesture(list, {
		el: a, id: "a",
		from: [{ id: "a", slot: 18, span: 1 }],
		to: [{ id: "a", slot: 18, span: 1 }, { id: "b", slot: 19, span: 1 }],
		bounds, announce() {}, say: null, refocus: null,
	});
	assert.equal(dispatched, true);
});

test("a committed move writes the layout and dispatches it", () => {
	const a = blockEl("a", 18, 1);
	const b = blockEl("b", 19, 1);
	const list = listOf(a, b);
	const to = [{ id: "a", slot: 19, span: 1 }, { id: "b", slot: 18, span: 1 }];
	const said = [];
	const dispatched = commitGesture(list, {
		el: a, id: "a",
		from: [{ id: "a", slot: 18, span: 1 }, { id: "b", slot: 19, span: 1 }],
		to, bounds,
		announce: (m) => said.push(m), say: "Dropped, 9:30 to 10:00.", refocus: null,
	});
	assert.equal(dispatched, true);
	// The dataset stub doesn't coerce to string the way a real DOM does.
	assert.equal(String(a.dataset.slot), "19");
	assert.equal(String(b.dataset.slot), "18");
	assert.deepEqual(said, ["Dropped, 9:30 to 10:00."]);
	assert.equal(list.events.length, 1);
	assert.equal(list.events[0].type, "layout");
	assert.deepEqual(list.events[0].detail.layout, to);
});

test("the ARIA mirrors match the committed placement", () => {
	const a = blockEl("a", 18, 2);
	const list = listOf(a);
	commitGesture(list, {
		el: a, id: "a",
		from: [{ id: "a", slot: 18, span: 2 }],
		to: [{ id: "a", slot: 20, span: 3 }],
		bounds, announce() {}, say: null, refocus: null,
	});
	assert.equal(a.grip.attrs["aria-valuenow"], 3);
	assert.equal(a.grip.attrs["aria-valuemax"], bounds.end - 20, "valuemax tracks the committed slot");
	assert.equal(a.grip.attrs["aria-valuetext"], "10:00 to 11:30");
	assert.equal(a.sr.textContent, "10:00 to 11:30, ");
});

test("a missing element or grip never throws", () => {
	// `to` carries a block with no element in the list (SSE patch removed it).
	const a = blockEl("a", 18, 1);
	const list = listOf(a);
	assert.doesNotThrow(() => commitGesture(list, {
		el: a, id: "a",
		from: [{ id: "a", slot: 18, span: 1 }, { id: "ghost", slot: 20, span: 1 }],
		to: [{ id: "a", slot: 19, span: 1 }, { id: "ghost", slot: 21, span: 1 }],
		bounds, announce() {}, say: null, refocus: null,
	}));
	// The gesture's own block missing from `to` skips the mirrors, still commits.
	const c = blockEl("c", 18, 1);
	c.querySelector = () => null; // and no grip/.sr-only in the fragment at all
	const list2 = listOf(c);
	assert.doesNotThrow(() => commitGesture(list2, {
		el: c, id: "c",
		from: [{ id: "c", slot: 18, span: 1 }],
		to: [{ id: "other", slot: 19, span: 1 }],
		bounds, announce() {}, say: null, refocus: null,
	}));
});

// The fixed sameLayout: structural equality must be bidirectional.
test("sameLayout compares both directions", () => {
	const one = [{ id: "a", slot: 18, span: 1 }];
	const two = [{ id: "a", slot: 18, span: 1 }, { id: "b", slot: 19, span: 1 }];
	assert.equal(sameLayout(one, one), true);
	assert.equal(sameLayout(one, two), false, "gained a block");
	assert.equal(sameLayout(two, one), false, "lost a block");
	assert.equal(sameLayout([], one), false, "empty is not vacuously equal");
	assert.equal(sameLayout([], []), true);
	assert.equal(sameLayout(one, [{ id: "a", slot: 19, span: 1 }]), false);
});
