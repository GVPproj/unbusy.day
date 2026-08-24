import assert from "node:assert/strict";
import test from "node:test";

import { readVimPreference, writeVimPreference } from "./preference.js";

function storageWith(value) {
	return {
		getItem(key) {
			assert.equal(key, "unbusy:jot-vim");
			return value;
		},
		setItem() {},
	};
}

test("Vim preference is disabled when storage has no value", () => {
	assert.equal(readVimPreference(storageWith(null)), false);
});

test("Vim preference reads only the exact enabled representation", () => {
	assert.equal(readVimPreference(storageWith("true")), true);
	assert.equal(readVimPreference(storageWith("false")), false);
	for (const malformed of ["TRUE", "1", "yes", "", "null"]) {
		assert.equal(readVimPreference(storageWith(malformed)), false);
	}
});

test("Vim preference is disabled when storage cannot be read", () => {
	const storage = {
		getItem() {
			throw new Error("blocked");
		},
	};
	assert.equal(readVimPreference(storage), false);
});

test("Vim preference writes exact boolean strings", () => {
	const writes = [];
	const storage = {
		setItem(key, value) {
			writes.push([key, value]);
		},
	};
	assert.equal(writeVimPreference(storage, true), true);
	assert.equal(writeVimPreference(storage, false), true);
	assert.deepEqual(writes, [
		["unbusy:jot-vim", "true"],
		["unbusy:jot-vim", "false"],
	]);
});

test("a blocked preference write does not escape", () => {
	const storage = {
		setItem() {
			throw new Error("blocked");
		},
	};
	assert.equal(writeVimPreference(storage, true), false);
});
