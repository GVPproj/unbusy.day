import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 } });

const scenarios = ["native", "late caret", "arrow", "typing", "early typing", "composition typing", "home", "pointer", "remote", "rapid", "home rapid", "offscreen"];

for (const scenario of scenarios) {
	test(`Jotpad preserves scroll when refocused before its redisplay measurement (${scenario})`, async ({ context, page }) => {
		await signIn(context);
		await page.goto(baseURL, { waitUntil: "load" });
		const note = Array.from({ length: 100 }, (_, i) => `Line ${i + 1}: keep this note`).join("\n");
		const writes = new Set();
		page.on("response", (response) => {
			if (new URL(response.url()).pathname === "/jot" && response.ok()) writes.add(response.request().postDataJSON()?._jot);
		});
		const content = page.locator(".cm-content");
		await content.click();
		await page.keyboard.insertText(note);
		await page.keyboard.press("ArrowLeft");
		await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
		if (scenario === "remote") await expect.poll(() => writes.has(note)).toBe(true);
		const state = await page.evaluateHandle(async (scenario) => {
			const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
			const view = EditorView.findFromDOM(document.querySelector(".cm-editor"));
			if (scenario === "offscreen") {
				view.scrollDOM.scrollTop = 0;
				await new Promise((resolve) => view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) }));
			}
			return { view, scroll: view.scrollDOM.scrollTop, selection: view.state.selection.toJSON(), text: view.state.doc.toString() };
		}, scenario);
		expect(await state.evaluate((saved) => saved.selection.ranges[saved.selection.main].head)).toBe(note.length - 1);
		if (scenario === "offscreen") expect(await state.evaluate((saved) => saved.scroll)).toBe(0);
		else expect(await state.evaluate((saved) => saved.scroll)).toBeGreaterThan(0);
		await page.getByRole("tab", { name: "Habits", exact: true }).click();
		await expect.poll(() => state.evaluate((saved) => saved.view.inView)).toBe(false);
		await page.locator('[commandfor="habit-create-dialog"][command="show-modal"]').click();
		await page.locator("#habit-create").getByLabel("Name", { exact: true }).fill("Another input");
		await page.keyboard.press("Escape");
		const expected = await state.evaluate((saved, scenario) => {
			const selection = structuredClone(saved.selection), range = selection.ranges[selection.main];
			let text = saved.text, scroll = saved.scroll;
			if (scenario.endsWith("typing")) {
				text = text.slice(0, range.head) + "X" + text.slice(range.head);
				range.anchor = ++range.head;
			}
			if (scenario === "arrow") range.anchor = --range.head;
			if (scenario.startsWith("home") || scenario === "pointer") range.anchor = range.head = scroll = 0;
			if (scenario === "remote") {
				text = "PREFIX " + text;
				range.anchor = range.head += "PREFIX ".length;
			}
			return { scroll, selection, nativeCaret: scenario === "offscreen" ? null : range.head, text };
		}, scenario);
		// Keep refocus and early input in one frame, independent of runner speed.
		await state.evaluate((saved, scenario) => {
			const editor = saved.view.contentDOM;
			if (scenario !== "native") editor.addEventListener("focus", () => {
				// Replay browser caret placement both within and beyond CM's focus heuristic.
				getSelection().collapse(editor, 0);
				if (scenario !== "early typing" && scenario !== "composition typing") {
					const until = performance.now() + 250;
					while (performance.now() < until) {}
				}
			}, { once: true });
			const showJot = () => {
				document.querySelector('[aria-controls="jot-panel"]').click();
				document.querySelector("#jot-panel").focus();
				editor.focus();
			};
			showJot();
			if (scenario === "arrow" || scenario.endsWith("typing") || scenario.startsWith("home")) {
				let key = "X", code = "KeyX", keyCode = 88, ctrlKey = false, metaKey = false;
				if (scenario === "arrow") { key = code = "ArrowLeft"; keyCode = 37; }
				if (scenario === "composition typing") { key = "Unidentified"; code = ""; keyCode = 229; }
				if (scenario.startsWith("home")) {
					metaKey = /Mac/.test(navigator.platform);
					ctrlKey = !metaKey;
					key = code = metaKey ? "ArrowUp" : "Home";
					keyCode = metaKey ? 38 : 36;
				}
				editor.dispatchEvent(new KeyboardEvent("keydown", { key, code, keyCode, ctrlKey, metaKey, bubbles: true, cancelable: true }));
				if (scenario === "composition typing") editor.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
				if (scenario.endsWith("typing")) document.execCommand("insertText", false, "X");
				if (scenario === "composition typing") editor.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "X" }));
			}
			if (scenario === "pointer") {
				editor.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
				saved.view.dispatch({ selection: { anchor: 0 }, scrollIntoView: true, userEvent: "select.pointer" });
			}
			if (scenario === "remote") window.__jotRemote(1_000_000, "PREFIX " + saved.text);
			if (scenario.endsWith("rapid")) {
				document.querySelector('[aria-controls="habits-panel"]').click();
				showJot();
			}
		}, scenario);
		await expect(content).toBeFocused();
		// Observe the completed return lifecycle, not a matching intermediate state.
		await state.evaluate((saved) => new Promise((resolve) => saved.view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) })));
		await expect.poll(() => state.evaluate((saved, scenario) => {
			const selection = getSelection();
			// Unrendered positions have no exact native DOM caret; CM's model remains authoritative.
			const nativeCaret = scenario !== "offscreen" && saved.view.contentDOM.contains(selection?.focusNode)
				? saved.view.posAtDOM(selection.focusNode, selection.focusOffset) : null;
			return { scroll: saved.view.scrollDOM.scrollTop, selection: saved.view.state.selection.toJSON(), nativeCaret, text: saved.view.state.doc.toString() };
		}, scenario)).toEqual(expected);
		await state.dispose();
		if (scenario.endsWith("typing")) {
			await expect.poll(() => writes.has(expected.text)).toBe(true);
			await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
			await page.reload();
			await expect.poll(() => page.evaluate(async () => {
				const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
				const editor = document.querySelector(".cm-editor");
				return editor ? EditorView.findFromDOM(editor)?.state.doc.toString() : null;
			})).toBe(expected.text);
		}
	});
}
