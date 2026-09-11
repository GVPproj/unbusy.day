// Pointer drag/resize: CSS interpolates preview targets; JavaScript owns input,
// push decisions, authoritative morph arbitration, and the final FLIP commit.

import { pushLayout } from "./push.js";
import { droppedMsg, resizedMsg } from "./keyboard-reducer.js";
import { enterEdit } from "./rename.js";
import { commitGesture } from "./commit.js";
import { waitForTransitions } from "../transitions.js";
import {
	blocksIn,
	placementOf,
	layoutIn,
	boundsNow,
	slotPitch,
	writeLayout,
	sameLayout,
} from "./grid.js";

const reduceMotion = matchMedia("(prefers-reduced-motion: reduce)");
const TAP_SLOP = 4;
const EDGE = 48;
const MAX_SPEED = 16;

let list;
let announce;
let arb;
let drag = null;
let resize = null;
let settling = false;
let settleGesture = null;

function isActive() {
	return drag !== null || resize !== null || settling;
}

function cancel() {
	if (drag) {
		const d = drag;
		drag = null;
		if (d.raf) cancelAnimationFrame(d.raf);
		tearDown(d, d.current);
	}
	if (resize) {
		const r = resize;
		resize = null;
		tearDown(r, r.current);
	}
	if (settleGesture) abortForServer(settleGesture);
}

export function init(ctx, arbitration) {
	list = ctx.list;
	announce = ctx.announce;
	arb = arbitration;
	list.addEventListener("pointerdown", onPointerdown);
	list.addEventListener("pointermove", onPointermove);
	list.addEventListener("pointerup", onPointerup);
	list.addEventListener("pointercancel", onPointercancel);
	return { isActive, cancel };
}

function onPointerdown(e) {
	if (drag || resize || settling || e.button !== 0) return;
	arb.keyboard?.cancel();
	if (e.target.closest(".block-label[contenteditable]")) return;
	const el = e.target.closest(".block-item");
	if (!el || el.parentElement !== list) return;
	e.preventDefault();
	el.setPointerCapture(e.pointerId);
	if (e.target.closest(".grip")) startResize(e, el);
	else startDrag(e, el);
}

function onPointermove(e) {
	if (drag && e.pointerId === drag.pointerId) {
		drag.lastX = e.clientX;
		drag.lastY = e.clientY;
		applyDrag();
		autoScroll(e.clientY);
	} else if (resize && e.pointerId === resize.pointerId) {
		previewResize(
			resize.orig.span + Math.round((e.clientY - resize.startY) / resize.pitch),
		);
	}
}

function onPointerup(e) {
	settleDrag(e, true);
	settleResize(e, true);
}

function onPointercancel(e) {
	settleDrag(e, false);
	settleResize(e, false);
}

function watchServer(gesture) {
	gesture.serverChanged = false;
	gesture.aborted = false;
	gesture.onPatch = (event) => {
		if (event.detail.type !== "datastar-patch-elements") return;
		const elements = event.detail.argsRaw.elements;
		if (typeof elements !== "string") return;
		const patch = document.createElement("template");
		patch.innerHTML = elements;
		if (patch.content.querySelector("#block-list")) abortForServer(gesture);
	};
	// Column patches carry #block-list; capture runs before Datastar's morph
	// watcher, even when the incoming placement is identical to our snapshot.
	document.addEventListener("datastar-fetch", gesture.onPatch, true);
	gesture.morphs = new MutationObserver((records) => {
		if (records.some(isLayoutMutation)) abortForServer(gesture);
	});
	gesture.morphs.observe(list, {
		subtree: true,
		childList: true,
		attributes: true,
		attributeFilter: ["data-slot", "data-span", "data-day-start", "data-day-end"],
	});
}

function isLayoutMutation(record) {
	if (record.type === "attributes") return true;
	return [...record.addedNodes, ...record.removedNodes].some((node) =>
		node.nodeType === Node.ELEMENT_NODE &&
		(node.matches(".block-item") || node.querySelector(".block-item")),
	);
}

function abortForServer(gesture) {
	if (gesture.aborted) return;
	gesture.serverChanged = true;
	gesture.aborted = true;
	if (drag === gesture) {
		drag = null;
		if (gesture.raf) cancelAnimationFrame(gesture.raf);
	}
	if (resize === gesture) resize = null;
	if (settleGesture === gesture) {
		settleGesture = null;
		settling = false;
	}
	tearDown(gesture, gesture.current);
}

