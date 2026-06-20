// Live "now" line: marks the viewer's current LOCAL time on the day grid.
// The server can't know the viewer's clock, so this is the one client-side
// bit — it positions the server-rendered #now-line element (see column.templ)
// at the current 30-min slot and fills its time pill. Same pass strikes through
// the labels of blocks that have fully elapsed (end slot at/above the line).
// Re-runs on a timer and after every SSE morph of #block-list (idiomorph
// re-renders the line hidden).

let observer;

function place() {
	const list = document.getElementById("block-list");
	const line = document.getElementById("now-line");
	if (!list || !line) return;
	// Suspend the observer: our writes below mutate #block-list and reacting
	// to them would loop.
	if (observer) observer.disconnect();

	const d = new Date();
	const slot = Math.floor((d.getHours() * 60 + d.getMinutes()) / 30);
	const dayStart = parseInt(list.dataset.dayStart || "0", 10);
	const dayEnd = parseInt(list.dataset.dayEnd || "48", 10);

	if (slot >= dayStart && slot < dayEnd) {
		line.style.gridRow = String(slot - dayStart + 1);
		// Offset within the slot: the line pins to the slot's top edge, so push
		// it down by the fraction of the 30 min already elapsed. Measure a real
		// slot's pixel height since --slot-h can be a non-fixed calc().
		const minsIntoSlot = d.getHours() * 60 + d.getMinutes() - slot * 30;
		const sample = list.querySelector(".slot");
		const slotH = sample ? sample.offsetHeight : 0;
		line.style.transform = `translateY(${(minsIntoSlot / 30) * slotH}px)`;
		const h = ((d.getHours() + 11) % 12) + 1; // 12-hour, no leading zero
		line.querySelector(".now-time").textContent =
			`${h}:${String(d.getMinutes()).padStart(2, "0")}`;
		line.hidden = false;
	} else {
		line.hidden = true;
	}

	// Strike through blocks that have fully elapsed: a block is "past" once its
	// end slot (start + span) sits at or above the now line (<= current slot).
	for (const item of list.querySelectorAll(".block-item")) {
		const start = parseInt(item.dataset.slot || "0", 10);
		const span = parseInt(item.dataset.span || "1", 10);
		const label = item.querySelector(".block-label");
		if (label) label.classList.toggle("past", start + span <= slot);
		// The block the now line currently sits inside gets an accent left rail.
		item.classList.toggle("active", start <= slot && slot < start + span);
	}

	if (observer) observer.observe(list, { childList: true, subtree: true });
}

function boot() {
	const list = document.getElementById("block-list");
	if (list) {
		let pending = 0;
		observer = new MutationObserver(() => {
			cancelAnimationFrame(pending);
			pending = requestAnimationFrame(place);
		});
	}
	place();
	setInterval(place, 20000); // follow the clock forward
}

if (document.readyState === "loading") {
	addEventListener("DOMContentLoaded", boot);
} else {
	boot();
}
