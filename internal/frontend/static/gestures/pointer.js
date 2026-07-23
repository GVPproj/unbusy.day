// Pointer gestures for the #block-list day grid: drag to move and grip-stretch
// to resize. Motion springs the displaced siblings live (the real <li> is
// transformed, no ghost), the push cascade (push.js, ADR 0005) is previewed as
// you drag, and the gesture commits as a FLIP — one synchronous frame swaps the
// spring styles for the committed grid placement, then a single `layout` event
// carries the result to Datastar. Listeners are delegated to #block-list so
// they survive SSE morphs.
//
// Arbitration: a pointerdown cancels any active keyboard gesture
// (arb.keyboard.cancel()); this path's own handlers bail while a drag/resize is
// already in flight or the FLIP settle is animating (isActive()).

import {
	animate,
	motionValue,
	styleEffect,
} from "https://cdn.jsdelivr.net/npm/motion@12.40.0/+esm";
import { pushLayout } from "../push.js";
import { enterEdit } from "./rename.js";
import {
	blocksIn,
	placementOf,
	layoutIn,
	boundsNow,
	slotPitch,
	writeLayout,
	sameLayout,
} from "./grid.js";

const SPRING = { type: "spring", stiffness: 600, damping: 38 };
// Reduced-motion viewers get an instant snap instead of the settle/push spring.
// The held block still tracks the pointer 1:1 (applyDrag .set()s x/y directly,
// unanimated), so direct manipulation survives — only the eased motion drops.
const reduceMotion = matchMedia("(prefers-reduced-motion: reduce)");
const settle = () => (reduceMotion.matches ? { duration: 0 } : SPRING);
// Past this many px the gesture is a drag, not a tap-to-edit.
const TAP_SLOP = 4;
const EDGE = 48; // px band at each edge that triggers auto-scroll
const MAX_SPEED = 16; // px per frame at the very edge

let list; // #block-list, injected via init
let arb; // cross-gesture arbitration handle ({ keyboard, pointer })
let drag = null;
let resize = null;
let settling = false;

function isActive() {
	return drag !== null || resize !== null || settling;
}

// Abort an in-flight drag/resize by reverting to its origin synchronously (no
// spring — the gesture is superseded, not settled; unlike pointercancel, which
// routes through settle*(e, false)). Currently unreferenced — the keyboard path
// bails while the pointer is active rather than cancelling it — but kept so the
// arbitration contract holds if that policy ever flips.
function cancel() {
	if (drag) {
		const d = drag;
		drag = null;
		if (d.raf) cancelAnimationFrame(d.raf);
		tearDown(d, "drag", d.current);
	}
	if (resize) {
		const r = resize;
		resize = null;
		if (r.hAnim) r.hAnim.stop();
		tearDown(r, "resize", r.current);
	}
}

export function init(ctx, arbitration) {
	list = ctx.list;
	arb = arbitration;
	list.addEventListener("pointerdown", onPointerdown);
	list.addEventListener("pointermove", onPointermove);
	list.addEventListener("pointerup", onPointerup);
	list.addEventListener("pointercancel", onPointercancel);
	return { isActive, cancel };
}

