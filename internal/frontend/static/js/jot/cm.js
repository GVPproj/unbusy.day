// The Jotpad editor: CodeMirror 6 with the markdown keymap, wired to the
// shared jot-sync save/convergence driver.
//
// The CodeMirror graph is vendored as one resolved module set. Markdown is
// local; fenced code stays generic and does not load language packages.

import {
	EditorView,
	keymap,
	placeholder,
	drawSelection,
	highlightSpecialChars,
	Decoration,
	ViewPlugin,
} from "/static/vendor/codemirror/modules/@codemirror__view__view.mjs";
import {
	EditorState,
	RangeSetBuilder,
} from "/static/vendor/codemirror/modules/@codemirror__state__state.mjs";
import {
	history,
	defaultKeymap,
	historyKeymap,
} from "/static/vendor/codemirror/modules/@codemirror__commands__commands.mjs";
import {
	syntaxHighlighting,
	syntaxTree,
} from "/static/vendor/codemirror/modules/@codemirror__language__language.mjs";
import { classHighlighter } from "/static/vendor/codemirror/modules/@lezer__highlight__highlight.mjs";
import {
	markdown,
	markdownLanguage,
} from "/static/vendor/codemirror/modules/@codemirror__lang-markdown__lang-markdown.mjs";

import { createJotSync, minimalEdit, wireTeardown } from "./sync.js";

// Newlines render as line breaks, letting the empty pad sketch a weekly outline.
const PLACEHOLDER = `e.g.

## Monday

- call Mom

## Tuesday

## Wednesday

## Thursday

## Friday`;

// Code stays monospace even under a proportional feeling font: line
// decorations on fenced-code lines (fences included) and mark decorations
// on inline code, styled from app.css. classHighlighter has no monospace
// class, and nested-language tokens wouldn't carry one anyway.
const codeLine = Decoration.line({ class: "jot-codeline" });
const codeSpan = Decoration.mark({ class: "jot-codespan" });
// GFM task-list markers ("[ ]"/"[x]") get a class for pointer styling;
// taskToggle below does the actual click-to-toggle.
const taskSpan = Decoration.mark({ class: "jot-taskmarker" });

function jotDecorations(view) {
	const builder = new RangeSetBuilder();
	for (const { from, to } of view.visibleRanges) {
		syntaxTree(view.state).iterate({
			from,
			to,
			enter: (node) => {
				if (node.name === "FencedCode" || node.name === "CodeBlock") {
					for (let pos = node.from; pos <= node.to; ) {
						const line = view.state.doc.lineAt(pos);
						builder.add(line.from, line.from, codeLine);
						pos = line.to + 1;
					}
					return false;
				}
				if (node.name === "InlineCode") {
					builder.add(node.from, node.to, codeSpan);
					return false;
				}
				if (node.name === "TaskMarker") {
					builder.add(node.from, node.to, taskSpan);
					return false;
				}
			},
		});
	}
	return builder.finish();
}

const jotDecorationsPlugin = ViewPlugin.fromClass(
	class {
		constructor(view) {
			this.decorations = jotDecorations(view);
		}
		update(u) {
			// The markdown parse can finish after the doc update, so also
			// recompute when the syntax tree itself changes.
			if (
				u.docChanged ||
				u.viewportChanged ||
				syntaxTree(u.state) !== syntaxTree(u.startState)
			)
				this.decorations = jotDecorations(u.view);
		}
	},
	{ decorations: (v) => v.decorations },
);

// Click/tap on a task-list "[ ]" toggles it in place. The dispatch is an
// ordinary doc change, so it rides the same sync path as typing.
const taskToggle = EditorView.domEventHandlers({
	mousedown(event, view) {
		const pos = view.posAtCoords({ x: event.clientX, y: event.clientY });
		if (pos == null) return false;
		const tree = syntaxTree(view.state);
		let node = tree.resolveInner(pos, 1);
		if (node.name !== "TaskMarker") node = tree.resolveInner(pos, -1);
		if (node.name !== "TaskMarker") return false;
		const checked = /x/i.test(view.state.sliceDoc(node.from, node.to));
		view.dispatch({
			changes: { from: node.from, to: node.to, insert: checked ? "[ ]" : "[x]" },
		});
		event.preventDefault();
		return true;
	},
});

/**
 * Mounts a CodeMirror Jotpad into `mount`. Saving, remote application, and the
 * save-state indicator all run through the shared jot-sync driver; opts is
 * {version, onStatus} from the server render. Remote text lands as one minimal
 * CM transaction, so the selection maps through it and the cursor stays put
 * unless the remote edit deleted the text under it.
 */
