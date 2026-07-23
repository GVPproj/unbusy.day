// PROTOTYPE — throwaway. Three variants of the x-locked drag lifted state,
// switchable via ?variant= (a|b|c) with a floating bottom bar. Sets
// data-proto-drag on <html>; pointer.js reads it for the lateral behavior,
// app.css's "PROTOTYPE proto-drag" section styles each lifted state.
// Delete this file, its blocks.templ include, the pointer.js hook, and the
// CSS section once a variant wins.

const VARIANTS = [
	{ key: "a", name: "Rail — hard lock, classic lift" },
	{ key: "b", name: "Tether — rubber-band pull + tilt" },
	{ key: "c", name: "Groove — pressed-in tile, sibs dim" },
];

function currentKey() {
	const k = new URLSearchParams(location.search).get("variant");
	return VARIANTS.some((v) => v.key === k) ? k : "a";
}

function apply(key) {
	document.documentElement.dataset.protoDrag = key;
	const v = VARIANTS.find((v) => v.key === key);
	bar.querySelector(".proto-bar-label").textContent =
		key.toUpperCase() + " — " + v.name.split("—")[1].trim();
	const url = new URL(location.href);
	url.searchParams.set("variant", key);
	history.replaceState(null, "", url);
}

function cycle(dir) {
	const i = VARIANTS.findIndex((v) => v.key === currentKey());
	apply(VARIANTS[(i + dir + VARIANTS.length) % VARIANTS.length].key);
}

const bar = document.createElement("div");
bar.className = "proto-bar";
bar.innerHTML =
	'<button type="button" class="proto-bar-prev" aria-label="Previous variant">←</button>' +
	'<span class="proto-bar-label"></span>' +
	'<button type="button" class="proto-bar-next" aria-label="Next variant">→</button>';
bar.querySelector(".proto-bar-prev").addEventListener("click", () => cycle(-1));
bar.querySelector(".proto-bar-next").addEventListener("click", () => cycle(1));
document.body.append(bar);

document.addEventListener("keydown", (e) => {
	if (e.target.closest("input, textarea, [contenteditable]")) return;
	if (e.key === "ArrowLeft") cycle(-1);
	else if (e.key === "ArrowRight") cycle(1);
});

apply(currentKey());
