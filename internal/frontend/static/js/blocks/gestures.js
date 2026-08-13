// Block-gestures entry — the sole public seam for the pointer + keyboard
// gestures on #block-list. Boots both path modules and wires the shared
// arbitration registry (`arb`): each path holds a live { isActive, cancel }
// handle to the other and enforces the policy itself.
//
// Committed gestures leave as CustomEvents on #block-list, read by Datastar's
// data-on:* — `layout` { layout: [{id, slot, span}] }, `rename` { id, label },
// `delete` { id }.

import { init as initKeyboard } from "./keyboard.js";

// A path missing its handle would silently no-op every arbitration guard, so
// fail loudly at boot instead.
function bindArb(name, handle) {
	if (!handle || typeof handle.isActive !== "function" || typeof handle.cancel !== "function")
		throw new Error(`block-gestures: ${name} path returned no arbitration handle`);
	return handle;
}

// `announce` is a function (msg) => void, not the #sr-announce element.
export function initBlockGestures(list, announce) {
	const ctx = { list, announce };

	// Pointer is imported lazily: node --test can't resolve Motion's CDN URL.
	const arb = {};
	arb.keyboard = bindArb("keyboard", initKeyboard(ctx, arb));
	import("./pointer.js").then(({ init }) => {
		arb.pointer = bindArb("pointer", init(ctx, arb));
	});
}
