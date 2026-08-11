// Jotpad textarea wiring: Enter list continuation, plus the whole save loop —
// debounced compare-and-swap POSTs, retry with backoff, the save-state
// indicator, and teardown beacons — via the shared jot-sync driver (/jot
// answers JSON now, which Datastar's @post cannot read). Remote edits arrive
// through the driver too: a clean textarea is replaced wholesale; a dirty one
// waits for the merge response. Cursor preservation under remote edits is a
// CodeMirror-only feature by design (jot-cm.js).

import { createJotSync, wireTeardown } from "./jot-sync.js";

// A list marker at the start of a line: an indent, then either a bullet
// (optionally carrying a [ ] checkbox) or a number with . or ). One separating
// space is required — a bare dash is a sentence, not a list — and exactly one
// is consumed, so any further whitespace counts as the line's content and an
// "empty" marker line still reads as empty.
const MARKER = /^([ \t]*)(?:([-*+])[ \t](\[[ xX]\][ \t])?|(\d+)([.)])[ \t])/;

/**
 * Decides what Enter should do inside a Jotpad line, as a pure function of the
 * text and the caret. Returns the whole next {value, cursor}, or null when the
 * line isn't a list line and the browser's own Enter should stand.
 */
export function continueList(value, selectionStart, selectionEnd = selectionStart) {
	// A selection means Enter replaces it — the browser's job, not ours.
	if (selectionStart !== selectionEnd) return null;

	const lineStart = value.lastIndexOf("\n", selectionStart - 1) + 1;
	const nextBreak = value.indexOf("\n", selectionStart);
	const lineEnd = nextBreak === -1 ? value.length : nextBreak;
	const line = value.slice(lineStart, lineEnd);

	const m = MARKER.exec(line);
	if (!m) return null;
	// A caret inside the marker means the marker is what's being edited.
	if (selectionStart < lineStart + m[0].length) return null;

	// A marker with nothing but whitespace after it: Enter clears the line.
	if (line.slice(m[0].length).trim() === "") {
		return {
			value: value.slice(0, lineStart) + value.slice(lineEnd),
			cursor: lineStart,
		};
	}

	const [, indent, bullet, checkbox, number, delim] = m;
	const marker = bullet
		? bullet + " " + (checkbox ? "[ ] " : "")
		: Number(number) + 1 + delim + " ";
	const insert = "\n" + indent + marker;
	return {
		value: value.slice(0, selectionStart) + insert + value.slice(selectionStart),
		cursor: selectionStart + insert.length,
	};
}

/** Longest common prefix length of two strings. */
function commonPrefix(a, b) {
	let i = 0;
	while (i < a.length && i < b.length && a[i] === b[i]) i++;
	return i;
}

// Applies the continuation as the minimal edit, through execCommand: it is the
// only path that keeps the browser's own undo stack (the Jotpad has none of its
// own) and the only one that fires a native `input` event, which is what marks
// the pad dirty for the sync driver.
function applyEdit(el, next) {
	const before = el.value;
	const p = commonPrefix(before, next.value);
	let s = 0;
	while (
		s < before.length - p &&
		s < next.value.length - p &&
		before[before.length - 1 - s] === next.value[next.value.length - 1 - s]
	) {
		s++;
	}
	const text = next.value.slice(p, next.value.length - s);
	el.setSelectionRange(p, before.length - s);
	// insertText with "" is a no-op in some engines, so a pure deletion (an
	// Enter that clears an empty marker line) goes through delete instead.
	if (text) document.execCommand("insertText", false, text);
	else document.execCommand("delete");
	el.setSelectionRange(next.cursor, next.cursor);
}

/**
 * Wires one Jotpad textarea. opts is {version, status} from the server render.
 */
export function initJotpad(el, postURL, opts = {}) {
	const sync = createJotSync({
		getText: () => el.value,
		// Wholesale replacement, only ever reached when the textarea is clean
		// (the driver buffers remote events while local typing is unsaved).
		applyText: (text) => {
			el.value = text;
		},
		postURL,
		version: opts.version ?? 0,
		status: opts.status,
	});

	// The SSE stream hands (version, text) here via data-on-signal-patch.
	window.__jotRemote = (v, text) => sync.remote(v, text);

	el.addEventListener("input", sync.edited);

	el.addEventListener("keydown", (e) => {
		if (e.key !== "Enter" || e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return;
		if (e.isComposing) return;
		const next = continueList(el.value, el.selectionStart, el.selectionEnd);
		if (!next) return;
		e.preventDefault();
		applyEdit(el, next);
	});

	wireTeardown(sync, el);
}
