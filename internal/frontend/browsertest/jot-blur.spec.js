import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "Pacific/Kiritimati" });

test.beforeEach(async ({ context, page }) => {
 await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await expect(page.locator(".cm-editor")).toBeVisible();
});

test("Jotpad text survives blur during a pending save", async ({ page }) => {
	const content = page.locator(".cm-content");
	const text = "Keep this note";
	let releaseSave;
	const gate = new Promise((resolve) => { releaseSave = resolve; });
	await page.route("**/jot", async (route) => {
		await gate;
		await route.continue();
	});
	try {
		await content.click();
		const pending = page.waitForRequest((r) => new URL(r.url()).pathname === "/jot");
		await page.keyboard.insertText(text);
		await pending;
		await content.evaluate((element) => element.blur());
		const saved = page.waitForResponse((r) => new URL(r.url()).pathname === "/jot");
		releaseSave();
		expect((await saved).ok()).toBe(true);
		await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
		await page.reload({ waitUntil: "load" });
		await expect(content).toHaveText(text);
	} finally {
		releaseSave();
		await page.unroute("**/jot");
	}
});
