// Contract test for the cross-gesture arbitration seam: each path's init must
// return a live { isActive, cancel } handle, or every arbitration guard silently
// no-ops (the UNB-26 regression). Only the keyboard path is reachable here —
// pointer.js imports Motion over https, which node can't resolve, so its
// contract stays covered by /verify. Run: node --test internal/frontend/jstest
import test from "node:test";
import assert from "node:assert/strict";
import { init } from "../static/gestures/keyboard.js";

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
