import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "Pacific/Kiritimati" });

test.beforeEach(async ({ context, page }) => {
 await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });
	await expect(page.locator(".cm-editor")).toBeVisible();
});

const jotTab = (page) => page.getByRole("tab", { name: "Jotpad", exact: true });
const habitsTab = (page) => page.getByRole("tab", { name: "Habits", exact: true });
const habitName = (page) => page.locator("#habit-create").getByLabel("Name", { exact: true });
const habitStart = (page) => page.locator("#habit-create").getByLabel("Start date", { exact: true });
const habitRow = (page, name) => page.locator("#habit-matrix tbody tr").filter({ has: page.getByRole("rowheader", { name, exact: true }) });
const checkIn = (page, name, date) => page.getByRole("button", { name: `${name} on ${date}`, exact: true });

async function localCalendar(page) {
	return page.evaluate(() => {
		const now = new Date();
		const date = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
		return {
			today: date(now),
			tomorrow: date(new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)),
			day: now.getDate(),
			days: new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate(),
			month: now.toLocaleDateString("en-US", { month: "long", year: "numeric" }),
			beforePrevious: new Date(now.getFullYear(), now.getMonth() - 2, 1).toLocaleDateString("en-US", { month: "long", year: "numeric" }),
			previous: (() => {
				const first = new Date(now.getFullYear(), now.getMonth() - 1, 1);
				const last = new Date(now.getFullYear(), now.getMonth(), 0);
				return {
					first: date(first),
					last: date(last),
					days: last.getDate(),
					month: first.toLocaleDateString("en-US", { month: "long", year: "numeric" }),
				};
			})(),
		};
	});
}

async function createHabit(page, name, start) {
	await habitName(page).fill(name);
	if (start) await habitStart(page).fill(start);
	await page.locator("#habit-create").getByRole("button", { name: /create|add/i }).click();
	await expect(habitRow(page, name)).toBeVisible();
}

async function previousMonth(page) {
	const heading = page.locator("#habit-grid .month-nav h2");
	const before = await heading.textContent();
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(heading).not.toHaveText(before);
}

async function mobilePanel(page, name) {
	await page.locator(".menu-toggle").click();
	await page.locator("#sidenav").getByRole("button", { name, exact: true }).click();
 await expect(page.locator(".menu-toggle")).toHaveAttribute("aria-expanded", "false");
 await expect.poll(() => page.locator("#sidenav").evaluate((nav) => nav.getBoundingClientRect().left >= innerWidth)).toBe(true);
}

test("desktop defaults to Jotpad beside the plan; tabs have roving keyboard selection", async ({ page }) => {
	await expect(page.locator("#block-list")).toBeVisible();
	await expect(jotTab(page)).toHaveAttribute("aria-selected", "true");
	await expect(jotTab(page)).toHaveAttribute("tabindex", "0");
	await expect(habitsTab(page)).toHaveAttribute("aria-selected", "false");
	await expect(habitsTab(page)).toHaveAttribute("tabindex", "-1");
	const plan = await page.locator("#block-list").boundingBox();
	const editor = await page.locator(".cm-editor").boundingBox();
	expect(editor.x).toBeGreaterThanOrEqual(plan.x + plan.width);

	await jotTab(page).focus();
	for (const [key, active, inactive] of [
		["ArrowRight", habitsTab(page), jotTab(page)],
		["ArrowRight", jotTab(page), habitsTab(page)],
		["ArrowLeft", habitsTab(page), jotTab(page)],
		["Home", jotTab(page), habitsTab(page)],
		["End", habitsTab(page), jotTab(page)],
		["ArrowLeft", jotTab(page), habitsTab(page)],
	]) {
		await page.keyboard.press(key);
		await expect(active).toBeFocused();
		await expect(active).toHaveAttribute("aria-selected", "true");
		await expect(active).toHaveAttribute("tabindex", "0");
		await expect(inactive).toHaveAttribute("aria-selected", "false");
		await expect(inactive).toHaveAttribute("tabindex", "-1");
		const panel = page.locator(`#${await active.getAttribute("aria-controls")}`);
		await expect(panel).toHaveAttribute("role", "tabpanel");
		await expect(panel).toHaveAttribute("aria-labelledby", await active.getAttribute("id"));
		await expect(panel).toBeVisible();
	}
});

