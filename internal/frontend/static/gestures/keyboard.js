// Keyboard gestures for the #block-list day grid: grab→move→drop, splitter
// resize, delete, and F2 rename. Each step is optimistic + DOM-only and
// recomputes the cascade from the grab-START layout; a drop/commit dispatches
// one `layout` (or `delete`) event. Perceivability rides #sr-announce via
// ctx.announce — the wording itself lives in keyboard-reducer.js.
//
// Arbitration: keydown bails while the pointer path is mid-gesture or settling
// (arb.pointer.isActive()); a pointerdown cancels any active keyboard gesture
// via cancel().

import { enterEdit, restoreFocusAfterMorph } from "./rename.js";
import { blocksIn, boundsNow, layoutIn, writeLayout, sameLayout } from "./grid.js";
import {
	keyboardLayout,
	normalizeKey,
	timeRange,
	grabbedMsg,
	rangeMsg,
	droppedMsg,
	resizedMsg,
	deletedMsg,
	moveCancelledMsg,
	resizeCancelledMsg,
	blockedMsg,
} from "../keyboard-reducer.js";

let list; // #block-list, injected via init
let announce; // ctx.announce (msg) => void
let arb; // cross-gesture arbitration handle ({ keyboard, pointer })
let grab = null; // active keyboard move (grab → arrow → drop), null when idle
let kresize = null; // active keyboard resize on a focused grip, null when idle

export function isActive() {
	return grab !== null || kresize !== null;
}

// Revert any active keyboard gesture to its grab-START layout. Called when a
// pointer gesture supersedes this path.
export function cancel() {
	if (grab) cancelGrab();
	if (kresize) cancelKbResize();
}

export function init(ctx, arbitration) {
	list = ctx.list;
	announce = ctx.announce;
	arb = arbitration;
	list.addEventListener("keydown", onKeydown);
	list.addEventListener("focusout", onFocusout);
}

// valuenow is the span in slots, valuetext the spoken clock range.
function updateGripValue(grip, id, layout) {
	const p = layout.find((q) => q.id === id);
	grip.setAttribute("aria-valuenow", p.span);
	grip.setAttribute("aria-valuetext", timeRange(p.slot, p.span));
}

function onKeydown(e) {
	if (arb.pointer?.isActive()) return;
	const grip = e.target.closest(".grip");
	if (grip && grip.closest(".block-item")?.parentElement === list) {
		handleResizeKey(e, grip);
		return;
	}
	const el = e.target.closest(".block-item");
	if (!el || el.parentElement !== list || e.target !== el) return;
	if (!grab) {
		if (e.key === " " || e.key === "Enter") {
			e.preventDefault();
			startGrab(el);
		} else if (e.key === "F2" || e.key === "r") {
			// F2 is the established rename-in-place key; `r` is the vim-flavored
			// alias. Enter is taken by grab/drop.
			e.preventDefault();
			const label = el.querySelector(".block-label");
			if (label) enterEdit(label, undefined, undefined, list);
		} else if (e.key === "Backspace" || e.key === "Delete" || e.key === "d") {
			// Calendar/canvas convention; `d` is the vim alias. Immediate, no confirm
			// — the server re-render is the only safety net, like the mouse control.
			e.preventDefault();
			deleteBlock(el);
		}
		return;
	}
	const moveKey = normalizeKey(e.key);
	if (moveKey === "ArrowUp" || moveKey === "ArrowDown") {
		e.preventDefault();
		moveGrab(e.key);
	} else if (e.key === " " || e.key === "Enter") {
		e.preventDefault();
		dropGrab();
	} else if (e.key === "Escape") {
		e.preventDefault();
		cancelGrab();
	}
}

// Focus leaving a grabbed block abandons the move; focus leaving a resizing
// grip COMMITS (the splitter convention — blur saves).
function onFocusout(e) {
	if (grab && e.target === grab.el) cancelGrab();
	if (kresize && e.target === kresize.grip) commitKbResize(false);
}

// ---- keyboard move (grab → move → drop) ------------------------------
//
// Space/Enter grabs, Up/Down move one slot (optimistic, DOM-only), Space/Enter
// drops with one `layout` event, Escape cancels. The rbd/dnd-kit convention;
// perceivability is carried by #sr-announce, not aria-grabbed.

const labelOf = (el) => {
	const l = el.querySelector(".block-label");
	return (l && l.textContent.trim()) || "Block";
};

function startGrab(el) {
	const start = layoutIn(list);
	const p = start.find((q) => q.id === el.dataset.id);
	// `start` is the immutable grab-origin layout each step recomputes from;
	// `slot` is the running target, playing the pointer's role.
	grab = { el, id: el.dataset.id, bounds: boundsNow(list), start, layout: start, slot: p.slot };
	el.classList.add("dragging");
	announce(grabbedMsg(labelOf(el), p.slot));
}

