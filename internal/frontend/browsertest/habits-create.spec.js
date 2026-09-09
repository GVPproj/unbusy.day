import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "UTC" });

test.beforeEach(async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await expect(page.locator(".cm-editor")).toBeVisible();
	await page.getByRole("tab", { name: "Habits", exact: true }).click();
});

const dialog = (page) => page.locator("#habit-create-dialog");
const name = (page) => dialog(page).getByLabel("Name", { exact: true });
const start = (page) => dialog(page).getByLabel("Start date", { exact: true });
const submit = (page) => dialog(page).getByRole("button", { name: "Create habit", exact: true });
const row = (page, label) => page.getByRole("rowheader", { name: label, exact: true });

async function open(page) {
	await page.locator('[commandfor="habit-create-dialog"][command="show-modal"]').click();
	await expect(dialog(page)).toBeVisible();
}

test("creation rejection stays with its opening and reopening clears feedback without losing the draft", async ({ page }) => {
	await open(page);
	await name(page).fill("x".repeat(81));
	await start(page).fill("2020-01-01");
	await submit(page).click();
	await expect(dialog(page)).toBeVisible();
	await expect(page.locator("#habit-feedback")).toContainText("80");
	await expect(name(page)).toHaveValue("x".repeat(81));
	await expect(start(page)).toHaveValue("2020-01-01");
	await dialog(page).getByRole("button", { name: "Cancel", exact: true }).click();
	await open(page);
	await expect(page.locator("#habit-feedback")).toBeEmpty();
	await expect(name(page)).toHaveValue("x".repeat(81));
	await page.keyboard.press("Escape");
	await open(page);
	await name(page).fill("Corrected draft");
	const request = page.waitForRequest((request) => new URL(request.url()).pathname === "/habits" && request.method() === "POST");
	await submit(page).click();
	expect((await request).postDataJSON().habitcreateview).toBe(3);
	await expect(dialog(page)).toBeHidden();
	await expect(row(page, "Corrected draft")).toBeVisible();
});

for (const outcome of ["success", "rejection"]) {
	test(`a delayed creation ${outcome} affects only its submitting opening`, async ({ page }) => {
		await open(page);
		await name(page).fill(outcome === "success" ? "First creation" : "x".repeat(81));
		await start(page).fill("2020-01-01");
		let release;
		const gate = new Promise((resolve) => { release = resolve; });
		let received;
		const held = new Promise((resolve) => { received = resolve; });
		let firstSignals;
		await page.route("**/habits", async (route) => {
			firstSignals = route.request().postDataJSON();
			const response = await route.fetch();
			received();
			await gate;
			await route.fulfill({ response });
		});
		try {
			await submit(page).click();
			await held;
			await expect(submit(page)).toBeDisabled();
			if (outcome === "success") await expect(row(page, "First creation")).toBeVisible();
			await page.keyboard.press("Escape");
			await expect(dialog(page)).toBeHidden();
			await open(page);
			await name(page).fill("Newer draft");
			await start(page).fill("2020-02-02");
			release();
			// The indicator clears after Datastar has consumed the held SSE response.
			await expect(dialog(page).locator('button[type="submit"]')).toBeEnabled();
			await expect(dialog(page)).toBeVisible();
			await expect(name(page)).toHaveValue("Newer draft");
			await expect(start(page)).toHaveValue("2020-02-02");
			await expect(page.locator("#habit-feedback")).toBeEmpty();
			await page.unroute("**/habits");

			const next = page.waitForRequest((request) => new URL(request.url()).pathname === "/habits" && request.method() === "POST");
			await submit(page).click();
			const nextSignals = (await next).postDataJSON();
			expect(firstSignals.habitcreateview).toBe(1);
			expect(nextSignals.habitcreateview).toBe(2);
			await expect(dialog(page)).toBeHidden();
			await expect(row(page, "Newer draft")).toBeVisible();
			await open(page);
			await expect(page.locator("#habit-feedback")).toBeEmpty();
			await expect(name(page)).toHaveValue("Newer draft");
		} finally {
			release();
			await page.unrouteAll({ behavior: "wait" });
		}
	});
}
