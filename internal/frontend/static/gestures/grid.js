// State-free DOM↔layout helpers for the #block-list day grid. Each takes `list`
// explicitly so the gesture modules stay injectable in shape.

// Real block children only (slots and the now-pill are filtered out).
export const blocksIn = (list) =>
	[...list.children].filter((c) => c.classList.contains("block-item"));

// The committed placement persisted in a block element's data-* attributes.
export const placementOf = (c) => ({
	id: c.dataset.id,
	slot: parseInt(c.dataset.slot, 10),
	span: parseInt(c.dataset.span, 10) || 1,
});

export const layoutIn = (list) => blocksIn(list).map(placementOf);

export const boundsNow = (list) => ({
	start: parseInt(list.dataset.dayStart, 10),
	end: parseInt(list.dataset.dayEnd, 10),
});

// Row pitch measured from consecutive slot rows.
export function slotPitch(list) {
	const slots = [...list.querySelectorAll(":scope > .slot")];
	if (slots.length > 1)
		return (
			slots[1].getBoundingClientRect().top -
			slots[0].getBoundingClientRect().top
		);
	return slots[0].getBoundingClientRect().height;
}

// Write a committed layout into the persisted attributes and grid placement —
// the same shape the server renders, so the patch lands as a no-op morph.
export function writeLayout(list, layout, dayStart) {
	const by = new Map(layout.map((p) => [p.id, p]));
	for (const c of blocksIn(list)) {
		const p = by.get(c.dataset.id);
		if (!p) continue;
		c.dataset.slot = p.slot;
		c.dataset.span = p.span;
		c.style.setProperty("--span", p.span);
		c.style.gridRow = p.slot - dayStart + 1 + " / span " + p.span;
	}
}

// Structural equality (same ids in the same slots/spans).
export const sameLayout = (a, b) =>
	a.every((p) => {
		const q = b.find((x) => x.id === p.id);
		return q && q.slot === p.slot && q.span === p.span;
	});
