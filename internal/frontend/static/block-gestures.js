// Block-gestures entry — the sole public seam for the pointer + keyboard
// gestures on #block-list. Boots both path modules and wires the shared
// arbitration registry (`arb`): each path gets a live { isActive, cancel }
// handle to the other and enforces the policy itself (pointerdown cancels a
// keyboard grab; keydown bails while the pointer path is active/settling).
//
// Exports only initBlockGestures (never self-executes), so it's safe to import
// under node --test. The gestures/ modules are private to this entry; the one
// exception is the contract test (jstest/block-gestures.test.js), which imports
// gestures/keyboard.js directly since pointer.js pulls Motion from a CDN.
//
// Event contract — CustomEvents to Datastar's data-on:* on #block-list:
//   • layout { detail: { layout: [{id, slot, span}, …] } } — committed move/resize
//   • rename { detail: { id, label } }                     — inline label edit
//   • delete { detail: { id } }                            — keyboard delete
// The per-block delete button posts directly via data-on:click (UNB-26).

import { init as initKeyboard } from "./gestures/keyboard.js";

// A path that returns no { isActive, cancel } handle would silently no-op every
// arbitration guard (the UNB-26 regression), so fail loudly at boot instead.
function bindArb(name, handle) {
	if (!handle || typeof handle.isActive !== "function" || typeof handle.cancel !== "function")
		throw new Error(`block-gestures: ${name} path returned no arbitration handle`);
	return handle;
}

// `announce` is a function (msg) => void, not the #sr-announce element.
export function initBlockGestures(list, announce) {
	const ctx = { list, announce };

	// Populated as the modules init — keyboard synchronously, pointer once Motion
	// has loaded (imported lazily since node --test can't resolve its CDN URL).
	const arb = {};
	arb.keyboard = bindArb("keyboard", initKeyboard(ctx, arb));
	import("./gestures/pointer.js").then(({ init }) => {
		arb.pointer = bindArb("pointer", init(ctx, arb));
	});
}
