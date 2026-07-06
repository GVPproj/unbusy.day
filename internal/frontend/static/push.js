// Distribute a grow's deficit across the blocks below it, closest-first: the
// block nearest the handle shrinks to its one-slot floor before the next gives
// anything up. Returns null when even every block at span 1 can't fit.
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

// Pure push cascade (ADR 0005): one block's proposed placement → the full
// resulting layout. Displaced blocks slide toward the slot the moved block
// vacated, gaps absorb first; null when any block would be forced past either
// edge. With `compress` (resize-grow), an overflowing grow compresses the
// stack below closest-first instead of rejecting.
export function pushLayout(bounds, current, moved, { compress = false } = {}) {
	if (moved.slot < bounds.start || moved.slot + moved.span > bounds.end) return null;
	// Dragging down slides displaced blocks up into the vacated space; dragging
	// up (or a resize, same slot) pushes them down.
	const was = current.find((p) => p.id === moved.id);
	const up = was ? moved.slot > was.slot : false;

	// Only when tight-packing the blocks below still overflows do we compress;
	// otherwise free space absorbs the grow via the normal sweep.
	if (compress && !up) {
		const below = current
			.filter((p) => p.id !== moved.id && p.slot > moved.slot)
			.sort((a, b) => a.slot - b.slot);
		const available = bounds.end - (moved.slot + moved.span);
		const tight = below.reduce((s, p) => s + p.span, 0);
		if (tight > available) {
			const compressed = compressBelow(below, available);
			if (!compressed) return null;
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
	// Sweep outward from the moved block so each displaced block clears the
	// runs already settled before it.
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