function onPointerdown(e) {
	if (drag || resize || settling || e.button !== 0) return;
	// A pointer gesture supersedes an in-progress keyboard grab/resize.
	arb.keyboard?.cancel();
	// A label in edit mode owns its own pointer (caret/selection).
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

// Per-gesture motionValues for every other block, NOT animate(element, …):
// teardown wipes style.transform behind Motion's back, and animate(element)
// would resume from Motion's stale cached y and teleport a sibling.
function sibsFor(el) {
	const sibs = new Map();
	for (const c of blocksIn(list)) {
		if (c === el) continue;
		const y = motionValue(0);
		// h0 is the natural height (margins put it under span*pitch); springing
		// from here keeps unchanged siblings from twitching.
		const h0 = c.getBoundingClientRect().height;
		const h = motionValue(h0);
		sibs.set(c, {
			y,
			h,
			h0,
			detach: styleEffect(c, { y, height: h }),
			yAnim: null,
			hAnim: null,
		});
	}
	return sibs;
}

// Spring every other block to its slot delta (and, under compression, its
// height) for the live preview layout `lay`.
function springSibs(g, lay) {
	const by = new Map(lay.map((p) => [p.id, p]));
	g.sibs.forEach((s, c) => {
		const p = by.get(c.dataset.id);
		const fromSlot = parseInt(c.dataset.slot, 10);
		const fromSpan = parseInt(c.dataset.span, 10) || 1;
		if (s.yAnim) s.yAnim.stop();
		s.yAnim = animate(s.y, (p.slot - fromSlot) * g.pitch, settle());
		// keep the natural margin (fromSpan*pitch - h0) when springing to a new span
		const toH = p.span === fromSpan ? s.h0 : p.span * g.pitch - (fromSpan * g.pitch - s.h0);
		if (s.hAnim) s.hAnim.stop();
		s.hAnim = animate(s.h, toH, settle());
	});
}

// Stop sibling springs and clear the styles Motion left behind.
function teardownSibs(g) {
	g.sibs.forEach((s, c) => {
		if (s.yAnim) s.yAnim.stop();
		if (s.hAnim) s.hAnim.stop();
		s.detach();
		c.style.transform = "";
		c.style.height = "";
	});
}

// Shared end-of-gesture teardown (cancel + settle, drag + resize). The final
// writeLayout is guarded because a foreign morph during a settle await can
// detach the block — we must not re-assert stale positions over the server's.
// `kind` is "drag" (transform / .dragging) or "resize" (height / .resizing).
function tearDown(g, kind, layout) {
	g.detach();
	teardownSibs(g);
	if (kind === "drag") {
		g.el.style.transform = "";
		g.el.classList.remove("dragging");
	} else {
		g.el.style.height = "";
		g.el.classList.remove("resizing");
	}
	if (g.el.parentElement === list) writeLayout(list, layout, g.bounds.start);
}

const sibAnims = (g) => [...g.sibs.values()].flatMap((s) => [s.yAnim, s.hAnim]);

// Tell the server the gesture's result — unless nothing changed, or a foreign
// patch replaced the block mid-gesture (the server's layout already won).
function dispatchLayout(g) {
	if (sameLayout(g.valid.layout, g.current)) return;
	if (g.el.parentElement !== list) return;
	list.dispatchEvent(
		new CustomEvent("layout", { detail: { layout: g.valid.layout } }),
	);
}

// ---- drag to a slot --------------------------------------------------

function startDrag(e, el) {
	const orig = placementOf(el);
	const bounds = boundsNow(list);
	const pitch = slotPitch(list);
	const x = motionValue(0);
	const y = motionValue(0);
	// Tilt driven by the resisted lateral pull (see applyDrag).
	const rot = motionValue(0);
	drag = {
		el,
		orig,
		x,
		y,
		rot,
		bounds,
		current: layoutIn(list),
		pitch,
		// Clamp: a translate past the grid grows the scroll container's overflow
		// without bound.
		minY: (bounds.start - orig.slot) * pitch,
		maxY: (bounds.end - orig.span - orig.slot) * pitch,
		detach: styleEffect(el, { x, y, rotate: rot }),
		sibs: sibsFor(el),
		// last layout the cascade accepted — what an invalid pointer snaps to
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
	};
	el.classList.add("dragging");
}

// Split out of pointermove so the auto-scroll loop can re-apply the transform
// each frame (the pointer is stationary while the container scrolls under it).
function applyDrag() {
	const d = drag;
	const dx = d.lastX - d.startX;
	const dy = d.lastY - d.startY;
	if (Math.abs(dx) > TAP_SLOP || Math.abs(dy) > TAP_SLOP) d.moved = true;
	const y = Math.max(d.minY, Math.min(d.maxY, dy));
	// X is locked to the column: lateral pull is rubber-band resisted and tilts
	// the block, so the drag feels tethered to the list. Amplitude stays small
	// (10px, ~2.5°) — the grid's clip box has ~1rem of slack (app.css), which
	// must absorb the lift scale (~3px) plus this drift plus the tilt corners.
	// Tilt is decorative rotation, so reduced-motion drops it; the drift
	// itself is direct manipulation.
	const px = 10 * Math.tanh(dx / 60);
	d.x.set(px);
	d.rot.set(reduceMotion.matches ? 0 : px / 4);
	d.y.set(y);
	previewDrag(d.orig.slot + Math.round(y / d.pitch));
}

// Edge auto-scroll. Speed ramps with depth into the EDGE band; the rAF loop
// self-sustains while the pointer holds at an edge (no pointermove fires).
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

// One auto-scroll frame: scroll, then fold the scrolled distance into startY
// so the held block keeps tracking the stationary pointer.
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

// Preview the cascade at `slot`: clamp into the day, keep the last valid
// layout when the push rejects, so invalid drops snap to legal positions.
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
	springSibs(d, lay);
}

