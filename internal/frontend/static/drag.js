// Motion-powered drag + stretch for the #block-list day grid: transforms the
// real <li> (no ghost), previews the client-computed push cascade live
// (push.js, ADR 0005), commits as a FLIP, then dispatches one `layout` event
// carrying the full resulting layout. Listeners are delegated to #block-list
// (morph-stable) so wiring survives patches.
import { animate, motionValue, styleEffect } from "https://cdn.jsdelivr.net/npm/motion@12.40.0/+esm";
import { pushLayout } from "./push.js";

const list = document.getElementById('block-list');
const SPRING = { type: 'spring', stiffness: 600, damping: 38 };

const blocksIn = () => [...list.children].filter((c) => c.classList.contains('block'));
const boundsNow = () => ({ start: parseInt(list.dataset.dayStart, 10), end: parseInt(list.dataset.dayEnd, 10) });
const placementOf = (c) => ({ id: c.dataset.id, slot: parseInt(c.dataset.slot, 10), span: parseInt(c.dataset.span, 10) || 1 });
const layoutIn = () => blocksIn().map(placementOf);

// Row pitch from consecutive slot rows — every slot is a real fixed-height
// grid row now, so geometry is measured, never derived from a probe block.
function slotPitch() {
	const slots = [...list.querySelectorAll(':scope > .slot')];
	if (slots.length > 1) return slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top;
	return slots[0].getBoundingClientRect().height;
}

// Write a committed layout into the persisted attributes and grid placement —
// the same shape the server renders, so the patch lands as a no-op morph.
function writeLayout(layout, dayStart) {
	const by = new Map(layout.map((p) => [p.id, p]));
	for (const c of blocksIn()) {
		const p = by.get(c.dataset.id);
		if (!p) continue;
		c.dataset.slot = p.slot;
		c.dataset.span = p.span;
		c.style.setProperty('--span', p.span);
		c.style.gridRow = (p.slot - dayStart + 1) + ' / span ' + p.span;
	}
}

const sameLayout = (a, b) =>
	a.every((p) => { const q = b.find((x) => x.id === p.id); return q && q.slot === p.slot && q.span === p.span; });

let drag = null;
let resize = null;
let settling = false;

list.addEventListener('pointerdown', (e) => {
	if (drag || resize || settling || e.button !== 0) return;
	const el = e.target.closest('.block');
	if (!el || el.parentElement !== list) return;
	e.preventDefault();
	el.setPointerCapture(e.pointerId);
	if (e.target.closest('.grip')) startResize(e, el);
	else startDrag(e, el);
});

list.addEventListener('pointermove', (e) => {
	if (drag && e.pointerId === drag.pointerId) {
		drag.x.set(e.clientX - drag.startX);
		drag.y.set(e.clientY - drag.startY);
		previewDrag(drag.orig.slot + Math.round(drag.y.get() / drag.pitch));
	} else if (resize && e.pointerId === resize.pointerId) {
		previewResize(resize.orig.span + Math.round((e.clientY - resize.startY) / resize.pitch));
	}
});

list.addEventListener('pointerup', (e) => { settleDrag(e, true); settleResize(e, true); });
list.addEventListener('pointercancel', (e) => { settleDrag(e, false); settleResize(e, false); });

// Per-gesture motionValues for every other block, NOT animate(element, …):
// teardown wipes style.transform behind Motion's back, and animate(element)
// would resume from Motion's stale cached y and teleport a sibling.
function sibsFor(el) {
	const sibs = new Map();
	for (const c of blocksIn()) {
		if (c === el) continue;
		const v = motionValue(0);
		sibs.set(c, { v, detach: styleEffect(c, { y: v }), anim: null });
	}
	return sibs;
}

// Spring every other block to its slot delta under layout `lay` (the live
// push preview); identical for drag and resize.
function springSibs(g, lay) {
	const by = new Map(lay.map((p) => [p.id, p]));
	g.sibs.forEach((s, c) => {
		const from = parseInt(c.dataset.slot, 10);
		const to = by.get(c.dataset.id).slot;
		if (s.anim) s.anim.stop();
		s.anim = animate(s.v, (to - from) * g.pitch, SPRING);
	});
}

// ---- drag to a slot --------------------------------------------------

function startDrag(e, el) {
	const orig = placementOf(el);
	const x = motionValue(0);
	const y = motionValue(0);
	drag = {
		el, orig, x, y,
		bounds: boundsNow(),
		current: layoutIn(),
		pitch: slotPitch(),
		detach: styleEffect(el, { x, y }),
		sibs: sibsFor(el),
		// last layout the cascade accepted — what an invalid pointer snaps to
		valid: { slot: orig.slot, layout: layoutIn() },
		pointerId: e.pointerId,
		startX: e.clientX,
		startY: e.clientY,
	};
	el.classList.add('dragging');
}

