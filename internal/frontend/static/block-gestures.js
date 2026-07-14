// Block-gestures entry — the sole public seam for the pointer + keyboard
// gestures on the #block-list day grid. Boots the two path modules and owns the
// cross-gesture arbitration between them: a pointer gesture supersedes an
// in-progress keyboard grab/resize, and keydown bails while the pointer path is
// mid-gesture or settling (its FLIP settle must finish first).
//
// This module never self-executes — it only exports initBlockGestures — so it
// stays safe to import under node --test (it runs no DOM lookups at load time).
//
// Privacy: the `gestures/` modules are an implementation detail of this entry.
// Import them only via block-gestures.js, never directly from a route, another
// script, or a test. (Plain folder name — the privacy rule lives here, not in a
// `_`-prefixed path.)
//
// Event contract — three CustomEvents cross from the gesture modules to Datastar,
// consumed by the `data-on:*` attributes on #block-list in column.templ:
//   • layout  { detail: { layout: [{id, slot, span}, …] } }  — committed move/resize
//   • rename  { detail: { id, label } }                       — inline label edit
//   • delete  { detail: { id } }                              — keyboard delete
// The per-block delete BUTTON posts directly via `data-on:click="@post(…)"` —
// the idiomatic Datastar path for a stateless button; that asymmetry is intent,
// not drift (UNB-26).

import { init as initKeyboard } from "./gestures/keyboard.js";

// Each path's init must hand back a live { isActive, cancel } handle — the whole
// point of arbitration. A path that returns nothing would silently no-op every
// guard (the UNB-26 regression), so fail loudly at boot instead.
function bindArb(name, handle) {
	if (!handle || typeof handle.isActive !== "function" || typeof handle.cancel !== "function")
		throw new Error(`block-gestures: ${name} path returned no arbitration handle`);
	return handle;
}

// `announce` is a function (msg) => void, not the #sr-announce element, so the
// null-guard on a missing live region stays with the bootstrap that owns the DOM.
export function initBlockGestures(list, announce) {
	const ctx = { list, announce };

	// Cross-gesture arbitration: each path reads the other's state (isActive)
	// and can abort it (cancel). Populated as the modules init — keyboard
	// synchronously, pointer once Motion has loaded.
	const arb = {};
	arb.keyboard = bindArb("keyboard", initKeyboard(ctx, arb));

	// Motion loads from a CDN over https, which node --test can't resolve, so
	// the pointer path is imported lazily here (this entry never imports it at
	// module top level). Keyboard gestures work immediately; pointer gestures
	// wire up a few ms later once Motion arrives.
	import("./gestures/pointer.js").then(({ init }) => {
		arb.pointer = bindArb("pointer", init(ctx, arb));
	});
}