async function settleDrag(e, commit) {
	if (!drag || e.pointerId !== drag.pointerId) return;
	const d = drag;
	// A tap (no movement) on the label opens the inline editor after the settle.
	const editLabel =
		commit && !d.moved ? d.downTarget.closest(".block-label") : null;
	if (!commit) d.valid = { slot: d.orig.slot, layout: d.current };
	if (d.raf) cancelAnimationFrame(d.raf);
	springSibs(d, d.valid.layout);
	drag = null;
	settling = true;
	try {
		await Promise.all([
			animate(d.x, 0, settle()),
			animate(d.rot, 0, settle()),
			animate(d.y, (d.valid.slot - d.orig.slot) * d.pitch, settle()),
			...sibAnims(d),
		]);
	} finally {
		// Teardown and the layout write share one synchronous frame: same pixels,
		// new grid placement (FLIP).
		tearDown(d, "drag", d.valid.layout);
		settling = false;
	}
	if (editLabel && d.el.parentElement === list) {
		enterEdit(list, editLabel, d.startX, d.startY);
		return;
	}
	dispatchLayout(d);
}

// ---- stretch / compress ----------------------------------------------

function startResize(e, el) {
	const orig = placementOf(el);
	const pitch = slotPitch(list);
	const h0 = el.getBoundingClientRect().height;
	const h = motionValue(h0);
	resize = {
		el,
		orig,
		h,
		pitch,
		// CSS margin around a block, preserved so a resized block keeps the same
		// gap a static one has.
		margin: orig.span * pitch - h0,
		bounds: boundsNow(list),
		current: layoutIn(list),
		hAnim: null,
		detach: styleEffect(el, { height: h }),
		sibs: sibsFor(el),
		valid: { span: orig.span, layout: layoutIn(list) },
		pointerId: e.pointerId,
		startY: e.clientY,
	};
	el.classList.add("resizing");
}

// Preview the grown/shrunk span: clamp to the day end, keep the last valid
// layout when growing would push a block past the bottom.
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
	if (r.hAnim) r.hAnim.stop();
	r.hAnim = animate(r.h, span * r.pitch - r.margin, settle());
	springSibs(r, lay);
}

async function settleResize(e, commit) {
	if (!resize || e.pointerId !== resize.pointerId) return;
	const r = resize;
	if (!commit) {
		r.valid = { span: r.orig.span, layout: r.current };
		if (r.hAnim) r.hAnim.stop();
		r.hAnim = animate(r.h, r.orig.span * r.pitch - r.margin, settle());
		springSibs(r, r.current);
	}
	resize = null;
	settling = true;
	try {
		await Promise.all(
			[r.hAnim, ...sibAnims(r)].filter(Boolean),
		);
	} finally {
		if (r.hAnim) r.hAnim.stop();
		tearDown(r, "resize", r.valid.layout);
		settling = false;
	}
	dispatchLayout(r);
}
