// Jotpad sync: the one save/convergence path shared by both editors (jot.js
// textarea, jot-cm.js CodeMirror). The decision logic is pure functions (node
// --test covered); createJotSync wires them to fetch/timers. The server is the
// authority: writes are compare-and-swap on a version, stale writes come back
// merged, and remote edits arrive as (version, text) events that the editor
// applies itself — nothing here ever touches the DOM.

/**
 * What to do with a remote (version, text) event.
 * - 'ignore': stale or our own echo — a device adopts the version returned by
 *   its own POST, so its own fan-out arrives as version <= local.
 * - 'adopt':  same text, newer version — bump the version, no edit.
 * - 'buffer': local unsaved typing exists; converge through the pending POST's
 *   merge response instead of applying both (one convergence path, not two).
 * - 'apply':  clean editor, genuinely newer text — apply it.
 */
export function remoteAction(local, remote) {
	if (remote.version <= local.version) return "ignore";
	if (remote.text === local.text) return "adopt";
	if (local.dirty || local.inflight) return "buffer";
	return "apply";
}

/**
 * Interpret a POST's {version, text?} ack: text is present only when the
 * server stored something other than what we sent (merge or rejection), and
 * the editor must visibly snap to it.
 */
export function ackAction(ack) {
	return { version: ack.version, apply: ack.text ?? null };
}

/**
 * The smallest single replacement turning oldText into newText, or null when
 * equal — dispatched as one CM transaction so the selection maps through it.
 */
export function minimalEdit(oldText, newText) {
	if (oldText === newText) return null;
	const max = Math.min(oldText.length, newText.length);
	let from = 0;
	while (from < max && oldText[from] === newText[from]) from++;
	let s = 0;
	while (
		s < max - from &&
		oldText[oldText.length - 1 - s] === newText[newText.length - 1 - s]
	) {
		s++;
	}
	return { from, to: oldText.length - s, insert: newText.slice(from, newText.length - s) };
}

/** Capped exponential backoff: 2s, 4s, 8s, … max 30s. failures >= 1. */
export function retryDelay(failures) {
	return Math.min(30_000, 1000 * 2 ** Math.min(failures, 5));
}

/** The honest save state for the indicator. */
export function statusOf({ offline, dirty, inflight }) {
	if (offline) return "offline";
	if (dirty || inflight) return "saving";
	return "saved";
}

const STATUS_TEXT = {
	saved: "Saved",
	saving: "Saving…",
	offline: "Offline — will retry",
};

// "Saving…" only shows once the state has persisted ~500ms, so normal typing
// under a fast network never flickers the indicator.
const SAVING_DELAY = 500;
const DEBOUNCE = 1000;

/**
 * Wires the sync loop for one editor.
 *  - getText():        the editor's current document
 *  - applyText(text):  make the editor show exactly text (CM transaction or
 *                      el.value — the editor's business, never a DOM morph)
 *  - postURL, version: the save endpoint and the version the page rendered
 *  - status:           the indicator element (optional)
 * Returns {edited, flush, beacon, remote} — call edited() on every local
 * change, beacon() on teardown; remote() is fed by the SSE signal patches.
 */
