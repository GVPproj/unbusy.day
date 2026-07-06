// Live "now" pill: positions the server-rendered #now-pill at the viewer's
// current LOCAL time (which the server can't know) and strikes through elapsed
// blocks. Re-runs on a timer and after every SSE morph of #block-list.

let observer;

function place() {
	const list = document.getElementById("block-list");
	const pill = document.getElementById("now-pill");
	if (!list || !pill) return;
	// Suspend the observer: reacting to our own writes below would loop.
	if (observer) observer.disconnect();

	const d = new Date();
	const slot = Math.floor((d.getHours() * 60 + d.getMinutes()) / 30);
	const dayStart = parseInt(list.dataset.dayStart || "0", 10);
	const dayEnd = parseInt(list.dataset.dayEnd || "48", 10);

	if (slot >= dayStart && slot < dayEnd) {
		pill.style.gridRow = String(slot - dayStart + 1);
		// Push down by the elapsed fraction of the slot; measure a real slot's
		// height since --slot-h can be a non-fixed calc().
		const minsIntoSlot = d.getHours() * 60 + d.getMinutes() - slot * 30;
		const sample = list.querySelector(".slot");
		const slotH = sample ? sample.offsetHeight : 0;
		pill.style.transform = `translateY(${(minsIntoSlot / 30) * slotH}px)`;
		const h = ((d.getHours() + 11) % 12) + 1; // 12-hour, no leading zero
		pill.querySelector(".now-time").textContent =
			`${h}:${String(d.getMinutes()).padStart(2, "0")}`;
		pill.hidden = false;
	} else {
		pill.hidden = true;
	}

	for (const item of list.querySelectorAll(".block-item")) {
		const start = parseInt(item.dataset.slot || "0", 10);
		const span = parseInt(item.dataset.span || "1", 10);
		const label = item.querySelector(".block-label");
		if (label) label.classList.toggle("past", start + span <= slot);
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
	setInterval(place, 20000);
}

if (document.readyState === "loading") {
	addEventListener("DOMContentLoaded", boot);
} else {
	boot();
}
