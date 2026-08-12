// The one way a gesture ends. Both adapters (pointer.js, keyboard.js) commit
// through here, so the guard order — same? → still-in-list? → write → refresh
// accessible mirrors → announce → steer focus → dispatch — can't diverge.

import { writeLayout, sameLayout } from "./grid.js";
import { restoreFocusAfterMorph } from "./rename.js";
import { timeRange } from "./keyboard-reducer.js";

// Finish a gesture: persist the layout, refresh the accessible mirrors,
// announce, steer focus across the morph, and tell the server. Returns
// whether anything was dispatched. A no-op gesture (layout unchanged, or the
// block was replaced by a foreign morph mid-gesture) does nothing, silently.
export function commitGesture(list, {
	el,        // the gesture's element
	id,
	from,      // grab-start layout
	to,        // committed layout
	bounds,
	announce,  // (msg) => void
	say,       // announcement string, or null
	refocus,   // () => Element | null, or null to leave focus alone
}) {
	if (sameLayout(from, to)) return false;
	if (el.parentElement !== list) return false; // the server already won
	writeLayout(list, to, bounds.start);
	refreshMirrors(el, id, to, bounds);
	if (say && announce) announce(say);
	if (refocus) restoreFocusAfterMorph(list, refocus);
	list.dispatchEvent(new CustomEvent("layout", { detail: { layout: to } }));
	return true;
}

// Refresh the grip's splitter values and the block's .sr-only clock range to
// the committed placement (mirroring column_block.templ). Every lookup is
// guarded — an SSE patch can remove any of these mid-gesture.
function refreshMirrors(el, id, layout, bounds) {
	const p = layout.find((q) => q.id === id);
	if (!p) return;
	const grip = el.querySelector(".grip");
	if (grip) {
		grip.setAttribute("aria-valuenow", p.span);
		grip.setAttribute("aria-valuemax", bounds.end - p.slot);
		grip.setAttribute("aria-valuetext", timeRange(p.slot, p.span));
	}
	const sr = el.querySelector(".sr-only");
	if (sr) sr.textContent = timeRange(p.slot, p.span) + ", ";
}
