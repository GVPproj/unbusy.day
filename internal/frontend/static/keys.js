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
		const layout = pushLayout(bounds, current, { id: cur.id, slot: cur.slot + step, span: cur.span });
		if (!layout) return { layout: current, kind: "blocked" };
		return { layout, kind: "moved" };
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
