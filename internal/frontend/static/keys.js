// Pure keyboard decision reducer: one keystroke against the current layout →
// the next layout + an announcement `kind`. DOM-free; cascade delegated to push.js.
import { pushLayout } from "./push.js";

// Vim aliases: j/k are pure synonyms for ArrowDown/ArrowUp. Every surveyed app
// (Gmail, GCal, Notion Calendar) means navigate down/up by j/k, never a
// mutation. Folded in here so both move and resize call sites inherit it and
// it's unit-testable at this seam; the DOM glue passes the raw key through.
export const normalizeKey = (key) =>
	key === "j" ? "ArrowDown" : key === "k" ? "ArrowUp" : key;

export function keyboardLayout(bounds, current, grabbed, key) {
	key = normalizeKey(key);
	const cur = current.find((p) => p.id === grabbed.id);
	if (!cur) return null;
	if (grabbed.mode === "move") {
		const step = key === "ArrowDown" ? 1 : key === "ArrowUp" ? -1 : null;
		if (step === null) return null;
		// Each press recomputes the cascade from the grab-START layout (like a
		// pointer drag); threading the running layout back in would let
		// displacements accumulate into gaps a drag never produces. The loop also
		// skips past an unyielding block: advance the target until pushLayout
		// accepts it or we run off the day's edge.
		const from = grabbed.slot ?? cur.slot;
		for (let slot = from + step; slot >= bounds.start && slot + cur.span <= bounds.end; slot += step) {
			const layout = pushLayout(bounds, current, { id: cur.id, slot, span: cur.span });
			if (layout) return { layout, slot, kind: "moved" };
		}
		return { layout: current, kind: "blocked" };
	}
	if (grabbed.mode === "resize") {
		// Same recompute-from-START rule as move: shrinking after a grow can then
		// undo compression instead of stranding it.
		const from = grabbed.span ?? cur.span;
		const span =
			key === "ArrowDown" ? from + 1 :
			key === "ArrowUp" ? from - 1 :
			key === "Home" ? 1 :
			key === "End" ? maxResizeSpan(bounds, current, cur) :
			null;
		if (span === null) return null;
		if (span < 1) return { layout: current, kind: "blocked" };
		const layout = pushLayout(bounds, current, { id: cur.id, slot: cur.slot, span }, { compress: true });
		if (!layout) return { layout: current, kind: "blocked" };
		return { layout, span, kind: "resized" };
	}
	return null;
}

// Largest span the grabbed block can grow to: probe the compress cascade from
// full-day downward and take the first that fits.
function maxResizeSpan(bounds, current, cur) {
	for (let span = bounds.end - cur.slot; span > 1; span--)
		if (pushLayout(bounds, current, { id: cur.id, slot: cur.slot, span }, { compress: true }))
			return span;
	return 1;
}