test("creation defaults to browser-local today and persists the current-month availability matrix", async ({ page }) => {
	await habitsTab(page).click();
	const calendar = await localCalendar(page);
 await expect(page.locator("#habit-matrix")).toContainText("No habits yet");
	await expect(habitStart(page)).toHaveValue(calendar.today);
	await createHabit(page, "Read books");
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	const row = habitRow(page, "Read books");
	await expect(row.locator("td")).toHaveCount(calendar.days);
	for (let day = 1; day <= calendar.days; day++) {
		const cell = row.locator("td").nth(day - 1);
		if (day === calendar.day) {
			await expect(cell.getByRole("button", { name: `Read books on ${calendar.today}` })).toHaveAttribute("aria-pressed", "false");
		} else {
			await expect(cell.locator("[aria-label]")).toHaveAttribute("aria-label", /: unavailable$/);
			await expect(cell.getByRole("button")).toHaveCount(0);
		}
		await expect(cell.getByRole("checkbox")).toHaveCount(0);
	}
	await page.reload({ waitUntil: "load" });
 await expect(jotTab(page)).toHaveAttribute("aria-selected", "true");
	await habitsTab(page).click();
	await expect(habitRow(page, "Read books")).toBeVisible();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	await expect(habitRow(page, "Read books").locator("td")).toHaveCount(calendar.days);
 await page.screenshot({ path: test.info().outputPath("habits-desktop.png"), fullPage: true });
});

test("historical check-ins survive reload and This month resumes the present", async ({ page }) => {
	await habitsTab(page).click();
	const calendar = await localCalendar(page);
	await createHabit(page, "History", calendar.previous.first);
	await page.getByRole("button", { name: /Previous month/ }).focus();
	await page.keyboard.press("Enter");
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.previous.month);
	await expect(page.locator("#habit-previous-month")).toBeFocused();
	await expect(habitRow(page, "History").locator("td")).toHaveCount(calendar.previous.days);
	await checkIn(page, "History", calendar.previous.last).click();
	await expect(checkIn(page, "History", calendar.previous.last)).toHaveAttribute("aria-pressed", "true");

	await page.reload({ waitUntil: "load" });
	await habitsTab(page).click();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(checkIn(page, "History", calendar.previous.last)).toHaveAttribute("aria-pressed", "true");
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(habitRow(page, "History")).toHaveCount(0);
	await page.getByRole("button", { name: "Next month", exact: true }).click();
	await expect(habitRow(page, "History")).toBeVisible();
	await page.getByRole("button", { name: "This month", exact: true }).click();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	await expect(page.getByRole("button", { name: "Next month", exact: true })).toBeDisabled();
});

test("leap February and December/January navigation render real calendars", async ({ page }) => {
	await habitsTab(page).click();
	await createHabit(page, "Long history", "2020-01-01");
	const heading = page.locator("#habit-grid .month-nav h2");
	for (let steps = 0; await heading.textContent() !== "January 2025" && steps < 60; steps++) {
		await previousMonth(page);
	}
	await expect(heading).toHaveText("January 2025");
	await expect(habitRow(page, "Long history").locator("td")).toHaveCount(31);
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(heading).toHaveText("December 2024");
	await expect(habitRow(page, "Long history").locator("td")).toHaveCount(31);
	for (let steps = 0; await heading.textContent() !== "February 2024" && steps < 12; steps++) {
		await previousMonth(page);
	}
	await expect(heading).toHaveText("February 2024");
	await expect(habitRow(page, "Long history").locator("td")).toHaveCount(29);
});

test("different views keep their selected month during live corrections", async ({ page, context }) => {
	await habitsTab(page).click();
	const calendar = await localCalendar(page);
	await createHabit(page, "Across devices", calendar.previous.first);
	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });
	await habitsTab(other).click();

	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.previous.month);
	await checkIn(other, "Across devices", calendar.today).click();
	await expect(checkIn(other, "Across devices", calendar.today)).toHaveAttribute("aria-pressed", "true");
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.previous.month);

	await checkIn(page, "Across devices", calendar.previous.last).click();
	await expect(checkIn(page, "Across devices", calendar.previous.last)).toHaveAttribute("aria-pressed", "true");
	await expect(other.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	await expect(checkIn(other, "Across devices", calendar.today)).toHaveAttribute("aria-pressed", "true");
	await other.close();
});

test("rapid month requests cannot apply an older response", async ({ page }) => {
	await habitsTab(page).click();
	const calendar = await localCalendar(page);
	await createHabit(page, "Rapid navigation", calendar.previous.first);
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.previous.month);

	let release;
	const gate = new Promise((resolve) => { release = resolve; });
	await page.route("**/habits/month*", async (route) => {
		const signals = JSON.parse(new URL(route.request().url()).searchParams.get("datastar"));
		if (signals.habitmonth === calendar.today.slice(0, 7)) await gate;
		await route.continue();
	});
	const delayed = page.waitForRequest((request) => {
		if (new URL(request.url()).pathname !== "/habits/month") return false;
		return JSON.parse(new URL(request.url()).searchParams.get("datastar")).habitmonth === calendar.today.slice(0, 7);
	});
	await page.getByRole("button", { name: "Next month", exact: true }).click();
	await delayed;
	await page.getByRole("button", { name: /Previous month/ }).click();
	release();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.beforePrevious);
	await page.unroute("**/habits/month*");
});