function siblingsFor(el, pitch) {
	const siblings = new Map();
	for (const block of blocksIn(list)) {
		if (block === el) continue;
		const fromSpan = parseInt(block.dataset.span, 10) || 1;
		const naturalHeight = block.getBoundingClientRect().height;
		block.style.height = `${naturalHeight}px`;
		block.classList.add("gesture-preview");
		siblings.set(block, { margin: fromSpan * pitch - naturalHeight });
	}
	return siblings;
}

function moveSiblings(gesture, layout) {
	const byID = new Map(layout.map((placement) => [placement.id, placement]));
	gesture.siblings.forEach((preview, block) => {
		const placement = byID.get(block.dataset.id);
		if (!placement) return;
		const fromSlot = parseInt(block.dataset.slot, 10);
		block.style.transform = `translateY(${(placement.slot - fromSlot) * gesture.pitch}px)`;
		block.style.height = `${placement.span * gesture.pitch - preview.margin}px`;
	});
}

function gestureElements(gesture) {
	return [gesture.el, ...gesture.siblings.keys()];
}

// Replace transient pixels with grid placement only while this gesture still
// owns the start snapshot. A server morph always wins, even for a keyed block.
function tearDown(g, layout) {
	const changed = g.serverChanged || g.morphs.takeRecords().some(isLayoutMutation);
	g.morphs.disconnect();
	document.removeEventListener("datastar-fetch", g.onPatch, true);
	const owned = !changed && g.el.parentElement === list &&
		sameLayout(layoutIn(list), g.current);
	const els = gestureElements(g);
	for (const el of els) el.classList.add("gesture-flip");
	if (owned) writeLayout(list, layout, g.bounds.start);
	for (const [block] of g.siblings) {
		block.style.transform = "";
		block.style.height = "";
		block.classList.remove("gesture-preview");
	}
	g.el.style.transform = "";
	g.el.style.height = "";
	g.el.classList.remove("dragging", "resizing", "gesture-preview", "gesture-settling");
	// Force the transform-to-grid swap to paint without starting a return ease.
	void list.offsetHeight;
	for (const el of els) el.classList.remove("gesture-flip");
	return owned;
}

// ---- drag to a slot --------------------------------------------------

function startDrag(e, el) {
	const orig = placementOf(el);
	const bounds = boundsNow(list);
	const pitch = slotPitch(list);
	drag = {
		el,
		orig,
		bounds,
		current: layoutIn(list),
		pitch,
		minY: (bounds.start - orig.slot) * pitch,
		maxY: (bounds.end - orig.span - orig.slot) * pitch,
		siblings: siblingsFor(el, pitch),
		valid: { slot: orig.slot, layout: layoutIn(list) },
		pointerId: e.pointerId,
		startX: e.clientX,
		startY: e.clientY,
		lastX: e.clientX,
		lastY: e.clientY,
		scrollV: 0,
		raf: 0,
		moved: false,
		downTarget: e.target,
		lifted: false,
	};
	watchServer(drag);
	if (!e.target.closest(".block-label")) lift(drag);
}

function lift(d) {
	if (d.lifted) return;
	d.lifted = true;
	d.el.classList.add("dragging");
}

function applyDrag() {
	const d = drag;
	const dx = d.lastX - d.startX;
	const dy = d.lastY - d.startY;
	if (Math.abs(dx) > TAP_SLOP || Math.abs(dy) > TAP_SLOP) {
		d.moved = true;
		lift(d);
	}
	const y = Math.max(d.minY, Math.min(d.maxY, dy));
	const x = 10 * Math.tanh(dx / 60);
	const tilt = reduceMotion.matches ? 0 : x / 4;
	d.el.style.transform = `translateX(${x}px) translateY(${y}px) rotate(${tilt}deg)`;
	previewDrag(d.orig.slot + Math.round(y / d.pitch));
}

function autoScroll(clientY) {
	const d = drag;
	if (!d) return;
	const r = list.getBoundingClientRect();
	let v = 0;
	if (clientY < r.top + EDGE) v = -((r.top + EDGE - clientY) / EDGE) * MAX_SPEED;
	else if (clientY > r.bottom - EDGE)
		v = ((clientY - (r.bottom - EDGE)) / EDGE) * MAX_SPEED;
	d.scrollV = Math.max(-MAX_SPEED, Math.min(MAX_SPEED, v));
	if (d.scrollV && !d.raf) d.raf = requestAnimationFrame(scrollTick);
}