export function createJotSync({ getText, applyText, postURL, version, status }) {
	let synced = { version, text: getText() };
	let debounce = 0;
	let retryTimer = 0;
	let statusTimer = 0;
	let inflight = false;
	let buffered = null;
	let failures = 0;
	let offline = false;

	const dirty = () => getText() !== synced.text;

	// The indicator counts a browser-declared offline state as offline even
	// before a save has had the chance to fail.
	const isOffline = () =>
		offline || (typeof navigator !== "undefined" && navigator.onLine === false);

	const render = (state) => {
		if (!status) return;
		status.dataset.state = state;
		status.textContent = STATUS_TEXT[state];
	};

	const updateStatus = () => {
		const state = statusOf({ offline: isOffline(), dirty: dirty(), inflight });
		if (state !== "saving") {
			clearTimeout(statusTimer);
			statusTimer = 0;
			render(state);
			return;
		}
		if (statusTimer || status?.dataset.state === "saving") return;
		statusTimer = setTimeout(() => {
			statusTimer = 0;
			if (statusOf({ offline: isOffline(), dirty: dirty(), inflight }) === "saving") {
				render("saving");
			}
		}, SAVING_DELAY);
	};

	// A buffered remote is resolved after every ack: by then the merge response
	// has adopted a version at or past it, so it either applies or drops.
	const resolveBuffered = () => {
		if (!buffered) return;
		const b = buffered;
		buffered = null;
		remote(b.version, b.text);
	};

	const flush = () => {
		clearTimeout(debounce);
		debounce = 0;
		if (inflight) return; // the ack path re-flushes if still dirty
		const text = getText();
		if (text === synced.text) {
			updateStatus();
			return;
		}
		inflight = true;
		// keepalive: a save racing a navigation still completes.
		fetch(postURL, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ _jot: text, _jotVersion: synced.version }),
			keepalive: true,
		})
			.then((r) => {
				if (!r.ok) throw new Error(`jot save: ${r.status}`);
				return r.json();
			})
			.then((ack) => {
				inflight = false;
				failures = 0;
				offline = false;
				const { version: v, apply } = ackAction(ack);
				synced = { version: v, text: apply ?? text };
				// A merge/rejection makes the server text authoritative, visibly.
				if (apply !== null && apply !== getText()) applyText(apply);
				resolveBuffered();
				if (dirty()) flush();
				else updateStatus();
			})
			.catch(() => {
				inflight = false;
				failures++;
				offline = true;
				updateStatus();
				clearTimeout(retryTimer);
				retryTimer = setTimeout(flush, retryDelay(failures));
			});
	};

	const edited = () => {
		clearTimeout(debounce);
		debounce = setTimeout(flush, DEBOUNCE);
		updateStatus();
	};

	const remote = (v, text) => {
		const action = remoteAction(
			{ version: synced.version, text: getText(), dirty: dirty(), inflight },
			{ version: v, text },
		);
		if (action === "ignore") return;
		if (action === "adopt" || action === "apply") {
			synced = { version: v, text };
			if (action === "apply") applyText(text);
			updateStatus();
			return;
		}
		buffered = { version: v, text }; // converge via the pending POST's merge
	};

	// Teardown flush: a beacon survives page death but its response is
	// unreadable, so the committed version is reconciled by the next SSE echo
	// or page load. synced.text is advanced only to stop blur → hidden →
	// pagehide triple-firing; one duplicate write beats a lost paragraph.
	const beacon = () => {
		clearTimeout(debounce);
		const text = getText();
		if (text === synced.text) return;
		const body = new Blob(
			[JSON.stringify({ _jot: text, _jotVersion: synced.version })],
			{ type: "application/json" },
		);
		navigator.sendBeacon(postURL, body);
		synced = { version: synced.version, text };
		updateStatus();
	};

	window.addEventListener("online", () => {
		offline = false;
		clearTimeout(retryTimer);
		if (dirty()) flush();
		else updateStatus();
	});
	window.addEventListener("offline", updateStatus);

	return { edited, flush, beacon, remote };
}

/**
 * Wires the teardown beacon to every moment the page might not get another
 * tick. sendBeacon rather than fetch: a request started during teardown is
 * routinely cancelled, and an iOS PWA (ADR 0010) is suspended without a clean
 * unload — exactly where a paragraph would otherwise be lost. blurTarget is
 * the editor's focusable element.
 */
export function wireTeardown(sync, blurTarget) {
	blurTarget.addEventListener("blur", sync.beacon);
	window.addEventListener("pagehide", sync.beacon);
	document.addEventListener("visibilitychange", () => {
		if (document.hidden) sync.beacon();
	});
}