test("an in-flight historical correction cannot replace the month navigated to", async ({ page }) => {
	await habitsTab(page).click();
	const calendar = await localCalendar(page);
	await createHabit(page, "Slow history", calendar.previous.first);
	await page.getByRole("button", { name: /Previous month/ }).click();
	let release;
	const gate = new Promise((resolve) => { release = resolve; });
	await page.route("**/habits/check-in", async (route) => {
		await gate;
		await route.continue();
	});
	await checkIn(page, "Slow history", calendar.previous.last).click();
	await expect(checkIn(page, "Slow history", calendar.previous.last)).toHaveAttribute("aria-busy", "true");
	await page.getByRole("button", { name: "This month", exact: true }).click();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	const saved = page.waitForResponse((response) => new URL(response.url()).pathname === "/habits/check-in");
	release();
	expect((await saved).ok()).toBe(true);
	await page.unroute("**/habits/check-in");
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(checkIn(page, "Slow history", calendar.previous.last)).toHaveAttribute("aria-pressed", "true");
});

test("check-ins wait for confirmation, survive reload, expose failure, and retry explicitly", async ({ page }) => {
	await habitsTab(page).click();
	const { today } = await localCalendar(page);
	await createHabit(page, "Journal", today);
	let button = checkIn(page, "Journal", today);

	let release;
	const gate = new Promise((resolve) => { release = resolve; });
	await page.route("**/habits/check-in", async (route) => {
		await gate;
		await route.continue();
	});
	const request = page.waitForRequest((req) => new URL(req.url()).pathname === "/habits/check-in");
	await button.click();
	await request;
	await expect(button).toHaveAttribute("aria-pressed", "false");
	await expect(button).toHaveAttribute("aria-busy", "true");
	await expect(page.locator("#habit-checkin-feedback")).toHaveText("Saving…");
	release();
	await expect(button).toHaveAttribute("aria-pressed", "true");
	await expect(page.locator("#habit-checkin-feedback")).toHaveText("Saved.");
	await page.unroute("**/habits/check-in");

	await page.reload({ waitUntil: "load" });
	await habitsTab(page).click();
	button = checkIn(page, "Journal", today);
	await expect(button).toHaveAttribute("aria-pressed", "true");

	await page.route("**/habits/check-in", (route) => route.fulfill({ status: 500, body: "failed" }));
	await button.click();
	await expect(button).toHaveAttribute("aria-pressed", "true");
	await expect(button).toHaveAttribute("data-save-state", "failed");
	await expect(page.locator("#habit-checkin-feedback")).toContainText(/not saved.*retry/i);
	await page.unroute("**/habits/check-in");
	await button.click();
	await expect(button).toHaveAttribute("aria-pressed", "false");

	await button.focus();
	await page.keyboard.press("Space");
	await expect(button).toHaveAttribute("aria-pressed", "true");
	await page.keyboard.press("Enter");
	await expect(button).toHaveAttribute("aria-pressed", "false");
});

test("live check-ins converge while preserving focused date and horizontal scroll", async ({ page, context }) => {
	await habitsTab(page).click();
	const { today } = await localCalendar(page);
	await createHabit(page, "Stretch", `${today.slice(0, 8)}01`);
	const matrix = page.locator("#habit-matrix");
	const button = checkIn(page, "Stretch", today);
	await button.focus();
	await matrix.evaluate((element) => { element.scrollLeft = element.scrollWidth; });
	const scroll = await matrix.evaluate((element) => element.scrollLeft);

	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });
	await habitsTab(other).click();
	await checkIn(other, "Stretch", today).click();
	await expect(button).toHaveAttribute("aria-pressed", "true");
	await expect(button).toBeFocused();
	expect(Math.abs(await matrix.evaluate((element) => element.scrollLeft) - scroll)).toBeLessThan(2);

	await button.click();
	await expect(button).toHaveAttribute("aria-pressed", "false");
	await expect(checkIn(other, "Stretch", today)).toHaveAttribute("aria-pressed", "false");
	await expect(button).toBeFocused();
	expect(Math.abs(await matrix.evaluate((element) => element.scrollLeft) - scroll)).toBeLessThan(2);
	await other.close();
});

