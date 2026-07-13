// Live block countdown: shows the time remaining in the block active at the
// viewer's current LOCAL time (which the server can't know) in the
// #block-countdown pill, colored by the active block's type. Ticks each second
// and re-reads #block-list every tick so it survives SSE morphs.

function tick() {
	const list = document.getElementById("block-list");
	const pill = document.getElementById("block-countdown");
	if (!list || !pill) return;

	const d = new Date();
	const nowSecs = d.getHours() * 3600 + d.getMinutes() * 60 + d.getSeconds();
	const slot = Math.floor(nowSecs / 1800); // 30-min slots from midnight

	let active = null;
	for (const item of list.querySelectorAll(".block-item")) {
		const start = parseInt(item.dataset.slot || "0", 10);
		const span = parseInt(item.dataset.span || "1", 10);
		if (start <= slot && slot < start + span) {
			active = { end: start + span, type: item.dataset.type || "" };
			break;
		}
	}

	if (!active) {
		pill.hidden = true;
		return;
	}

	const remaining = active.end * 1800 - nowSecs; // seconds left in the block
	const pad = (n) => String(n).padStart(2, "0");
	pill.textContent = `${pad(Math.floor(remaining / 3600))}:${pad(
		Math.floor((remaining % 3600) / 60),
	)}:${pad(remaining % 60)}`;
	pill.dataset.type = active.type;
	pill.hidden = false;
}

function boot() {
	tick();
	setInterval(tick, 1000);
}

if (document.readyState === "loading") {
	addEventListener("DOMContentLoaded", boot);
} else {
	boot();
}
