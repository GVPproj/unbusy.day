import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 } });

test("Jotpad preserves scroll when refocused before its redisplay measurement", async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	const content = page.locator(".cm-content");
	await content.click();
	await page.keyboard.insertText(Array.from({ length: 100 }, (_, i) => `Line ${i + 1}: keep this note`).join("\n"));
	await page.keyboard.press("ArrowLeft");
	await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
	const state = await page.evaluateHandle(async () => {
		const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
		const view = EditorView.findFromDOM(document.querySelector(".cm-editor"));
		return { view, scroll: view.scrollDOM.scrollTop, selection: view.state.selection.toJSON(), text: view.state.doc.toString() };
	});
	expect(await state.evaluate((saved) => saved.scroll)).toBeGreaterThan(0);
	await page.getByRole("tab", { name: "Habits", exact: true }).click();
	await expect.poll(() => state.evaluate((saved) => saved.view.inView)).toBe(false);
	await page.locator('[commandfor="habit-create-dialog"][command="show-modal"]').click();
	await page.locator("#habit-create").getByLabel("Name", { exact: true }).fill("Another input");
	await page.keyboard.press("Escape");
	// Put native refocus in the same frame as showing, rather than relying on runner speed.
	await page.evaluate(() => {
		document.querySelector('[aria-controls="jot-panel"]').click();
		document.querySelector("#jot-panel").focus();
		document.querySelector(".cm-content").focus();
	});
	await expect(content).toBeFocused();
	// Scroll can settle before CM's asynchronous focus/selection update.
	await expect.poll(() => state.evaluate((saved) => {
		const selection = getSelection();
		const nativeCaret = saved.view.contentDOM.contains(selection?.focusNode)
			? saved.view.posAtDOM(selection.focusNode, selection.focusOffset) : null;
		return {
			scroll: saved.view.scrollDOM.scrollTop,
			selection: saved.view.state.selection.toJSON(),
			nativeCaret,
			text: saved.view.state.doc.toString(),
		};
	})).toEqual(await state.evaluate((saved) => ({
		scroll: saved.scroll,
		selection: saved.selection,
		nativeCaret: saved.selection.ranges[saved.selection.main].head,
		text: saved.text,
	})));
	await state.dispose();
});