test("duplicate and future-start errors retain the submitted name and date", async ({ page }) => {
	await habitsTab(page).click();
	const { today, tomorrow } = await localCalendar(page);
	await createHabit(page, "Walk", today);
	await habitName(page).fill("Walk");
	await habitStart(page).fill(today);
	await page.getByRole("button", { name: "Create habit", exact: true }).click();
	await expect(page.locator("#habit-feedback")).toContainText(/already|duplicate/i);
	await expect(habitName(page)).toHaveValue("Walk");
	await expect(habitStart(page)).toHaveValue(today);
	await expect(habitRow(page, "Walk")).toHaveCount(1);

	await habitName(page).fill("Tomorrow's run");
	await habitStart(page).fill(tomorrow);
	await page.getByRole("button", { name: "Create habit", exact: true }).click();
	await expect(page.locator("#habit-feedback")).toContainText("after today");
	await expect(habitName(page)).toHaveValue("Tomorrow's run");
	await expect(habitStart(page)).toHaveValue(tomorrow);
	await expect(habitRow(page, "Tomorrow's run")).toHaveCount(0);
});

test("an open form accepts today's date after browser-local midnight", async ({ page }) => {
 const now = new Date();
 const current = await localCalendar(page);
 await page.clock.setFixedTime(new Date(now.getTime() - 24 * 60 * 60 * 1000));
 await page.reload({ waitUntil: "load" });
 await habitsTab(page).click();
 await expect(habitStart(page)).not.toHaveValue(current.today);
 await page.clock.setFixedTime(now);
 await createHabit(page, "Start today", current.today);
 await expect(habitRow(page, "Start today")).toBeVisible();
});

test("habit names accept 80 Unicode characters and reject longer names without losing the draft", async ({ page }) => {
 await habitsTab(page).click();
 const name = "📖".repeat(80);
 await createHabit(page, name);
 await habitName(page).fill("x".repeat(81));
 await page.getByRole("button", { name: "Create habit", exact: true }).click();
 await expect(page.locator("#habit-feedback")).toContainText("80");
 await expect(habitName(page)).toHaveValue("x".repeat(81));
 await expect(page.locator("#habit-matrix tbody tr")).toHaveCount(1);
});

test("mobile Plan / Notes & Habits navigation keeps names visible while dates scroll", async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await expect(page.locator("#block-list")).toBeVisible();
	await expect(page.locator(".cm-editor")).toBeHidden();
	await mobilePanel(page, "Notes & Habits");
	await expect(page.locator("#block-list")).toBeHidden();
	await expect(jotTab(page)).toHaveAttribute("aria-selected", "true");
	await expect(page.locator(".cm-editor")).toBeVisible();
	await habitsTab(page).click();
	await createHabit(page, "A daily walk");
	const { today } = await localCalendar(page);
	await checkIn(page, "A daily walk", today).click();
	await expect(checkIn(page, "A daily walk", today)).toHaveAttribute("aria-pressed", "true");
	const matrix = page.locator("#habit-matrix");
	const name = habitRow(page, "A daily walk").getByRole("rowheader");
	const before = await name.boundingBox();
	await matrix.evaluate((element) => { element.scrollLeft = element.scrollWidth; });
	await expect.poll(() => matrix.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
	const after = await name.boundingBox();
	expect(Math.abs(after.x - before.x)).toBeLessThan(2);
	await expect(name).toBeInViewport();
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
	const calendar = await localCalendar(page);
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.previous.month);
	await page.getByRole("button", { name: "This month", exact: true }).click();
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);

	await mobilePanel(page, "Plan");
	await expect(page.locator("#block-list")).toBeVisible();
	await expect(matrix).toBeHidden();
	await mobilePanel(page, "Notes & Habits");
	await expect(habitsTab(page)).toHaveAttribute("aria-selected", "true");
	await expect(habitRow(page, "A daily walk")).toBeVisible();
 await page.screenshot({ path: test.info().outputPath("habits-mobile.png"), fullPage: true });
});

test("habits created in another tab arrive live without replacing an unfinished form", async ({ page, context }) => {
	await habitsTab(page).click();
	await habitName(page).fill("Unfinished draft");
	await habitStart(page).fill("2020-01-02");
	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });
	await habitsTab(other).click();
	await createHabit(other, "Stretch");
	await expect(habitRow(page, "Stretch")).toBeVisible();
	await expect(habitName(page)).toHaveValue("Unfinished draft");
	await expect(habitStart(page)).toHaveValue("2020-01-02");
	await expect(habitsTab(page)).toHaveAttribute("aria-selected", "true");
	await other.close();
});

