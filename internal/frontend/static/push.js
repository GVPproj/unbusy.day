// Distribute a grow's deficit across the blocks below it, closest-first: the
// block nearest the handle shrinks to its one-slot floor before the next gives
// up a single slot. Pure: below-block runs (sorted closest-first) + the slots
// available below the grower → compressed runs (same order), or null when even
// every block at span 1 can't fit the grow.
export function compressBelow(below, available) {
	const out = below.map((p) => ({ id: p.id, span: p.span }));
	let deficit = out.reduce((s, p) => s + p.span, 0) - available;
	for (const c of out) {
		if (deficit <= 0) break;
		const give = Math.min(c.span - 1, deficit);
		c.span -= give;
		deficit -= give;
	}
	return deficit <= 0 ? out : null;
}

// Pure push cascade (ADR 0005): given the day bounds, the current layout,
// and one block's proposed placement, returns the full resulting layout —
// displaced blocks slide toward the slot the moved block vacated (so a
// down-drag pushes others up and an up-drag pushes others down, the "slide
// behind" reorder), gaps absorb first — or null when any block would be
// forced past either edge of the day. With `compress` (the resize-grow path),
// a grow that would shove the stack below past the bottom edge compresses those
// blocks closest-first instead of rejecting.
export function pushLayout(bounds, current, moved, { compress = false } = {}) {
	// Fixed runs nothing may overlap; the moved block's run is fixed first,
	// then each settled block's. Displaced blocks sweep outward from the moved
	// block and are pushed past whatever they overlap — the minimum distance,
	// in the direction of the vacated slot — so gaps absorb the cascade.
	if (moved.slot < bounds.start || moved.slot + moved.span > bounds.end) return null;
	// Direction: dragging down (past the block's old slot) slides displaced
	// blocks up into the vacated space; dragging up (or a resize, same slot)
	// pushes them down. The old slot lives in `current`; `moved` is the new one.
	const was = current.find((p) => p.id === moved.id);
	const up = was ? moved.slot > was.slot : false;

	// Resize-grow compression: the blocks below the grower must fit packed into
	// [moved.bottom, bounds.end). Free space (the bottom gap plus inter-block
	// gaps) absorbs first — only when tight-packing still overflows do we
	// compress the surplus away closest-first; otherwise the normal sweep runs.
	if (compress && !up) {
		const below = current
			.filter((p) => p.id !== moved.id && p.slot > moved.slot)
			.sort((a, b) => a.slot - b.slot);
		const available = bounds.end - (moved.slot + moved.span);
		const tight = below.reduce((s, p) => s + p.span, 0);
		if (tight > available) {
			const compressed = compressBelow(below, available);
			if (!compressed) return null; // even span-1 floors can't fit the grow
			const out = new Map(current.map((p) => [p.id, { ...p }]));
			out.set(moved.id, { ...moved });
			let slot = moved.slot + moved.span;
			for (const c of compressed) {
				out.set(c.id, { ...out.get(c.id), slot, span: c.span });
				slot += c.span;
			}
			return current.map((p) => out.get(p.id));
		}
	}
	const fixed = [{ slot: moved.slot, span: moved.span }];
	const out = new Map([[moved.id, { ...moved }]]);
	// Sweep from the moved block outward: ascending pushes down, descending up,
	// so each displaced block clears the runs already settled before it.
	const others = current
		.filter((p) => p.id !== moved.id)
		.slice()
		.sort((a, b) => (up ? b.slot - a.slot : a.slot - b.slot));
	for (const p of others) {
		let slot = p.slot;
		for (let again = true; again; ) {
			again = false;
			for (const f of fixed) {
				if (slot < f.slot + f.span && f.slot < slot + p.span) {
					slot = up ? f.slot - p.span : f.slot + f.span;
					again = true;
				}
			}
		}
		if (slot < bounds.start || slot + p.span > bounds.end) return null;
		fixed.push({ slot, span: p.span });
		out.set(p.id, { ...p, slot });
	}
	return current.map((p) => out.get(p.id));
}
