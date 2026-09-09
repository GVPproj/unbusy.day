import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "UTC" });

test("This month resumes following after Previous then Next, including rollover", async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await page.getByRole("tab", { name: "Habits", exact: true }).click();
	const heading = page.locator("#habit-grid .month-nav h2 time");
	await expect(heading).toBeVisible();
	const current = await heading.getAttribute("datetime");
	const thisMonth = page.getByRole("button", { name: "This month", exact: true });
	await expect(thisMonth).toBeDisabled();

	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(heading).not.toHaveAttribute("datetime", current);
	await page.getByRole("button", { name: "Next month", exact: true }).click();
	await expect(heading).toHaveAttribute("datetime", current);
	await expect(thisMonth).toBeEnabled();
	await thisMonth.click();
	await expect(thisMonth).toBeDisabled();

	// Replay the owner's month-rollover signal without changing the server clock.
	const next = new Date(`${current}-01T00:00:00Z`);
	next.setUTCMonth(next.getUTCMonth() + 1);
	const nextMonth = next.toISOString().slice(0, 7);
	await page.route("**/_test/habit-rollover*", (route) => route.fulfill({
		contentType: "text/event-stream",
		body: `event: datastar-patch-signals\ndata: signals ${JSON.stringify({ _habitcurrent: nextMonth, habitrefresh: "f".repeat(32) })}\n\n`,
	}));
	await page.evaluate(() => {
		const trigger = document.createElement("button");
		trigger.id = "rollover-test";
		trigger.textContent = "Replay rollover";
		trigger.setAttribute("data-on:click", "@get('/_test/habit-rollover')");
		document.body.append(trigger);
	});
	const read = page.waitForRequest((request) => {
		const url = new URL(request.url());
		return url.pathname === "/habits/month" && JSON.parse(url.searchParams.get("datastar")).habitmonth === nextMonth;
	});
	await page.locator("#rollover-test").click();
	await read;
	await expect(page.locator("#habit-grid")).toHaveAttribute("data-month", nextMonth);
});