test("Jotpad text, caret, scroll and pending save survive switching tabs and a habit SSE patch", async ({ page }) => {
	const content = page.locator(".cm-content");
	const text = Array.from({ length: 100 }, (_, i) => `Line ${i + 1}: keep this note`).join("\n");
	let releaseSave;
	const saveGate = new Promise((resolve) => { releaseSave = resolve; });
	await page.route("**/jot", async (route) => {
		await saveGate;
		await route.continue();
	});
	try {
		await content.click();
		const pending = page.waitForRequest((request) => new URL(request.url()).pathname === "/jot" && request.method() === "POST");
		await page.keyboard.insertText(text);
		await page.keyboard.press("ArrowLeft");
		await pending;
		await expect(page.locator("#jot-status")).not.toHaveAttribute("data-state", "saved");
		await expect.poll(() => page.locator(".cm-scroller").evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
		const state = await page.evaluateHandle(() => {
			const selection = getSelection();
			return {
				editor: document.querySelector(".cm-editor"),
				anchorText: selection.anchorNode.textContent,
				offset: selection.anchorOffset,
				scroll: document.querySelector(".cm-scroller").scrollTop,
			};
		});
		await habitsTab(page).click();
		await createHabit(page, "Keep notes safe");
		const { today } = await localCalendar(page);
		await checkIn(page, "Keep notes safe", today).click();
		await expect(checkIn(page, "Keep notes safe", today)).toHaveAttribute("aria-pressed", "true");
		await page.getByRole("button", { name: /Previous month/ }).click();
		await page.getByRole("button", { name: "This month", exact: true }).click();
		await jotTab(page).click();
		await content.focus();
		expect(await state.evaluate((saved) => ({
			sameEditor: saved.editor === document.querySelector(".cm-editor"),
			sameCaret: saved.anchorText === getSelection().anchorNode.textContent && saved.offset === getSelection().anchorOffset,
			scrollDifference: Math.abs(saved.scroll - document.querySelector(".cm-scroller").scrollTop),
		}))).toEqual({ sameEditor: true, sameCaret: true, scrollDifference: 0 });
		await state.dispose();
		const saved = page.waitForResponse((response) => new URL(response.url()).pathname === "/jot" && response.request().method() === "POST");
		releaseSave();
		expect((await saved).ok()).toBe(true);
		await expect(page.locator("#jot-status")).toHaveAttribute("data-state", "saved");
		// Copy reads the whole document even when CodeMirror virtualizes offscreen lines.
		await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
		const expectWholeNote = async () => {
			await content.focus();
			await page.evaluate(() => navigator.clipboard.writeText(""));
			await page.keyboard.press("ControlOrMeta+a");
			await page.keyboard.press("ControlOrMeta+c");
			await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(text);
		};
		await expectWholeNote();
		await page.reload({ waitUntil: "load" });
		await expectWholeNote();
		await habitsTab(page).click();
		await expect(habitRow(page, "Keep notes safe")).toBeVisible();
	} finally {
		releaseSave();
		await page.unroute("**/jot");
	}
});

test("adding and clearing plan blocks leaves habits and Jotpad unchanged after reload", async ({ page }) => {
	await page.locator(".cm-content").click();
	await page.keyboard.insertText("Keep my perpetual notes");
	await expect(page.locator("#jot-status")).toHaveAttribute("data-state", "saved");
	await habitsTab(page).click();
	await createHabit(page, "Read every day");
	await page.getByRole("button", { name: /^Add block at/ }).first().click();
	await page.locator("#create-modal").getByLabel("Name").fill("Focused reading");
	await page.locator("#create-submit").click();
	await expect(page.locator(".block-item")).toHaveCount(1);
	await expect(page.locator(".block-item")).toContainText("Focused reading");
	await page.locator('[commandfor="clear-modal"][command="show-modal"]').click();
	await page.locator("#clear-modal").getByRole("button", { name: "Clear", exact: true }).click();
	await expect(page.locator(".block-item")).toHaveCount(0);
	await expect(habitRow(page, "Read every day")).toBeVisible();
	await jotTab(page).click();
	await expect(page.locator(".cm-content")).toHaveText("Keep my perpetual notes");
	await page.reload({ waitUntil: "load" });
	await expect(page.locator(".block-item")).toHaveCount(0);
	await expect(page.locator(".cm-content")).toHaveText("Keep my perpetual notes");
	await habitsTab(page).click();
	await expect(habitRow(page, "Read every day")).toBeVisible();
});
