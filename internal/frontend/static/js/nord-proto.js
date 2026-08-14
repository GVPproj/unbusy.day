// PROTOTYPE(nord-light): floating switcher for the three light-Nord token
// candidates in app.css (nord-a/b/c). ?variant=a|b|c is shareable and
// reload-stable; arrows/keyboard cycle by reload so Datastar signals stay
// honest. Localhost-only. Remove with the app.css blocks + layout include.
(function () {
	// Dev-only gate: hide on the deployed app, allow localhost/tailnet hosts.
	if (/unbusy\.day$|fly\.dev$/.test(location.hostname)) return;

	var VARIANTS = [
		["nord-a", "A — Snow Storm"],
		["nord-b", "B — One Nord"],
		["nord-c", "C — Cream Nord"],
	];
	var param = new URLSearchParams(location.search).get("variant");
	var idx = VARIANTS.findIndex(function (p) {
		return p[0] === "nord-" + param;
	});

	// Runs before Datastar loads: localStorage is what hydrates the theme
	// signals, and setting the attributes now beats first paint.
	if (idx !== -1) {
		localStorage.setItem("colorscheme", VARIANTS[idx][0]);
		localStorage.setItem("colormode", "light");
		var d = document.documentElement;
		d.setAttribute("data-colorscheme", VARIANTS[idx][0]);
		d.setAttribute("data-colormode", "light");
	}

	function go(i) {
		var u = new URL(location.href);
		u.searchParams.set(
			"variant",
			VARIANTS[(i + VARIANTS.length) % VARIANTS.length][0].slice(5),
		);
		location.href = u;
	}
	function next() {
		go(idx === -1 ? 0 : idx + 1);
	}
	function prev() {
		go(idx === -1 ? VARIANTS.length - 1 : idx - 1);
	}
	function reset() {
		localStorage.setItem("colorscheme", "solarized");
		var u = new URL(location.href);
		u.searchParams.delete("variant");
		location.href = u;
	}

	document.addEventListener("keydown", function (e) {
		var t = e.target;
		if (
			t.closest &&
			t.closest("input, textarea, [contenteditable], .cm-editor")
		)
			return;
		if (e.key === "ArrowRight") next();
		if (e.key === "ArrowLeft") prev();
	});

	document.addEventListener("DOMContentLoaded", function () {
		var bar = document.createElement("div");
		bar.style.cssText =
			"position:fixed;bottom:1rem;left:50%;transform:translateX(-50%);" +
			"z-index:9999;display:flex;align-items:center;gap:.25rem;" +
			"background:#111;color:#fff;border-radius:999px;padding:.3rem .5rem;" +
			"font:13px/1 system-ui,sans-serif;box-shadow:0 4px 12px rgba(0,0,0,.4)";
		function btn(text, fn, title) {
			var b = document.createElement("button");
			b.textContent = text;
			b.title = title || "";
			b.style.cssText =
				"background:none;border:none;color:#fff;cursor:pointer;" +
				"font:inherit;padding:.3rem .5rem";
			b.addEventListener("click", fn);
			return b;
		}
		var label = document.createElement("span");
		label.style.cssText = "padding:0 .25rem;white-space:nowrap";
		label.textContent =
			idx === -1
				? "prototype off — " + (localStorage.getItem("colorscheme") || "?")
				: VARIANTS[idx][1];
		bar.append(
			btn("◀", prev, "previous variant (←)"),
			label,
			btn("▶", next, "next variant (→)"),
			btn("✕", reset, "back to solarized"),
		);
		document.body.append(bar);
	});
})();
