// The Jotpad sync decision logic, as pure functions (see jot/sync.js). The
// fetch/timer driver around them is exercised in the browser; what node pins
// is convergence: which remote events apply, which buffer, which are echoes.

import test from "node:test";
import assert from "node:assert/strict";

import {
	remoteAction,
	ackAction,
	minimalEdit,
	retryDelay,
	statusOf,
} from "./sync.js";

const clean = (over = {}) => ({
	version: 5,
	text: "shared text",
	dirty: false,
	inflight: false,
	...over,
});

test("a stale or same-version remote event is ignored", () => {
	assert.equal(remoteAction(clean(), { version: 4, text: "old" }), "ignore");
	assert.equal(remoteAction(clean(), { version: 5, text: "same-version" }), "ignore");
});

test("our own echo is ignored by the version guard alone", () => {
	// A device adopts the version returned by its own POST before the fan-out
	// arrives, so the echo shows up as version <= local — no flag passing.
	assert.equal(remoteAction(clean({ version: 6 }), { version: 6, text: "typed here" }), "ignore");
});

test("a newer remote with identical text adopts the version without editing", () => {
	assert.equal(remoteAction(clean(), { version: 6, text: "shared text" }), "adopt");
});

test("a newer remote applies to a clean editor", () => {
	assert.equal(remoteAction(clean(), { version: 6, text: "typed elsewhere" }), "apply");
});

test("a newer remote buffers while local typing is unsaved or in flight", () => {
	// One convergence path: the pending POST's merge response resolves both.
	assert.equal(
		remoteAction(clean({ dirty: true }), { version: 6, text: "typed elsewhere" }),
		"buffer",
	);
	assert.equal(
		remoteAction(clean({ inflight: true }), { version: 6, text: "typed elsewhere" }),
		"buffer",
	);
});

test("an ack without text means the write was taken verbatim", () => {
	assert.deepEqual(ackAction({ version: 7 }), { version: 7, apply: null });
});

test("an ack with text means a merge happened and the editor must snap to it", () => {
	assert.deepEqual(ackAction({ version: 7, text: "merged" }), { version: 7, apply: "merged" });
});

test("minimalEdit finds the smallest single replacement", () => {
	assert.equal(minimalEdit("same", "same"), null);
	assert.deepEqual(minimalEdit("a cat sat", "a dog sat"), { from: 2, to: 5, insert: "dog" });
	assert.deepEqual(minimalEdit("abc", "abXc"), { from: 2, to: 2, insert: "X" });
	assert.deepEqual(minimalEdit("abXc", "abc"), { from: 2, to: 3, insert: "" });
	assert.deepEqual(minimalEdit("", "new"), { from: 0, to: 0, insert: "new" });
	assert.deepEqual(minimalEdit("old", ""), { from: 0, to: 3, insert: "" });
});

test("minimalEdit round-trips: applying the edit yields the new text", () => {
	const cases = [
		["## Monday\n- milk\n", "## Monday\n- milk\n- eggs\n"],
		["aaaa", "aa"],
		["xyx", "xx"],
		["abab", "ab"],
	];
	for (const [oldText, newText] of cases) {
		const e = minimalEdit(oldText, newText);
		const applied = oldText.slice(0, e.from) + e.insert + oldText.slice(e.to);
		assert.equal(applied, newText, `${oldText} -> ${newText}`);
	}
});

test("retryDelay walks 2s, 4s, 8s… and caps at 30s", () => {
	assert.equal(retryDelay(1), 2000);
	assert.equal(retryDelay(2), 4000);
	assert.equal(retryDelay(3), 8000);
	assert.equal(retryDelay(10), 30000);
});

test("statusOf tells the truth: offline beats saving beats saved", () => {
	assert.equal(statusOf({ offline: true, dirty: true, inflight: false }), "offline");
	assert.equal(statusOf({ offline: false, dirty: true, inflight: false }), "saving");
	assert.equal(statusOf({ offline: false, dirty: false, inflight: true }), "saving");
	assert.equal(statusOf({ offline: false, dirty: false, inflight: false }), "saved");
});