function scrollTick() {
	const d = drag;
	if (!d) return;
	d.raf = 0;
	if (!d.scrollV) return;
	const before = list.scrollTop;
	list.scrollTop += d.scrollV;
	const moved = list.scrollTop - before;
	if (!moved) return;
	d.startY -= moved;
	applyDrag();
	d.raf = requestAnimationFrame(scrollTick);
}

function previewDrag(slot) {
	const d = drag;
	slot = Math.max(d.bounds.start, Math.min(d.bounds.end - d.orig.span, slot));
	if (slot === d.valid.slot) return;
	const lay = pushLayout(d.bounds, d.current, {
		id: d.orig.id,
		slot,
		span: d.orig.span,
	});
	if (!lay) return;
	d.valid = { slot, layout: lay };
	moveSiblings(d, lay);
}

async function finishSettle(gesture) {
	settling = true;
	settleGesture = gesture;
	await waitForTransitions(gestureElements(gesture), ["transform", "height"]);
	if (gesture.aborted) return false;
	const owned = tearDown(gesture, gesture.valid.layout);
	settleGesture = null;
	settling = false;
	return owned;
}

async function settleDrag(e, commit) {
	if (!drag || e.pointerId !== drag.pointerId) return;
	const d = drag;
	const editLabel = commit && !d.moved ? d.downTarget.closest(".block-label") : null;
	if (!commit) d.valid = { slot: d.orig.slot, layout: d.current };
	if (d.raf) cancelAnimationFrame(d.raf);
	moveSiblings(d, d.valid.layout);
	d.el.classList.add("gesture-settling");
	d.el.style.transform = `translateX(0px) translateY(${(d.valid.slot - d.orig.slot) * d.pitch}px) rotate(0deg)`;
	drag = null;
	if (!await finishSettle(d)) return;
	if (editLabel && d.el.parentElement === list) {
		enterEdit(list, editLabel, d.startX, d.startY);
		return;
	}
	commitGesture(list, {
		el: d.el,
		id: d.orig.id,
		from: d.current,
		to: d.valid.layout,
		bounds: d.bounds,
		announce,
		say: droppedMsg(d.orig.id, d.valid.layout),
		refocus: null,
	});
}

// ---- stretch / compress ----------------------------------------------

function startResize(e, el) {
	const orig = placementOf(el);
	const pitch = slotPitch(list);
	const h0 = el.getBoundingClientRect().height;
	resize = {
		el,
		orig,
		pitch,
		margin: orig.span * pitch - h0,
		bounds: boundsNow(list),
		current: layoutIn(list),
		siblings: siblingsFor(el, pitch),
		valid: { span: orig.span, layout: layoutIn(list) },
		pointerId: e.pointerId,
		startY: e.clientY,
	};
	el.style.height = `${h0}px`;
	el.classList.add("resizing", "gesture-preview");
	watchServer(resize);
}

function previewResize(span) {
	const r = resize;
	span = Math.max(1, Math.min(r.bounds.end - r.orig.slot, span));
	if (span === r.valid.span) return;
	const lay = pushLayout(
		r.bounds,
		r.current,
		{ id: r.orig.id, slot: r.orig.slot, span },
		{ compress: true },
	);
	if (!lay) return;
	r.valid = { span, layout: lay };
	r.el.style.height = `${span * r.pitch - r.margin}px`;
	moveSiblings(r, lay);
}

async function settleResize(e, commit) {
	if (!resize || e.pointerId !== resize.pointerId) return;
	const r = resize;
	if (!commit) {
		r.valid = { span: r.orig.span, layout: r.current };
		r.el.style.height = `${r.orig.span * r.pitch - r.margin}px`;
		moveSiblings(r, r.current);
	}
	resize = null;
	if (!await finishSettle(r)) return;
	commitGesture(list, {
		el: r.el,
		id: r.orig.id,
		from: r.current,
		to: r.valid.layout,
		bounds: r.bounds,
		announce,
		say: resizedMsg(r.orig.id, r.valid.layout),
		refocus: null,
	});
}
