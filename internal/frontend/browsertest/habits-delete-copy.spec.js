import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 } });

test("delete confirmation copies literal names on every opening, including after a rename", async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await expect(page.locator(".cm-editor")).toBeVisible();
	await page.getByRole("tab", { name: "Habits", exact: true }).click();

	const original = `Read "books" & 'notes' <img src=x onerror=alert(1)>`;
	const renamed = `New <b>name</b> & "quotes" \\ path`;
	await page.locator('[commandfor="habit-create-dialog"][command="show-modal"]').click();
	await page.locator("#habit-name").fill(original);
	await page.locator('#habit-create button[type="submit"]').click();
	await expect(page.locator("#habit-create-dialog")).toBeHidden();

	const dialog = page.getByRole("dialog", { name: "Delete habit", exact: true });
	const copy = dialog.locator("#habit-delete-warning strong");
	async function openAndDismiss(name, key) {
		const invoker = page.getByRole("button", { name: `Delete ${name}`, exact: true });
		await invoker.focus();
		await page.keyboard.press("Enter");
		await expect(dialog).toBeVisible();
		expect(await dialog.evaluate((el) => el instanceof HTMLDialogElement && el.matches(":modal"))).toBe(true);
		await expect(copy).toHaveText(name);
		await expect(copy.locator("*")).toHaveCount(0);
		await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
		await page.keyboard.press(key);
		await expect(dialog).toBeHidden();
		await expect(invoker).toBeFocused();
	}

	await openAndDismiss(original, "Escape");
	await openAndDismiss(original, "Enter");
	await page.getByRole("button", { name: `Edit ${original}`, exact: true }).click();
	const editor = page.getByRole("dialog", { name: "Edit habit", exact: true });
	await editor.getByLabel("Name", { exact: true }).fill(renamed);
	await editor.getByRole("button", { name: "Save", exact: true }).click();
	await expect(editor).toBeHidden();
	await openAndDismiss(renamed, "Escape");

	await page.getByRole("button", { name: `Delete ${renamed}`, exact: true }).click();
	await expect(copy).toHaveText(renamed);
	const request = page.waitForRequest((r) => new URL(r.url()).pathname === "/habits/delete" && r.method() === "POST");
	await dialog.getByRole("button", { name: "Delete permanently", exact: true }).click();
	expect(Object.keys((await request).postDataJSON()).sort()).toEqual(["habitdeleteid", "habitdeleteview"]);
	await expect(dialog).toBeHidden();
	await expect(page.getByRole("button", { name: `Delete ${renamed}`, exact: true })).toHaveCount(0);
	await expect(page.locator("#habit-matrix")).toBeFocused();
});
