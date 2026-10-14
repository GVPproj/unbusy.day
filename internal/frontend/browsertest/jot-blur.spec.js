import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "Pacific/Kiritimati" });

test.beforeEach(async ({ context, page }) => {
 await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await expect(page.locator("#jot-daily-panel .cm-editor")).toBeVisible();
});

test("Jotpad text survives blur during a pending save", { tag: "@smoke" }, async ({ page }) => {
	const content = page.locator("#jot-daily-panel .cm-content");
	const text = "Keep this note";
	let releaseSave;
	let saveRequests = 0;
	const gate = new Promise((resolve) => { releaseSave = resolve; });
	await page.route("**/jot?note=daily", async (route) => {
		saveRequests++;
		await gate;
		await route.continue();
	});
	try {
		await content.click();
		const pending = page.waitForRequest((r) => new URL(r.url()).pathname === "/jot");
		await page.keyboard.insertText(text);
		await pending;
		await content.evaluate((element) => element.blur());
		await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saving");
		const saved = page.waitForResponse((r) => new URL(r.url()).pathname === "/jot");
		releaseSave();
		expect((await saved).ok()).toBe(true);
		await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
		expect(saveRequests).toBe(1);
		await page.reload({ waitUntil: "load" });
		await expect(content).toHaveText(text);
	} finally {
		releaseSave();
		await page.unroute("**/jot?note=daily");
	}
});
