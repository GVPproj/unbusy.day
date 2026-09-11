// Guard against UNB-26: keyboard init must return a live { isActive, cancel }
// handle so cross-gesture arbitration cannot silently no-op.
import test from "node:test";
import assert from "node:assert/strict";
import { init } from "./keyboard.js";

// init only wires listeners on `list` and stores ctx refs, so a stub element is
// enough to exercise the contract without a DOM.
const fakeList = () => ({ addEventListener() {} });

test("keyboard init returns a live arbitration handle", () => {
	const handle = init({ list: fakeList(), announce() {} }, {});
	assert.equal(typeof handle.isActive, "function", "handle exposes isActive()");
	assert.equal(typeof handle.cancel, "function", "handle exposes cancel()");
});

test("a freshly-inited keyboard path is idle", () => {
	const handle = init({ list: fakeList(), announce() {} }, {});
	assert.equal(handle.isActive(), false);
});
