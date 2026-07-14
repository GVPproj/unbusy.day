// Inline label editor shared by both gesture paths — a pointer tap on the label
// (settleDrag) and the keyboard F2/r shortcut both route through enterEdit. Owns
// caret placement and the post-morph focus-steer so a keyboard rename (which
// fires a server re-render morph) lands focus back on the block instead of
// stranding it on <body>.

// Turn a label into a plaintext editor: commit on blur or Enter, revert on
// Escape. A pointer tap passes the click point (x, y); keyboard entry passes
// none — caret to the end, focus steered back to the block across the morph.
// `list` leads (required context) ahead of the optional coordinates.
export function enterEdit(list, label, x, y) {
	const block = label.closest(".block-item");
	if (!block) return; // a morph detached the label during the settle await
	const id = block.dataset.id;
	const byKeyboard = x === undefined;
	const original = label.textContent;
	label.contentEditable = "plaintext-only";
	label.focus();
	if (byKeyboard) caretToEnd(label);
	else placeCaret(x, y);

	let done = false;
	const finish = (save) => {
		if (done) return;
		done = true;
		label.removeAttribute("contenteditable");
		label.removeEventListener("keydown", onKey);
		label.removeEventListener("blur", onBlur);
		const next = label.textContent.trim();
		if (!save || next === "" || next === original.trim()) {
			label.textContent = original; // morph will re-assert anyway
			if (byKeyboard) block.focus(); // no commit morph coming — refocus directly
			return;
		}
		// Keyboard rename triggers a morph; steer focus back to the block after it.
		if (byKeyboard) restoreFocusAfterMorph(list, () => document.getElementById(id));
		list.dispatchEvent(
			new CustomEvent("rename", { detail: { id, label: next } }),
		);
	};
	const onKey = (e) => {
		if (e.key === "Enter") {
			e.preventDefault();
			finish(true);
		} else if (e.key === "Escape") {
			e.preventDefault();
			finish(false);
		}
	};
	const onBlur = () => finish(true);
	label.addEventListener("keydown", onKey);
	label.addEventListener("blur", onBlur);
}

function caretToEnd(label) {
	const sel = getSelection();
	const range = document.createRange();
	range.selectNodeContents(label);
	range.collapse(false);
	sel.removeAllRanges();
	sel.addRange(range);
}

// Drop the caret at the click point.
function placeCaret(x, y) {
	const sel = getSelection();
	let range = null;
	if (document.caretPositionFromPoint) {
		const p = document.caretPositionFromPoint(x, y);
		if (p) {
			range = document.createRange();
			range.setStart(p.offsetNode, p.offset);
			range.collapse(true);
		}
	} else {
		// Older Safari lacks caretPositionFromPoint; the dynamic key keeps TS from
		// resolving the deprecated caretRangeFromPoint member.
		const key = /** @type {string} */ ("caretRangeFromPoint");
		const fromPoint = document[key];
		if (fromPoint) range = fromPoint.call(document, x, y);
	}
	if (range) {
		sel.removeAllRanges();
		sel.addRange(range);
	}
}

// After a commit morph, return focus to the acted-on target. idiomorph may
// preserve the element and keep focus, or replace it and drop focus to <body>;
// refocus only in the latter case so we never yank focus the user moved
// elsewhere. Bounded so the observer can't leak.
export function restoreFocusAfterMorph(list, resolve) {
	const refocus = () => {
		const a = document.activeElement;
		if (a && a !== document.body) return;
		const el = resolve();
		if (el) el.focus();
	};
	const obs = new MutationObserver(refocus);
	obs.observe(list, { childList: true, subtree: true });
	setTimeout(() => obs.disconnect(), 1000);
}