export function initJotpadCM(mount, initialText, postURL, maxLen, opts = {}) {
	// Set while the sync driver rewrites the doc, so its own transaction isn't
	// mistaken for typing and re-posted as a local edit.
	let applying = false;
	let panelReturn;
	const finishReturn = () => { panelReturn = undefined; };
	const nativeCaretReset = (view) => {
		const selection = getSelection(), range = view.state.selection.main;
		return view.hasFocus && selection?.isCollapsed && view.contentDOM.contains(selection.focusNode) &&
			view.posAtDOM(selection.focusNode, selection.focusOffset) === 0 && (!range.empty || range.head > 0);
	};
	const flushReturn = (view) => {
		if (!panelReturn?.refocusing) return;
		// CM's focus heuristic can consume the first notification without refreshing its cache.
		for (let i = 0; i < 2 && nativeCaretReset(view); i++) {
			view.dom.ownerDocument.dispatchEvent(new Event("selectionchange"));
		}
		// Android may only queue reconciliation; focus writes the current model selection.
		if (nativeCaretReset(view)) view.focus();
	};
	const prepareInput = () => {
		flushReturn(view);
		if (!nativeCaretReset(view)) finishReturn();
	};

	const view = new EditorView({
		parent: mount,
		state: EditorState.create({
			doc: initialText,
			extensions: [
				// CM's injected base styles are UNLAYERED, so they beat anything
				// in app.css's @layer components regardless of specificity; the
				// feeling font must be set from CM's own theme seam.
				EditorView.theme({
					".cm-scroller": { fontFamily: "var(--font-family)" },
					// drawSelection replaces the native caret with a drawn
					// .cm-cursor whose base color is black — invisible on
					// dark colorschemes.
					".cm-cursor, .cm-dropCursor": {
						borderLeftColor: "var(--ink)",
					},
				}),
				highlightSpecialChars(),
				history(),
				drawSelection(),
				EditorView.lineWrapping,
				placeholder(PLACEHOLDER),
				// markdown() adds its own list-editing keymap. markdownLanguage is
				// the GFM dialect needed for task markers, tables, and strikethrough.
				markdown({ base: markdownLanguage }),
				// classHighlighter emits plain .tok-* classes, styled from
				// app.css with the theme tokens — no CSS-in-JS theme.
				syntaxHighlighting(classHighlighter),
				jotDecorationsPlugin,
				taskToggle,
				keymap.of([...defaultKeymap, ...historyKeymap]),
				// The jot.MaxLen cap; approximate (UTF-16 units vs server
				// runes) but the server logs-and-drops over-cap writes anyway.
				EditorState.transactionFilter.of((tr) =>
					tr.newDoc.length > maxLen ? [] : tr,
				),
				// A redisplayed editor's native caret-at-start must not replace CM's selection.
				EditorState.transactionFilter.of((tr) =>
					panelReturn?.refocusing && !tr.docChanged && tr.isUserEvent("select") &&
					tr.selection?.main.empty && tr.selection.main.head === 0
						? [tr, { selection: tr.startState.selection }] : tr,
				),
				EditorView.updateListener.of((u) => {
					if (u.docChanged && panelReturn?.scroll) panelReturn.scroll = panelReturn.scroll.map(u.changes);
					if (!u.docChanged || applying) return;
					if (panelReturn?.refocusing) finishReturn();
					sync.edited();
				}),
			],
		}),
	});

	// Capture before CM's Android key deferral, which can bypass its event observers.
	for (const type of ["keydown", "beforeinput", "compositionstart"]) {
		view.contentDOM.addEventListener(type, prepareInput, { capture: true });
	}

	// Native refocus can scroll to zero before CM measures a redisplayed editor.
	const panel = mount.closest('[role="tabpanel"]');
	panel?.addEventListener("companion-hide", () => {
		flushReturn(view);
		// Measure queued scroll effects before a rapid hide can snapshot the old viewport.
		view.coordsAtPos(view.state.selection.main.head);
		panelReturn = { scroll: view.scrollSnapshot() };
	});
	panel?.addEventListener("companion-show", () => {
		if (panelReturn?.scroll) view.dispatch({ effects: panelReturn.scroll });
	});
	view.contentDOM.addEventListener("focus", () => {
		const pending = panelReturn;
		if (!pending) return;
		pending.refocusing = true;
		if (pending.scroll) view.dispatch({ effects: pending.scroll });
		view.requestMeasure({
			key: pending,
			read: () => null,
			write: () => queueMicrotask(() => {
				if (panelReturn !== pending) return;
				flushReturn(view);
				if (panelReturn === pending && !nativeCaretReset(view)) finishReturn();
			}),
		});
	});
	// Wheel discards the saved position; pointer placement also ends selection protection.
	view.scrollDOM.addEventListener("wheel", () => {
		if (panelReturn) panelReturn.scroll = undefined;
	}, { passive: true });
	view.scrollDOM.addEventListener("pointerdown", finishReturn);

	const sync = createJotSync({
		getText: () => view.state.doc.toString(),
		applyText: (text) => {
			const edit = minimalEdit(view.state.doc.toString(), text);
			if (!edit) return;
			applying = true;
			try {
				view.dispatch({
					changes: edit,
					// A doc-length cap transaction filter is active; remote
					// authoritative text must land regardless.
					filter: false,
				});
			} finally {
				applying = false;
			}
		},
		postURL,
		version: opts.version ?? 0,
		onStatus: opts.onStatus,
	});

	// The SSE stream hands (version, text) here via data-on-signal-patch.
	window.__jotRemote = (v, text) => sync.remote(v, text);

	wireTeardown(sync, view.contentDOM);

	return view;
}
