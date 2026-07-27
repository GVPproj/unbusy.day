// Jotpad editing assistance and save flushes. The normal save path is pure
// Datastar (data-bind:_jot + a debounced @post, see components/jotpad.templ);
// this adds the two things it can't express — Enter list continuation, and
// flushing the pending debounce when the page is about to go away.

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
// own) and the only one that fires a native `input` event, which is what tells
// data-bind:_jot the text changed at all.
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
 * Wires one Jotpad textarea. postURL receives the current text on every flush.
 */
export function initJotpad(el, postURL) {
	el.addEventListener("keydown", (e) => {
		if (e.key !== "Enter" || e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return;
		if (e.isComposing) return;
		const next = continueList(el.value, el.selectionStart, el.selectionEnd);
		if (!next) return;
		e.preventDefault();
		applyEdit(el, next);
	});

	// Flush the pending 1s debounce whenever the page might not get another
	// tick. sendBeacon rather than fetch: a request started during teardown is
	// routinely cancelled, and an iOS PWA (ADR 0010) is suspended without a
	// clean unload — exactly where a paragraph would otherwise be lost.
	// Deliberately fail-safe: `sent` tracks only our own beacons, so a flush
	// after a debounced @post repeats it. One duplicate write beats a lost one;
	// the guard just stops blur → hidden → pagehide firing three times over.
	let sent = el.value;
	const flush = () => {
		if (el.value === sent) return;
		sent = el.value;
		// The Blob's type sets the Content-Type; /jot reads the body as JSON.
		const body = new Blob([JSON.stringify({ _jot: sent })], {
			type: "application/json",
		});
		navigator.sendBeacon(postURL, body);
	};

	el.addEventListener("blur", flush);
	window.addEventListener("pagehide", flush);
	document.addEventListener("visibilitychange", () => {
		if (document.hidden) flush();
	});
}
