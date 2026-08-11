// PROTOTYPE — branch prototype/jotpad-codemirror. CodeMirror 6 variant of the
// Jotpad, answering "does a CM markdown editor feel right here?". Loaded only
// when the page is opened with ?jot=cm. Throwaway: not for main.
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
} from "https://esm.sh/@codemirror/view@6.43.8";
import { EditorState } from "https://esm.sh/@codemirror/state@6.7.1";
import {
	history,
	defaultKeymap,
	historyKeymap,
} from "https://esm.sh/@codemirror/commands@6.10.4";
import { syntaxHighlighting } from "https://esm.sh/@codemirror/language@6.12.4";
import { classHighlighter } from "https://esm.sh/@lezer/highlight@1.2.3";
import { markdown } from "https://esm.sh/@codemirror/lang-markdown@6.5.2";
import { languages } from "https://esm.sh/@codemirror/language-data@6.5.2";

// Mirrors jotPlaceholder in components/jotpad.templ.
const PLACEHOLDER = `e.g.

## Monday

- call Mom

## Tuesday

## Wednesday

## Thursday

## Friday`;

/**
 * Mounts a CodeMirror Jotpad into `mount`. Save contract matches jot.js: a 1s
 * debounced POST of {_jot} to postURL, flushed via sendBeacon whenever the
 * page might not get another tick.
 */
export function initJotpadCM(mount, initialText, postURL, maxLen) {
	let debounce = 0;
	let sent = initialText;

	const post = (text) => {
		sent = text;
		fetch(postURL, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ _jot: text }),
		}).catch(() => {});
	};

	const view = new EditorView({
		parent: mount,
		state: EditorState.create({
			doc: initialText,
			extensions: [
				highlightSpecialChars(),
				history(),
				drawSelection(),
				EditorView.lineWrapping,
				placeholder(PLACEHOLDER),
				// markdown() adds its own Enter/Backspace keymap (list
				// continuation, marker cleanup) at high precedence; language-data
				// gives fenced code blocks their inner language's highlighting.
				markdown({ codeLanguages: languages }),
				// classHighlighter emits plain .tok-* classes, styled from
				// app.css with the theme tokens — no CSS-in-JS theme.
				syntaxHighlighting(classHighlighter),
				keymap.of([...defaultKeymap, ...historyKeymap]),
				// The jot.MaxLen cap; approximate (UTF-16 units vs server
				// runes) but the server logs-and-drops over-cap writes anyway.
				EditorState.transactionFilter.of((tr) =>
					tr.newDoc.length > maxLen ? [] : tr,
				),
				EditorView.updateListener.of((u) => {
					if (!u.docChanged) return;
					clearTimeout(debounce);
					debounce = setTimeout(() => post(view.state.doc.toString()), 1000);
				}),
			],
		}),
	});

	const flush = () => {
		const text = view.state.doc.toString();
		if (text === sent) return;
		clearTimeout(debounce);
		sent = text;
		const body = new Blob([JSON.stringify({ _jot: text })], {
			type: "application/json",
		});
		navigator.sendBeacon(postURL, body);
	};
	view.contentDOM.addEventListener("blur", flush);
	window.addEventListener("pagehide", flush);
	document.addEventListener("visibilitychange", () => {
		if (document.hidden) flush();
	});

	return view;
}
