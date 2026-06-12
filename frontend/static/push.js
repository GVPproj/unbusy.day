// Pure push cascade (ADR 0005): given the day bounds, the current layout,
// and one card's proposed placement, returns the full resulting layout —
// displaced cards slide down by the minimum distance, gaps absorb first —
// or null when any card would be forced past the end of the day.
export function pushLayout(bounds, current, moved) {
	// Fixed runs nothing may overlap; the moved card's run is fixed first,
	// then each settled card's. Sweeping the others in ascending slot order
	// pushes each past whatever it overlaps — the minimum distance — and
	// leaves gaps to absorb the cascade.
	if (moved.slot < bounds.start || moved.slot + moved.span > bounds.end) return null;
	const fixed = [{ slot: moved.slot, span: moved.span }];
	const out = new Map([[moved.id, { ...moved }]]);
	const others = current
		.filter((p) => p.id !== moved.id)
		.slice()
		.sort((a, b) => a.slot - b.slot);
	for (const p of others) {
		let slot = p.slot;
		for (let again = true; again; ) {
			again = false;
			for (const f of fixed) {
				if (slot < f.slot + f.span && f.slot < slot + p.span) {
					slot = f.slot + f.span;
					again = true;
				}
			}
		}
		if (slot + p.span > bounds.end) return null;
		fixed.push({ slot, span: p.span });
		out.set(p.id, { ...p, slot });
	}
	return current.map((p) => out.get(p.id));
}