// One arrow step: recompute the cascade from grab.start (never the running
// layout, so displacements don't accumulate) and write it straight to the DOM.
function moveGrab(key) {
	const res = keyboardLayout(grab.bounds, grab.start, { id: grab.id, mode: "move", slot: grab.slot }, key);
	if (!res) return;
	if (res.kind === "blocked") {
		announce(blockedMsg("move", normalizeKey(key)));
		return;
	}
	grab.slot = res.slot;
	grab.layout = res.layout;
	writeLayout(list, grab.layout, grab.bounds.start);
	grab.el.scrollIntoView({ block: "nearest" });
	announce(rangeMsg(grab.id, grab.layout));
}

// Drop dispatches the same `layout` event a drag does (if anything moved).
function dropGrab() {
	const g = grab;
	grab = null;
	g.el.classList.remove("dragging");
	announce(droppedMsg(g.id, g.layout));
	if (sameLayout(g.layout, g.start) || g.el.parentElement !== list) return;
	restoreFocusAfterMorph(list, () => document.getElementById(g.id));
	list.dispatchEvent(new CustomEvent("layout", { detail: { layout: g.layout } }));
}

function cancelGrab() {
	const g = grab;
	grab = null;
	g.el.classList.remove("dragging");
	writeLayout(list, g.start, g.bounds.start);
	announce(moveCancelledMsg());
}

// ---- keyboard delete -------------------------------------------------
//
// Delete/Backspace/d on a focused block: dispatch the same `delete` event the
// per-block delete button posts, and steer focus to a neighbouring block after
// the commit morph so the keyboard user isn't stranded on <body>.
function deleteBlock(el) {
	const blocks = blocksIn(list);
	const i = blocks.indexOf(el);
	const neighbor = blocks[i + 1] || blocks[i - 1] || null;
	announce(deletedMsg(labelOf(el)));
	if (neighbor)
		restoreFocusAfterMorph(list, () => document.getElementById(neighbor.dataset.id));
	list.dispatchEvent(new CustomEvent("delete", { detail: { id: el.dataset.id } }));
}

// ---- keyboard resize (APG Window Splitter) ---------------------------
//
// The grip is role="separator"; Up/Down grow/shrink one slot, Home/End jump to
// min/max span, all optimistic + DOM-only. Enter or blur commits one `layout`
// event; Escape reverts. Each step recomputes from the grab-START layout, so
// shrinking undoes a grow's compression.

function handleResizeKey(e, grip) {
	if (["ArrowUp", "ArrowDown", "Home", "End"].includes(normalizeKey(e.key))) {
		e.preventDefault();
		if (!kresize) startKbResize(grip);
		stepKbResize(e.key);
	} else if (e.key === "Enter") {
		e.preventDefault();
		if (kresize) commitKbResize(true);
	} else if (e.key === "Escape") {
		e.preventDefault();
		if (kresize) cancelKbResize();
	}
}

function startKbResize(grip) {
	const el = grip.closest(".block-item");
	const start = layoutIn(list);
	const p = start.find((q) => q.id === el.dataset.id);
	kresize = { el, grip, id: el.dataset.id, bounds: boundsNow(list), start, layout: start, span: p.span };
	el.classList.add("resizing");
}

function stepKbResize(key) {
	const r = kresize;
	const res = keyboardLayout(r.bounds, r.start, { id: r.id, mode: "resize", span: r.span }, key);
	if (!res) return;
	if (res.kind === "blocked") {
		announce(blockedMsg("resize", normalizeKey(key)));
		return;
	}
	r.span = res.span;
	r.layout = res.layout;
	writeLayout(list, r.layout, r.bounds.start);
	updateGripValue(r.grip, r.id, r.layout);
	r.grip.scrollIntoView({ block: "nearest" });
	announce(rangeMsg(r.id, r.layout));
}

// `refocus` is true on Enter (steer focus back across the morph) and false on
// blur (the user Tabbed on — leave focus where it went).
function commitKbResize(refocus) {
	const r = kresize;
	kresize = null;
	r.el.classList.remove("resizing");
	if (sameLayout(r.layout, r.start) || r.el.parentElement !== list) return;
	announce(resizedMsg(r.id, r.layout));
	if (refocus)
		restoreFocusAfterMorph(list, () => document.getElementById(r.id)?.querySelector(".grip"));
	list.dispatchEvent(new CustomEvent("layout", { detail: { layout: r.layout } }));
}

function cancelKbResize() {
	const r = kresize;
	kresize = null;
	r.el.classList.remove("resizing");
	writeLayout(list, r.start, r.bounds.start);
	updateGripValue(r.grip, r.id, r.start);
	announce(resizeCancelledMsg());
}
