// Pure push cascade (ADR 0005): given the day bounds, the current layout,
// and one block's proposed placement, returns the full resulting layout —
// displaced blocks slide toward the slot the moved block vacated (so a
// down-drag pushes others up and an up-drag pushes others down, the "slide
// behind" reorder), gaps absorb first — or null when any block would be
// forced past either edge of the day.
export function pushLayout(bounds, current, moved) {
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
