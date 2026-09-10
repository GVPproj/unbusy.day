import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 } });

test.describe("Jotpad Android JS-branch emulation (not a real Android OS test)", () => {
	const scenarios = [
		{ name: "229 composition replaces selection", composing: true },
		{ name: "delayed Backspace deletes before caret", composing: false },
		{ name: "composition after deferred selectionchange", composing: true, notify: true },
		{ name: "Backspace after deferred selectionchange", composing: false, notify: true },
		{ name: "composition replaces remotely mapped selection", composing: true, remote: true },
		{ name: "Backspace uses remotely mapped caret", composing: false, remote: true },
		{ name: "composition without keydown replaces selection", composing: true, noKey: true },
		{ name: "Backspace without beforeinput uses delayed fallback", composing: false, keyOnly: true },
		{ name: "beforeinput-only Backspace restores caret before deletion", composing: false, noKey: true },
		{ name: "idle refocus reconciles selected range", composing: true, idle: true },
	];
	for (const scenario of scenarios) {
		test(scenario.name, async ({ context, page }) => {
			const { composing } = scenario;
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
			if (scenario.remote) {
				await state.evaluate((saved) => {
					window.__jotRemote(1_000_000, "PREFIX " + saved.text);
					saved.text = saved.view.state.doc.toString();
					saved.selection = saved.view.state.selection.toJSON();
				});
				expect(await state.evaluate((saved) => saved.text)).toBe("PREFIX " + note);
				expect(await state.evaluate((saved) => saved.selection)).toEqual({ ranges: [{ anchor: (composing ? 2386 : 2390) + 7, head: 2397 }], main: 0 });
			}
			const expected = await state.evaluate((saved, { composing, idle }) => {
				if (idle) return { scroll: saved.scroll, selection: saved.selection, nativeCaret: 2390, text: saved.text };
				const selection = structuredClone(saved.selection), range = selection.ranges[selection.main];
				const from = composing ? range.anchor : range.head - 1;
				const text = saved.text.slice(0, from) + (composing ? "X" : "") + saved.text.slice(range.head);
				range.anchor = range.head = from + (composing ? 1 : 0);
				return { scroll: saved.scroll, selection, nativeCaret: range.head, text };
			}, scenario);
			const replay = await state.evaluate((saved, { composing, notify, noKey, keyOnly, idle }) => {
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
				if (notify) {
					document.dispatchEvent(new Event("selectionchange"));
					document.dispatchEvent(new Event("selectionchange"));
				}
				if (idle) return { replayed, nativeCaret };
				if (!noKey) editor.dispatchEvent(new KeyboardEvent("keydown", {
					key: composing ? "Unidentified" : "Backspace", code: composing ? "" : "Backspace",
					keyCode: composing ? 229 : 8, bubbles: true, cancelable: true,
				}));
				const capturePrepared = () => {
					const native = getSelection();
					saved.prepared = {
						selection: saved.view.state.selection.toJSON(), text: saved.view.state.doc.toString(),
						anchor: saved.view.posAtDOM(native.anchorNode, native.anchorOffset),
						head: saved.view.posAtDOM(native.focusNode, native.focusOffset),
					};
				};
				if (!noKey) capturePrepared();
				if (composing) {
					editor.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
					if (noKey) capturePrepared();
					editor.dispatchEvent(new InputEvent("beforeinput", { inputType: "insertCompositionText", data: "X", isComposing: true, bubbles: true }));
					document.execCommand("insertText", false, "X");
					editor.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "X" }));
				} else if (!keyOnly) {
					const allowDelete = editor.dispatchEvent(new InputEvent("beforeinput", { inputType: "deleteContentBackward", bubbles: true, cancelable: true }));
					if (noKey) capturePrepared();
					if (allowDelete) document.execCommand("delete");
				}
				return { replayed, nativeCaret };
			}, scenario);
			expect(replay).toEqual({ replayed: true, nativeCaret: 0 });
			if (!scenario.idle) expect(await state.evaluate((saved) => saved.prepared)).toEqual(await state.evaluate((saved) => ({
				selection: saved.selection, text: saved.text,
				anchor: saved.selection.ranges[0].anchor, head: saved.selection.ranges[0].head,
			})));
			await expect(content).toBeFocused();
			await state.evaluate((saved) => new Promise((resolve) => saved.view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) })));
			await expect.poll(() => state.evaluate((saved) => {
				const selection = getSelection();
				const nativeCaret = saved.view.contentDOM.contains(selection?.focusNode)
					? saved.view.posAtDOM(selection.focusNode, selection.focusOffset) : null;
				return { scroll: saved.view.scrollDOM.scrollTop, selection: saved.view.state.selection.toJSON(), nativeCaret, text: saved.view.state.doc.toString() };
			})).toEqual(expected);
			if (scenario.idle) expect(await state.evaluate((saved) => {
				const selection = getSelection();
				return saved.view.posAtDOM(selection.anchorNode, selection.anchorOffset);
			})).toBe(2386);
			await state.dispose();
		});
	}
});
