// Motion-powered drag + stretch for #card-list: transforms the real <li>
// (no ghost), commits reorder as a FLIP, then dispatches reorder/cardresize.
// Listeners are delegated to #card-list (morph-stable) so wiring survives patches.
import { animate, motionValue, styleEffect } from "https://cdn.jsdelivr.net/npm/motion@12.40.0/+esm";

const list = document.getElementById('card-list');
const SPRING = { type: 'spring', stiffness: 600, damping: 38 };
const GAP = parseFloat(getComputedStyle(list).rowGap);
// Natural span-1 card height. The probe may be server-rendered stretched,
// so divide its height back down by its span — measuring raw would inflate CARD_H.
const probe = list.querySelector('.card');
const probeSpan = parseInt(probe.dataset.span, 10) || 1;
const CARD_H = (probe.getBoundingClientRect().height - (probeSpan - 1) * GAP) / probeSpan;
const PITCH = CARD_H + GAP;
const heightFor = (span) => CARD_H + (span - 1) * PITCH;

// Card id -> span in slots (absent = 1); cache of the persisted data-span,
// re-seeded from the DOM after every morph. During a gesture the map leads.
const spans = new Map();
const spanOf = (el) => spans.get(el.dataset.id) || 1;
const cardsIn = () => [...list.children].filter((c) => c.classList.contains('card'));
const slotsIn = () => [...list.children].filter((c) => c.classList.contains('slot'));
const extraSpans = () => cardsIn().reduce((n, c) => n + spanOf(c) - 1, 0);

// Called only when no gesture is in flight, so it never clobbers an in-progress span.
function syncSpansFromDOM() {
	for (const c of cardsIn()) {
		const s = parseInt(c.dataset.span, 10) || 1;
		if (s > 1) spans.set(c.dataset.id, s);
		else spans.delete(c.dataset.id);
	}
}

let drag = null;
let resize = null;
let settling = false;

// The one writer of stable stretch styles: card heights from the spans map,
// bottom-most slots collapsed (height 0, -GAP margin) to balance them.
// Disconnects the observer around its own writes so it never observes itself.
function apply() {
	observer.disconnect();
	for (const c of cardsIn()) {
		const s = spanOf(c);
		c.style.height = s === 1 ? '' : heightFor(s) + 'px';
	}
	const open = slotsIn().length - extraSpans();
	slotsIn().forEach((slot, i) => {
		const consumed = i >= open;
		slot.style.height = consumed ? '0px' : CARD_H + 'px';
		slot.style.marginTop = consumed ? -GAP + 'px' : '';
	});
	observer.observe(list, OBSERVED);
}

// On morph, re-seed the cache and re-assert stretch styles. Watching
// data-span (not just style) is what makes a foreign resize land.
const OBSERVED = { childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'data-span'] };
const observer = new MutationObserver(() => {
	if (drag || resize || settling) return;
	syncSpansFromDOM();
	apply();
});

list.addEventListener('pointerdown', (e) => {
	if (drag || resize || settling || e.button !== 0) return;
	const el = e.target.closest('.card');
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
		shiftSiblings(targetFor(drag.y.get()));
	} else if (resize && e.pointerId === resize.pointerId) {
		const span = resize.startSpan + Math.round((e.clientY - resize.startY) / PITCH);
		snapTo(Math.max(1, Math.min(resize.max, span)));
	}
});

list.addEventListener('pointerup', (e) => { settleDrag(e, true); settleResize(e, true); });
list.addEventListener('pointercancel', (e) => { settleDrag(e, false); settleResize(e, false); });

// ---- drag to reorder ----------------------------------------------

function startDrag(e, el) {
	const cards = cardsIn();
	const x = motionValue(0);
	const y = motionValue(0);
	drag = {
		el, cards, x, y,
		// Measured per-card: stretched cards mean no uniform step.
		pitch: cards.map((c) => c.getBoundingClientRect().height + GAP),
		detach: styleEffect(el, { x, y }),
		sibs: new Map(),
		pointerId: e.pointerId,
		from: cards.indexOf(el),
		to: cards.indexOf(el),
		startX: e.clientX,
		startY: e.clientY,
	};
	// Per-drag motionValues, NOT animate(element, …): teardown wipes
	// style.transform behind Motion's back, and animate(element) would
	// resume from Motion's stale cached y and teleport a sibling.
	for (const c of cards) {
		if (c === el) continue;
		const v = motionValue(0);
		drag.sibs.set(c, { v, detach: styleEffect(c, { y: v }), anim: null });
	}
	el.classList.add('dragging');
}

// Signed translate landing the held card at index `to`: summed pitches of passed cards.
function offsetFor(d, to) {
	let off = 0;
	for (let i = d.from + 1; i <= to; i++) off += d.pitch[i];
	for (let i = to; i < d.from; i++) off -= d.pitch[i];
	return off;
}

// Index the pointer offset has dragged the card into — crossings happen
// halfway through each passed card's pitch.
function targetFor(dy) {
	const d = drag;
	let to = d.from;
	while (to + 1 < d.cards.length && dy > offsetFor(d, to) + d.pitch[to + 1] / 2) to++;
	while (to > 0 && dy < offsetFor(d, to) - d.pitch[to - 1] / 2) to--;
	return to;
}

