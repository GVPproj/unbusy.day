// Unit tests for the Jotpad's Enter list continuation, the only editing
// assistance the Jotpad offers. Run: node --test internal/frontend/jstest
import test from "node:test";
import assert from "node:assert/strict";
import { continueList } from "../static/jot.js";

// Convenience: mark the caret with | in a template so cases read as text.
const at = (marked) => ({
	value: marked.replace("|", ""),
	cursor: marked.indexOf("|"),
});

const run = (marked) => {
	const { value, cursor } = at(marked);
	return continueList(value, cursor);
};

test("a non-list line is left to the browser's own Enter", () => {
	assert.equal(run("just a thought|"), null);
	assert.equal(run("|"), null);
	assert.equal(run("a\nb|\nc"), null);
});

test("Enter at the end of a bullet line repeats the marker", () => {
	assert.deepEqual(run("- milk|"), { value: "- milk\n- ", cursor: 9 });
});

test("the other bullet markers continue too", () => {
	assert.deepEqual(run("* milk|"), { value: "* milk\n* ", cursor: 9 });
	assert.deepEqual(run("+ milk|"), { value: "+ milk\n+ ", cursor: 9 });
});

test("indentation is carried onto the continued line", () => {
	assert.deepEqual(run("    - milk|"), {
		value: "    - milk\n    - ",
		cursor: 17,
	});
});

test("an ordered marker continues with the next number", () => {
	assert.deepEqual(run("1. first|"), { value: "1. first\n2. ", cursor: 12 });
	assert.deepEqual(run("9. ninth|"), { value: "9. ninth\n10. ", cursor: 13 });
});

test("the ) form of an ordered marker is kept", () => {
	assert.deepEqual(run("3) third|"), { value: "3) third\n4) ", cursor: 12 });
});

// A checkbox is plain text here — nothing renders it — but continuing one is
// what a scratchpad list is for. A ticked box continues as a fresh empty one.
test("a checkbox marker continues as a fresh unticked box", () => {
	assert.deepEqual(run("- [ ] wash up|"), {
		value: "- [ ] wash up\n- [ ] ",
		cursor: 20,
	});
	assert.deepEqual(run("- [x] wash up|"), {
		value: "- [x] wash up\n- [ ] ",
		cursor: 20,
	});
});

test("Enter on an otherwise empty marker line clears the line", () => {
	assert.deepEqual(run("- milk\n- |"), { value: "- milk\n", cursor: 7 });
});

test("clearing an empty marker line takes its indentation with it", () => {
	assert.deepEqual(run("  - milk\n  - |"), { value: "  - milk\n", cursor: 9 });
});

test("an empty marker line with trailing spaces still clears", () => {
	assert.deepEqual(run("- |  "), { value: "", cursor: 0 });
	assert.deepEqual(run("1. |"), { value: "", cursor: 0 });
	assert.deepEqual(run("- [ ] |"), { value: "", cursor: 0 });
});

test("continuation works mid-document, not just on the last line", () => {
	assert.deepEqual(run("intro\n- milk|\noutro"), {
		value: "intro\n- milk\n- \noutro",
		cursor: 15,
	});
});

test("Enter mid-line splits it and carries the marker to the remainder", () => {
	assert.deepEqual(run("- milk |and eggs"), {
		value: "- milk \n- and eggs",
		cursor: 10,
	});
});

// The caret sitting inside the marker means the user is editing the marker,
// not adding an item; the browser's plain newline is the right behaviour.
test("a caret inside the marker gets no assistance", () => {
	assert.equal(run("-| milk"), null);
	assert.equal(run("|- milk"), null);
	assert.equal(run("1|. first"), null);
});

// A marker needs its trailing space; a bare dash is a sentence, not a list.
test("a bare marker with no trailing space is not a list line", () => {
	assert.equal(run("-milk|"), null);
	assert.equal(run("well - not a list|"), null);
});

// A selection is a replace-the-selection Enter; that is the browser's job.
test("no assistance when there is a selection", () => {
	assert.equal(continueList("- milk", 2, 5), null);
});
