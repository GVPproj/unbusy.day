// Pure keyboard decision reducer (PRD: keyboard-accessible blocks). Maps one
// keystroke against the current layout to the next layout + an announcement
// `kind`, delegating the cascade to pushLayout so push.js stays the single
// source of truth. DOM-free and unit-tested (keys.test.js).
import { pushLayout } from "./push.js";

export function keyboardLayout(bounds, current, grabbed, key) {
	const cur = current.find((p) => p.id === grabbed.id);
	if (!cur) return null;
	if (grabbed.mode === "move") {
		const step = key === "ArrowDown" ? 1 : key === "ArrowUp" ? -1 : null;
		if (step === null) return null;
		// `current` is the grab-START layout and `grabbed.slot` the mover's running
		// target slot — each press recomputes the cascade from start, exactly as a
		// pointer drag always pushes from the gesture-start layout (drag.js d.current).
		// Threading the running layout back in instead lets displacements accumulate:
		// stepping a span-1 block up past two tall blocks would shove each down by its
		// own span rather than by the mover's, leaving gaps a drag never produces.
		// Skip past an unyielding block too: a single step can be infeasible (a taller
		// neighbour can't fit the slot we vacate) while the next one over resolves, so
		// advance the target until pushLayout accepts it or we run off the day's edge.
		const from = grabbed.slot ?? cur.slot;
		for (let slot = from + step; slot >= bounds.start && slot + cur.span <= bounds.end; slot += step) {
			const layout = pushLayout(bounds, current, { id: cur.id, slot, span: cur.span });
			if (layout) return { layout, slot, kind: "moved" };
		}
		return { layout: current, kind: "blocked" };
	}
	if (grabbed.mode === "resize") {
		const span =
			key === "ArrowDown" ? cur.span + 1 :
			key === "ArrowUp" ? cur.span - 1 :
			key === "Home" ? 1 :
			key === "End" ? maxResizeSpan(bounds, current, cur) :
			null;
		if (span === null) return null;
		if (span < 1) return { layout: current, kind: "blocked" }; // one-slot floor
		const layout = pushLayout(bounds, current, { id: cur.id, slot: cur.slot, span }, { compress: true });
		if (!layout) return { layout: current, kind: "blocked" };
		return { layout, kind: "resized" };
	}
	return null;
}

// Largest span the grabbed block can grow to: probe the compress cascade from a
// full-day-to-end span downward and take the first that fits (delegating the
// floor math to push.js). Always ≥ 1, since the current one-slot span fits.
function maxResizeSpan(bounds, current, cur) {
	for (let span = bounds.end - cur.slot; span > 1; span--)
		if (pushLayout(bounds, current, { id: cur.id, slot: cur.slot, span }, { compress: true }))
			return span;
	return 1;
}
