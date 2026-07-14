// The Day Plan's live "now" indicator — the one module for everything the server
// can't render because it turns on the VIEWER'S local clock (CONTEXT.md: "The
// User's current time of day is indicated live on the plan."). It merges what
// were countdown.js + now-pill.js so the now-slot math and the #block-list scan
// exist exactly once. Each tick it:
//   • positions #now-pill at the current time and reveals it,
//   • marks elapsed blocks .past and the block spanning now .active,
//   • fills #block-countdown with the time left in that active block, tinted by
//     its type via data-type.
// The server render stays authoritative for layout; this only overlays "now".
//
// One tick a second (the countdown counts down in seconds), one MutationObserver
// so the overlay snaps to an SSE morph within a frame instead of waiting out the
// tick, one scan of .block-item feeding both the pill and the countdown.
//
// The time-math helpers below are pure and DOM-free so they run under
// node --test (jstest/now.test.js); tick() is the only DOM glue.

// Slots are 30-min steps from local midnight; SLOT_SECS is one slot in seconds.
const SLOT_SECS = 1800;

// Seconds since local midnight — the single clock reading a tick derives from.
export const secsSinceMidnight = (d) =>
	d.getHours() * 3600 + d.getMinutes() * 60 + d.getSeconds();

// Which slot the clock sits in, and how far (0–1) it is through that slot.
export const nowSlot = (nowSecs) => Math.floor(nowSecs / SLOT_SECS);
export const slotFraction = (nowSecs) => (nowSecs % SLOT_SECS) / SLOT_SECS;

// Seconds left in a block that ends at endSlot (exclusive), given the clock.
export const remainingSecs = (endSlot, nowSecs) => endSlot * SLOT_SECS - nowSecs;

// A block [start, start+span) relative to the current slot.
export const isActive = (start, span, slot) => start <= slot && slot < start + span;
export const isPast = (start, span, slot) => start + span <= slot;

const pad = (n) => String(n).padStart(2, "0");

// HH:MM:SS for the countdown pill.
export const formatCountdown = (secs) =>
	`${pad(Math.floor(secs / 3600))}:${pad(Math.floor((secs % 3600) / 60))}:${pad(secs % 60)}`;

// 12-hour h:MM (no leading zero on the hour) for the now-pill label.
export const formatClock = (hours24, mins) => `${((hours24 + 11) % 12) + 1}:${pad(mins)}`;

// --- DOM glue ---------------------------------------------------------

let observer;

function tick() {
	const list = document.getElementById("block-list");
	if (!list) return;
	// Our own writes below (notably .now-time's text) mutate the observed
	// subtree, so suspend the observer to keep them from re-triggering it.
	if (observer) observer.disconnect();

	const now = new Date();
	const nowSecs = secsSinceMidnight(now);
	const slot = nowSlot(nowSecs);

	placeNowPill(list, now, nowSecs, slot);
	const active = markBlocks(list, slot); // one scan, feeds the countdown
	updateCountdown(active, nowSecs);

	if (observer) observer.observe(list, { childList: true, subtree: true });
}

// The one .block-item scan: toggles .active/.past and returns the active block
// (end slot + type) for the countdown, or null when now falls outside every block.
function markBlocks(list, slot) {
	let active = null;
	for (const item of list.querySelectorAll(".block-item")) {
		const start = parseInt(item.dataset.slot || "0", 10);
		const span = parseInt(item.dataset.span || "1", 10);
		const on = isActive(start, span, slot);
		item.classList.toggle("active", on);
		const label = item.querySelector(".block-label");
		if (label) label.classList.toggle("past", isPast(start, span, slot));
		if (on && !active) active = { end: start + span, type: item.dataset.type || "" };
	}
	return active;
}

// Position #now-pill at the current time inside the day's bounds, or hide it when
// the clock is outside them.
function placeNowPill(list, now, nowSecs, slot) {
	const pill = document.getElementById("now-pill");
	if (!pill) return;
	const dayStart = parseInt(list.dataset.dayStart || "0", 10);
	const dayEnd = parseInt(list.dataset.dayEnd || "48", 10);
	if (slot < dayStart || slot >= dayEnd) {
		pill.hidden = true;
		return;
	}
	pill.style.gridRow = String(slot - dayStart + 1);
	// Push down by the elapsed fraction of the slot; measure a real slot's
	// height since --slot-h can be a non-fixed calc().
	const sample = list.querySelector(".slot");
	const slotH = sample ? sample.offsetHeight : 0;
	pill.style.transform = `translateY(${slotFraction(nowSecs) * slotH}px)`;
	pill.querySelector(".now-time").textContent = formatClock(now.getHours(), now.getMinutes());
	pill.hidden = false;
}

// Fill #block-countdown with the time left in the active block, or hide it.
function updateCountdown(active, nowSecs) {
	const pill = document.getElementById("block-countdown");
	if (!pill) return;
	if (!active) {
		pill.hidden = true;
		return;
	}
	pill.textContent = formatCountdown(remainingSecs(active.end, nowSecs));
	pill.dataset.type = active.type;
	pill.hidden = false;
}

function boot() {
	const list = document.getElementById("block-list");
	if (list) {
		let pending = 0;
		observer = new MutationObserver(() => {
			cancelAnimationFrame(pending);
			pending = requestAnimationFrame(tick);
		});
	}
	tick();
	setInterval(tick, 1000);
}

// Auto-boot in the browser (this is a plain <script> include, not an init seam).
// Guard on `document` so importing the pure helpers under node --test is safe.
if (typeof document !== "undefined") {
	if (document.readyState === "loading") {
		addEventListener("DOMContentLoaded", boot);
	} else {
		boot();
	}
}
