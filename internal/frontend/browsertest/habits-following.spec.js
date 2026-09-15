import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "UTC" });

async function addRolloverReplay(page, nextWeek, refresh) {
	await page.route("**/_test/habit-rollover*", (route) => route.fulfill({
		contentType: "text/event-stream",
		body: `event: datastar-patch-signals\ndata: signals ${JSON.stringify({ _habitcurrentweek: nextWeek, habitrefresh: refresh })}\n\n`,
	}));
	await page.evaluate(() => {
		const trigger = document.createElement("button");
		trigger.id = "rollover-test";
		trigger.textContent = "Replay rollover";
		trigger.setAttribute("data-on:click", "@get('/_test/habit-rollover')");
		document.body.append(trigger);
	});
}

function nextWeekAfter(week) {
	const next = new Date(`${week}T00:00:00Z`);
	next.setUTCDate(next.getUTCDate() + 7);
	return next.toISOString().slice(0, 10);
}

test("This week resumes following after Previous then Next, including rollover", async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await page.getByRole("tab", { name: "Habits", exact: true }).click();
	const heading = page.locator("#habit-grid .week-nav h2 time");
	await expect(heading).toBeVisible();
	const current = await heading.getAttribute("datetime");
	const thisWeek = page.getByRole("button", { name: "This week", exact: true });
	await expect(thisWeek).toBeDisabled();

	await page.getByRole("button", { name: /Previous week/ }).click();
	await expect(heading).not.toHaveAttribute("datetime", current);
	await page.getByRole("button", { name: "Next week", exact: true }).click();
	await expect(heading).toHaveAttribute("datetime", current);
	await expect(thisWeek).toBeEnabled();
	await thisWeek.click();
	await expect(thisWeek).toBeDisabled();

	// Replay the owner's week-rollover signal while stubbing the server clock's response.
	const nextWeek = nextWeekAfter(current);
	await addRolloverReplay(page, nextWeek, "f".repeat(32));
	await page.route("**/habits/week?*", (route) => {
		const signals = JSON.parse(new URL(route.request().url()).searchParams.get("datastar"));
		if (signals.habitweek === nextWeek) {
			return route.fulfill({ contentType: "text/event-stream", body: ": simulated future week\n\n" });
		}
		return route.continue();
	});
	const read = page.waitForRequest((request) => {
		const url = new URL(request.url());
		return url.pathname === "/habits/week" && JSON.parse(url.searchParams.get("datastar")).habitweek === nextWeek;
	});
	await page.locator("#rollover-test").click();
	await read;
	await expect(page.locator("#habit-grid")).toHaveAttribute("data-week", nextWeek);
});

test("a historical week remains selected across rollover", async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await page.getByRole("tab", { name: "Habits", exact: true }).click();
	const heading = page.locator("#habit-grid .week-nav h2 time");
	const current = await heading.getAttribute("datetime");
	await page.getByRole("button", { name: /Previous week/ }).click();
	await expect(heading).not.toHaveAttribute("datetime", current);
	const historical = await heading.getAttribute("datetime");

	const nextWeek = nextWeekAfter(current);
	const refresh = "e".repeat(32);
	await addRolloverReplay(page, nextWeek, refresh);
	const read = page.waitForRequest((request) => {
		const url = new URL(request.url());
		return url.pathname === "/habits/week" && JSON.parse(url.searchParams.get("datastar")).habitrefresh === refresh;
	});
	await page.locator("#rollover-test").click();
	const signals = JSON.parse(new URL((await read).url()).searchParams.get("datastar"));
	expect(signals.habitweek).toBe(historical);
	await expect(heading).toHaveAttribute("datetime", historical);
	await expect(page.locator("#habit-grid")).toHaveAttribute("data-week", historical);
});
