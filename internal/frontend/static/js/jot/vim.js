import { Vim } from "/static/vendor/codemirror/modules/@replit__codemirror-vim__codemirror-vim.mjs";

// Vim 6.4's insertion recorder is global; keep its private-state bridge here.
export function createVimSession(view, adapter) {
	const macro = Vim.getVimGlobalState_().macroModeState;
	const recording = { changes: [], expectCursorActivityForChange: false };
	const activate = () => { macro.lastInsertModeChanges = recording; };
	const run = (action) => {
		const previous = macro.lastInsertModeChanges;
		activate();
		try {
			return action();
		} finally {
			macro.lastInsertModeChanges = previous;
		}
	};
	const cancelInsertRepeat = () => {
		const state = adapter.state.vim;
		if (!state?.insertMode) return;
		state.insertModeRepeat = 1;
		if (state.lastEditInputState) state.lastEditInputState.repeatOverride = 1;
	};
	// Capture also covers native paste, deferred composition, and Vim prompt inputs.
	const events = ["keydown", "keyup", "beforeinput", "paste", "compositionstart", "compositionupdate", "compositionend"];
	for (const type of events) view.dom.addEventListener(type, activate, { capture: true });
	adapter.on("inputEvent", activate);

	return {
		run,
		remote(action) {
			return run(() => {
				cancelInsertRepeat();
				// Insert/caret state survives; Visual, Replace and pending operators do not.
				if (!adapter.state.vim?.insertMode || adapter.state.overwrite) {
					Vim.handleKey(adapter, "<Esc>", "user");
				}
				const wasPlaying = macro.isPlaying;
				// The core's replay guard excludes authoritative text/cursor changes from recording.
				macro.isPlaying = true;
				try {
					return action();
				} finally {
					macro.isPlaying = wasPlaying;
				}
			});
		},
		destroy() {
			try {
				run(() => {
					cancelInsertRepeat();
					// leaveVimMode alone leaks Insert's native keydown listener in this release.
					if (adapter.state.vim?.insertMode) Vim.exitInsertMode(adapter, true);
				});
			} finally {
				adapter.off("inputEvent", activate);
				for (const type of events) view.dom.removeEventListener(type, activate, { capture: true });
			}
		},
	};
}
