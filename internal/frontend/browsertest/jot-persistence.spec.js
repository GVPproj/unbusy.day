import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test("Jotpad edits converge across tabs and survive reload", { tag: "@smoke" }, async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });

	const text = "Keep this note across tabs";
	await page.locator(".cm-content").click();
	const saved = page.waitForResponse((response) =>
		new URL(response.url()).pathname === "/jot" && response.request().method() === "POST",
	);
	await page.keyboard.insertText(text);
	expect((await saved).ok()).toBe(true);
	await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
	await expect(other.locator(".cm-content")).toHaveText(text);

	await other.locator(".cm-content").click();
	await other.keyboard.press("ControlOrMeta+End");
	const updated = other.waitForResponse((response) =>
		new URL(response.url()).pathname === "/jot" && response.request().method() === "POST",
	);
	await other.keyboard.insertText(" — updated");
	expect((await updated).ok()).toBe(true);
	await expect(other.locator("#companion-status")).toHaveAttribute("data-state", "saved");
	await expect(page.locator(".cm-content")).toHaveText(`${text} — updated`);

	await page.reload({ waitUntil: "load" });
	await expect(page.locator(".cm-content")).toHaveText(`${text} — updated`);
});