// Preview the cascade at `slot`: clamp into the day, keep the last valid
// layout when the push rejects, so invalid drops snap to legal positions.
function previewDrag(slot) {
	const d = drag;
	slot = Math.max(d.bounds.start, Math.min(d.bounds.end - d.orig.span, slot));
	if (slot === d.valid.slot) return;
	const lay = pushLayout(d.bounds, d.current, { id: d.orig.id, slot, span: d.orig.span });
	if (!lay) return;
	d.valid = { slot, layout: lay };
	springSibs(d, lay);
}

async function settleDrag(e, commit) {
	if (!drag || e.pointerId !== drag.pointerId) return;
	const d = drag;
	if (!commit) d.valid = { slot: d.orig.slot, layout: d.current };
	springSibs(d, d.valid.layout);
	drag = null;
	settling = true;
	try {
		// land the held block AND let in-flight sibling springs finish
		await Promise.all([
			animate(d.x, 0, SPRING),
			animate(d.y, (d.valid.slot - d.orig.slot) * d.pitch, SPRING),
			...[...d.sibs.values()].map((s) => s.anim),
		]);
	} finally {
		// Teardown and the layout write below share one synchronous frame:
		// same pixels, new grid placement (FLIP).
		d.detach();
		d.sibs.forEach((s) => { if (s.anim) s.anim.stop(); s.detach(); });
		d.el.style.transform = '';
		d.sibs.forEach((_, c) => { c.style.transform = ''; });
		d.el.classList.remove('dragging');
		if (d.el.parentElement === list) writeLayout(d.valid.layout, d.bounds.start);
		settling = false;
	}
	if (sameLayout(d.valid.layout, d.current)) return;
	// A foreign patch may have replaced the children mid-drag — the server's
	// layout already won, so don't dispatch a stale one.
	if (d.el.parentElement !== list) return;
	list.dispatchEvent(new CustomEvent('layout', { detail: { layout: d.valid.layout } }));
}

// ---- stretch / compress ----------------------------------------------

function startResize(e, el) {
	const orig = placementOf(el);
	const pitch = slotPitch();
	const h = motionValue(el.getBoundingClientRect().height);
	resize = {
		el, orig, h, pitch,
		bounds: boundsNow(),
		current: layoutIn(),
		hAnim: null,
		detach: styleEffect(el, { height: h }),
		sibs: sibsFor(el),
		valid: { span: orig.span, layout: layoutIn() },
		pointerId: e.pointerId,
		startY: e.clientY,
	};
	el.classList.add('resizing');
}

// Preview the grown/shrunk span: clamp to the day end, keep the last valid
// layout when growing would push a block past the bottom.
function previewResize(span) {
	const r = resize;
	span = Math.max(1, Math.min(r.bounds.end - r.orig.slot, span));
	if (span === r.valid.span) return;
	const lay = pushLayout(r.bounds, r.current, { id: r.orig.id, slot: r.orig.slot, span });
	if (!lay) return;
	r.valid = { span, layout: lay };
	if (r.hAnim) r.hAnim.stop();
	r.hAnim = animate(r.h, span * r.pitch, SPRING);
	springSibs(r, lay);
}

async function settleResize(e, commit) {
	if (!resize || e.pointerId !== resize.pointerId) return;
	const r = resize;
	if (!commit) {
		r.valid = { span: r.orig.span, layout: r.current };
		if (r.hAnim) r.hAnim.stop();
		r.hAnim = animate(r.h, r.orig.span * r.pitch, SPRING);
		springSibs(r, r.current);
	}
	resize = null;
	settling = true;
	try {
		await Promise.all([r.hAnim, ...[...r.sibs.values()].map((s) => s.anim)].filter(Boolean));
	} finally {
		if (r.hAnim) r.hAnim.stop();
		r.detach();
		r.sibs.forEach((s) => { if (s.anim) s.anim.stop(); s.detach(); });
		r.el.style.height = '';
		r.sibs.forEach((_, c) => { c.style.transform = ''; });
		r.el.classList.remove('resizing');
		if (r.el.parentElement === list) writeLayout(r.valid.layout, r.bounds.start);
		settling = false;
	}
	if (sameLayout(r.valid.layout, r.current)) return;
	if (r.el.parentElement !== list) return;
	list.dispatchEvent(new CustomEvent('layout', { detail: { layout: r.valid.layout } }));
}
