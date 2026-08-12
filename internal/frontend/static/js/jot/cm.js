// The Jotpad editor: CodeMirror 6 with the markdown keymap, wired to the
// shared jot-sync save/convergence driver.
//
// Plain pinned esm.sh imports — no ?deps: default builds are pre-cached
// (?deps combos build on demand and routinely 408), and their shared deps
// are range URLs that redirect to one resolved version, so the whole graph
// dedupes to a single @codemirror/state + view instance (CM breaks silently
// on duplicates). jsdelivr +esm is unusable here: its cached CM builds pin
// *different* state versions per package.

import {
	EditorView,
	keymap,
	placeholder,
	drawSelection,
	highlightSpecialChars,
	Decoration,
	ViewPlugin,
} from "https://esm.sh/@codemirror/view@6.43.8";
import {
	EditorState,
	RangeSetBuilder,
} from "https://esm.sh/@codemirror/state@6.7.1";
import {
	history,
	defaultKeymap,
	historyKeymap,
} from "https://esm.sh/@codemirror/commands@6.10.4";
import {
	syntaxHighlighting,
	syntaxTree,
} from "https://esm.sh/@codemirror/language@6.12.4";
import { classHighlighter } from "https://esm.sh/@lezer/highlight@1.2.3";
import {
	markdown,
	markdownLanguage,
} from "https://esm.sh/@codemirror/lang-markdown@6.5.2";
import { languages } from "https://esm.sh/@codemirror/language-data@6.5.2";

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
 * {version, status} from the server render. Remote text lands as one minimal
 * CM transaction, so the selection maps through it and the cursor stays put
 * unless the remote edit deleted the text under it.
 */
export function initJotpadCM(mount, initialText, postURL, maxLen, opts = {}) {
	// Set while the sync driver rewrites the doc, so its own transaction isn't
	// mistaken for typing and re-posted as a local edit.
	let applying = false;

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
				// markdown() adds its own Enter/Backspace keymap (list
				// continuation, marker cleanup) at high precedence; language-data
				// gives fenced code blocks their inner language's highlighting.
				// The default base is commonmark — markdownLanguage is the GFM
				// dialect, needed for TaskMarker (and tables/strikethrough).
				markdown({ base: markdownLanguage, codeLanguages: languages }),
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
				EditorView.updateListener.of((u) => {
					if (!u.docChanged || applying) return;
					sync.edited();
				}),
			],
		}),
	});

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
		status: opts.status,
	});

	// The SSE stream hands (version, text) here via data-on-signal-patch.
	window.__jotRemote = (v, text) => sync.remote(v, text);

	wireTeardown(sync, view.contentDOM);

	return view;
}
