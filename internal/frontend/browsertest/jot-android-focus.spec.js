import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 } });

test.describe("Jotpad Android JS-branch emulation (not a real Android OS test)", () => {
	for (const scenario of ["229 composition replaces selection", "delayed Backspace deletes before caret"]) {
		test(scenario, async ({ context, page }) => {
			const composing = scenario.startsWith("229");
			await page.addInitScript(() => {
				Object.defineProperty(navigator, "userAgent", { value: `${navigator.userAgent} Android` });
				Object.defineProperty(window, "EditContext", { value: undefined });
			});
			await signIn(context);
			await page.goto(baseURL, { waitUntil: "load" });
			expect(await page.evaluate(() => navigator.userAgent)).toMatch(/Chrome\/.*Android\b/);
			const note = Array.from({ length: 100 }, (_, i) => `Line ${i + 1}: keep this note`).join("\n");
			const content = page.locator(".cm-content");
			await content.click();
			await page.keyboard.insertText(note);
			await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
			const state = await page.evaluateHandle(async (composing) => {
				const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
				const view = EditorView.findFromDOM(document.querySelector(".cm-editor"));
				view.dispatch({ selection: { anchor: composing ? 2386 : 2390, head: 2390 }, scrollIntoView: true });
				await new Promise((resolve) => view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) }));
				return { view, scroll: view.scrollDOM.scrollTop, selection: view.state.selection.toJSON(), text: view.state.doc.toString() };
			}, composing);
			expect(await state.evaluate((saved) => saved.view.contentDOM.editContext == null)).toBe(true);
			expect(await state.evaluate((saved) => saved.selection)).toEqual({ ranges: [{ anchor: composing ? 2386 : 2390, head: 2390 }], main: 0 });
			expect(await state.evaluate((saved) => saved.text)).toBe(note);
			expect(await state.evaluate((saved) => saved.scroll)).toBeGreaterThan(0);
			await page.getByRole("tab", { name: "Habits", exact: true }).click();
			await expect.poll(() => state.evaluate((saved) => saved.view.inView)).toBe(false);
			await page.locator('[commandfor="habit-create-dialog"][command="show-modal"]').click();
			await page.locator("#habit-create").getByLabel("Name", { exact: true }).fill("Another input");
			await page.keyboard.press("Escape");
			const expected = await state.evaluate((saved, composing) => {
				const selection = structuredClone(saved.selection), range = selection.ranges[selection.main];
				const from = composing ? range.anchor : range.head - 1;
				const text = saved.text.slice(0, from) + (composing ? "X" : "") + saved.text.slice(range.head);
				range.anchor = range.head = from + (composing ? 1 : 0);
				return { scroll: saved.scroll, selection, nativeCaret: range.head, text };
			}, composing);
			const replay = await state.evaluate((saved, composing) => {
				const editor = saved.view.contentDOM;
				let replayed = false;
				editor.addEventListener("focus", () => {
					getSelection().collapse(editor, 0);
					replayed = true;
				}, { once: true });
				document.querySelector('[aria-controls="jot-panel"]').click();
				document.querySelector("#jot-panel").focus();
				editor.focus();
				const selection = getSelection();
				const nativeCaret = saved.view.posAtDOM(selection.focusNode, selection.focusOffset);
				editor.dispatchEvent(new KeyboardEvent("keydown", {
					key: composing ? "Unidentified" : "Backspace", code: composing ? "" : "Backspace",
					keyCode: composing ? 229 : 8, bubbles: true, cancelable: true,
				}));
				if (composing) {
					editor.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
					editor.dispatchEvent(new InputEvent("beforeinput", { inputType: "insertCompositionText", data: "X", isComposing: true, bubbles: true }));
					document.execCommand("insertText", false, "X");
					editor.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "X" }));
				} else if (editor.dispatchEvent(new InputEvent("beforeinput", { inputType: "deleteContentBackward", bubbles: true, cancelable: true }))) {
					document.execCommand("delete");
				}
				return { replayed, nativeCaret };
			}, composing);
			expect(replay).toEqual({ replayed: true, nativeCaret: 0 });
			await expect(content).toBeFocused();
			await state.evaluate((saved) => new Promise((resolve) => saved.view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) })));
			await expect.poll(() => state.evaluate((saved) => {
				const selection = getSelection();
				const nativeCaret = saved.view.contentDOM.contains(selection?.focusNode)
					? saved.view.posAtDOM(selection.focusNode, selection.focusOffset) : null;
				return { scroll: saved.view.scrollDOM.scrollTop, selection: saved.view.state.selection.toJSON(), nativeCaret, text: saved.view.state.doc.toString() };
			})).toEqual(expected);
			await state.dispose();
		});
	}
});