// Spring displaced cards one held-card-pitch out of the way of landing index `to`.
function shiftSiblings(to) {
	if (to === drag.to) return;
	drag.to = to;
	const dodge = drag.pitch[drag.from];
	drag.cards.forEach((card, i) => {
		if (card === drag.el) return;
		let y = 0;
		if (i > drag.from && i <= to) y = -dodge;
		else if (i < drag.from && i >= to) y = dodge;
		const s = drag.sibs.get(card);
		if (s.anim) s.anim.stop();
		s.anim = animate(s.v, y, SPRING);
	});
}

async function settleDrag(e, commit) {
	if (!drag || e.pointerId !== drag.pointerId) return;
	const d = drag;
	if (!commit) shiftSiblings(d.from); // cancelled: spring everyone home
	drag = null;
	settling = true;
	try {
		// land the held card AND let in-flight sibling springs finish
		await Promise.all([
			animate(d.x, 0, SPRING),
			animate(d.y, offsetFor(d, d.to), SPRING),
			...[...d.sibs.values()].map((s) => s.anim),
		]);
	} finally {
		// Teardown and the DOM reorder below share one synchronous
		// frame: same pixels, new layout (FLIP).
		d.detach();
		d.sibs.forEach((s) => { if (s.anim) s.anim.stop(); s.detach(); });
		d.cards.forEach((c) => { c.style.transform = ''; });
		d.el.classList.remove('dragging');
		settling = false;
	}
	if (d.to === d.from) return;
	// A foreign patch may have replaced the children mid-drag — the
	// server's order already won, so don't re-append detached nodes.
	if (d.cards.some((c) => c.parentElement !== list)) return;
	const order = [...d.cards];
	order.splice(d.from, 1);
	order.splice(d.to, 0, d.el);
	// Reinsert ahead of the slot rail so slots stay at the bottom.
	const firstSlot = list.querySelector(':scope > .slot');
	order.forEach((c) => list.insertBefore(c, firstSlot));
	list.dispatchEvent(new CustomEvent('reorder', {
		detail: { order: order.map((c) => c.dataset.id) },
	}));
}

// ---- stretch / compress ---------------------------------------------

// Grip drag. motionValues are per-gesture (the Motion cache gotcha above —
// apply() rewrites these styles behind Motion's back).
function startResize(e, el) {
	const slots = slotsIn();
	const span = spanOf(el);
	const h = motionValue(el.getBoundingClientRect().height);
	resize = {
		el, h, span,
		startSpan: span,
		// shared pool: a card may grow by however many slots are open
		max: span + Math.max(0, slots.length - extraSpans()),
		hAnim: null,
		detach: styleEffect(el, { height: h }),
		slots: slots.map((slot) => {
			const sh = motionValue(slot.getBoundingClientRect().height);
			const sm = motionValue(parseFloat(getComputedStyle(slot).marginTop) || 0);
			return { sh, sm, detach: styleEffect(slot, { height: sh, marginTop: sm }), anims: [] };
		}),
		pointerId: e.pointerId,
		startY: e.clientY,
	};
	el.classList.add('resizing');
}

// Spring the card to the snapped span; the slot rail collapses in the same
// spring so the column bottom barely moves.
function snapTo(span) {
	if (span === resize.span) return;
	resize.span = span;
	if (resize.hAnim) resize.hAnim.stop();
	resize.hAnim = animate(resize.h, heightFor(span), SPRING);
	const open = resize.slots.length - (extraSpans() - (resize.startSpan - 1)) - (span - 1);
	resize.slots.forEach((s, i) => {
		const consumed = i >= open;
		s.anims.forEach((a) => a.stop());
		s.anims = [
			animate(s.sh, consumed ? 0 : CARD_H, SPRING),
			animate(s.sm, consumed ? -GAP : 0, SPRING),
		];
	});
}

async function settleResize(e, commit) {
	if (!resize || e.pointerId !== resize.pointerId) return;
	if (!commit) snapTo(resize.startSpan); // cancelled: spring back
	const r = resize;
	resize = null;
	settling = true;
	try {
		await Promise.all([r.hAnim, ...r.slots.flatMap((s) => s.anims)].filter(Boolean));
	} finally {
		if (r.hAnim) r.hAnim.stop();
		r.detach();
		r.slots.forEach((s) => { s.anims.forEach((a) => a.stop()); s.detach(); });
		r.el.classList.remove('resizing');
		settling = false;
	}
	if (r.span === 1) spans.delete(r.el.dataset.id);
	else spans.set(r.el.dataset.id, r.span);
	// Write the committed span into the persisted attributes now, or stale
	// .stretched/--span CSS snaps the card back until the patch lands.
	r.el.dataset.span = r.span;
	r.el.style.setProperty('--span', r.span);
	r.el.classList.toggle('stretched', r.span > 1);
	// Also covers a mid-gesture clobber: apply() targets fresh nodes by id.
	apply();
	// Skip a no-op gesture (a plain tap).
	if (r.span !== r.startSpan && r.el.parentElement === list) {
		list.dispatchEvent(new CustomEvent('cardresize', {
			detail: { id: r.el.dataset.id, span: r.span },
		}));
	}
}

syncSpansFromDOM();
apply();
