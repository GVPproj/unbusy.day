import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

for (const dependency of ["**/vendor/datastar/*.js", "**/js/jot/cm.js"]) {
	test(`startup waits for ${dependency}`, { tag: "@smoke" }, async ({ page, context }) => {
		await signIn(context);
		let release;
		const pending = new Promise(resolve => { release = resolve; });
		await page.route(dependency, async route => {
			await pending;
			await route.continue();
		});
		await page.goto(baseURL, { waitUntil: "commit" });
		await expect(page.getByRole("status").filter({ hasText: "Loading your day" })).toBeVisible();
		await expect(page.locator("#app-content")).toHaveAttribute("inert", "");
		release();
		await expect(page.locator("#app-loading")).toHaveCount(0);
		await expect(page.locator("#app-content")).not.toHaveAttribute("inert");
		await expect(page.locator(".cm-editor")).toBeVisible();
	});
}

test("failed JavaScript leaves a usable reload link", async ({ page, context }) => {
	await signIn(context);
	await page.route("**/js/jot/cm.js", route => route.abort());
	await page.goto(baseURL);
	await expect(page.locator("#app-loading")).toBeVisible();
	await expect(page.locator("#app-content")).toHaveAttribute("inert", "");
	await expect(page.getByRole("link", { name: "Reload app" })).toBeVisible();
	await expect(page.locator("#app-loading-message")).toContainText("taking longer", { timeout: 15000 });
	await page.unroute("**/js/jot/cm.js");
	await page.getByRole("link", { name: "Reload app" }).click();
	await expect(page.locator("#app-loading")).toHaveCount(0);
});
