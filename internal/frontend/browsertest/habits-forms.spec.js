import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "America/Los_Angeles" });

async function loadBeforeMidnight(context, page) {
	await signIn(context);
	await page.clock.setFixedTime(new Date("2025-01-01T07:59:00Z"));
	await page.goto(baseURL, { waitUntil: "load" });
	await expect(page.locator(".cm-editor")).toBeVisible();
	await page.getByRole("tab", { name: "Habits", exact: true }).click();
}

async function open(page) {
	await page.locator('[commandfor="habit-create-dialog"][command="show-modal"]').click();
	await expect(page.locator("#habit-create-dialog")).toBeVisible();
}

test("first opening after local midnight refreshes untouched creation defaults across the year boundary", async ({ context, page }) => {
	await loadBeforeMidnight(context, page);
	await expect(page.locator("#habit-start")).toHaveValue("2024-12-31");
	await page.clock.setFixedTime(new Date("2025-01-01T08:01:00Z"));
	await open(page);
	await expect(page.locator("#habit-start")).toHaveValue("2025-01-01");
	await page.locator("#habit-name").fill("Local new year");
	const request = page.waitForRequest((r) => new URL(r.url()).pathname === "/habits" && r.method() === "POST");
	await page.locator('#habit-create button[type="submit"]').click();
	expect((await request).postDataJSON().habitstart).toBe("2025-01-01");
	await expect(page.locator("#habit-create-dialog")).toBeHidden();
});

for (const draft of ["name", "backdated date", "explicit same date"]) {
	test(`${draft} draft survives local midnight while open and after dismissal`, async ({ context, page }) => {
		await loadBeforeMidnight(context, page);
		await open(page);
		if (draft === "name") await page.locator("#habit-name").fill("Nightly reading");
		else {
			await page.locator("#habit-start").click();
			await page.locator("#habit-start").fill(draft === "backdated date" ? "2024-12-01" : "2024-12-31");
		}
		const expectedDate = draft === "backdated date" ? "2024-12-01" : "2024-12-31";
		await page.clock.setFixedTime(new Date("2025-01-01T08:01:00Z"));
		await expect(page.locator("#habit-start")).toHaveValue(expectedDate);
		await page.locator('#habit-create button[command="close"]').click();
		await open(page);
		await expect(page.locator("#habit-start")).toHaveValue(expectedDate);
		await expect(page.locator("#habit-name")).toHaveValue(draft === "name" ? "Nightly reading" : "");
	});
}

test("opening and dismissing without drafting does not claim the date default", async ({ context, page }) => {
	await loadBeforeMidnight(context, page);
	await open(page);
	await expect(page.locator("#habit-start")).toHaveValue("2024-12-31");
	await page.keyboard.press("Escape");
	await page.clock.setFixedTime(new Date("2025-01-01T08:01:00Z"));
	await open(page);
	await expect(page.locator("#habit-start")).toHaveValue("2025-01-01");
	await page.keyboard.press("Escape");
	await page.clock.setFixedTime(new Date("2025-01-02T08:01:00Z"));
	await open(page);
	await expect(page.locator("#habit-start")).toHaveValue("2025-01-02");
});
