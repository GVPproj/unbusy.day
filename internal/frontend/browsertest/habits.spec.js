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
const deleteHabit = (page, name) => habitRow(page, name).getByRole("button", { name: `Delete ${name}`, exact: true });
const deleteDialog = (page) => page.getByRole("dialog", { name: "Delete habit", exact: true });
const confirmDelete = (page) => deleteDialog(page).getByRole("button", { name: "Delete permanently", exact: true });
const emptyHabits = "No habits yet in this month. Create a habit to begin.";

async function localCalendar(page) {
	return page.evaluate(() => {
		const now = new Date();
		const monthLabel = (d) => `${d.toLocaleDateString("en-US", { month: "short" }).replace(/^Sep$/, "Sept")} '${String(d.getFullYear()).slice(-2)}`;
		const date = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
		return {
			today: date(now),
			tomorrow: date(new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)),
			day: now.getDate(),
			days: new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate(),
			month: monthLabel(now),
			beforePrevious: monthLabel(new Date(now.getFullYear(), now.getMonth() - 2, 1)),
			previous: (() => {
				const first = new Date(now.getFullYear(), now.getMonth() - 1, 1);
				const last = new Date(now.getFullYear(), now.getMonth(), 0);
				return {
					first: date(first),
					last: date(last),
					days: last.getDate(),
					month: monthLabel(first),
				};
			})(),
		};
	});
}

async function openCreateHabit(page) {
	await page.locator('[commandfor="habit-create-dialog"][command="show-modal"]').click();
	await expect(page.getByRole("dialog", { name: "Create habit", exact: true })).toBeVisible();
}

async function createHabit(page, name, start) {
	await openCreateHabit(page);
	await habitName(page).fill(name);
	if (start) await habitStart(page).fill(start);
	await page.locator("#habit-create").getByRole("button", { name: /create|add/i }).click();
	await expect(page.locator("#habit-create-dialog")).toBeHidden();
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
	await expect(page.locator(".companion-head #companion-status")).toBeVisible();
	await expect(page.locator("#jot-panel h2, #jot-panel output")).toHaveCount(0);
	const header = await page.locator(".companion-head").boundingBox();
	const jotEditor = await page.locator(".cm-editor").boundingBox();
	expect(jotEditor.y - (header.y + header.height)).toBeLessThan(20);
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

for (const mobile of [false, true]) {
	test.describe(mobile ? "mobile habit names" : "desktop habit names", () => {
		test.use({ hasTouch: mobile });

		test("clamp long names and keep themed actions accessible", async ({ page }) => {
			if (mobile) {
				await page.setViewportSize({ width: 390, height: 900 });
				await mobilePanel(page, "Notes & Habits");
			}
			await habitsTab(page).click();
			const names = ["Eat", "Do large amounts of work every day with plenty of breaks", "Unbroken".repeat(10)];
			for (const name of names) await createHabit(page, name);

			for (const [feeling, icon] of [["cozy", "solar"], ["pixel", "pixel"], ["mono", "mono"]]) {
				const theme = page.locator("#theme-modal");
				await theme.evaluate((el) => el.showModal());
				await theme.getByRole("button", { name: new RegExp(`^${feeling}$`, "i") }).click();
				if (feeling === "pixel") {
					await theme.getByRole("button", { name: "Nord", exact: true }).click();
					await theme.getByRole("button", { name: "Dark", exact: true }).click();
				}
				await theme.getByRole("button", { name: "Done", exact: true }).click();
				await page.evaluate(() => document.fonts.ready);
				for (const name of names) {
					const row = habitRow(page, name);
					const label = row.locator(".habit-name");
					await expect(label).toHaveAttribute("title", name);
					const size = await label.evaluate((el) => ({
						height: el.getBoundingClientRect().height,
						line: parseFloat(getComputedStyle(el).lineHeight),
						scroll: el.scrollHeight,
					}));
					expect(size.height).toBeLessThanOrEqual(size.line * 2 + 1);
					if (name !== "Eat") {
						expect(size.height).toBeCloseTo(size.line * 2, 0);
						expect(size.scroll).toBeGreaterThan(size.height);
					}
					const header = await row.getByRole("rowheader").boundingBox();
					expect(header.width).toBeGreaterThanOrEqual(192);
					for (const action of ["Edit", "Delete"]) {
						const button = row.getByRole("button", { name: `${action} ${name}`, exact: true });
						await expect(button).toHaveText("");
						await expect(button.locator("svg:visible")).toHaveCount(1);
						await expect(button.locator(`.icon-${icon}`)).toBeVisible();
						const box = await button.boundingBox();
						expect(box.width).toBeGreaterThanOrEqual(mobile ? 44 : 32);
						expect(box.height).toBeGreaterThanOrEqual(mobile ? 44 : 32);
						expect(box.x + box.width).toBeLessThanOrEqual(header.x + header.width);
					}
				}
				await page.locator("#habit-matrix").screenshot({ path: test.info().outputPath(`habit-names-${feeling}.png`) });
			}

			const row = habitRow(page, names[1]);
			await row.getByRole("button", { name: `Edit ${names[1]}`, exact: true }).click();
			await expect(page.locator("#habit-edit-name")).toHaveValue(names[1]);
			await page.locator("#habit-edit").getByRole("button", { name: "Cancel", exact: true }).click();
			await deleteHabit(page, names[1]).click();
			await expect(deleteDialog(page)).toContainText(names[1]);
			await deleteDialog(page).getByRole("button", { name: "Cancel", exact: true }).click();
			await expect(row).toBeVisible();
		});
	});
}

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
	for (let steps = 0; await heading.textContent() !== "Jan '25" && steps < 60; steps++) {
		await previousMonth(page);
	}
	await expect(heading).toHaveText("Jan '25");
	await expect(habitRow(page, "Long history").locator("td")).toHaveCount(31);
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(heading).toHaveText("Dec '24");
	await expect(habitRow(page, "Long history").locator("td")).toHaveCount(31);
	for (let steps = 0; await heading.textContent() !== "Feb '24" && steps < 12; steps++) {
		await previousMonth(page);
	}
	await expect(heading).toHaveText("Feb '24");
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
	await expect(page.locator("#companion-status")).toHaveText("Saving…");
	await expect(page.locator("#habit-checkin-feedback")).toBeEmpty();
	release();
	await expect(button).toHaveAttribute("aria-pressed", "true");
	await expect(page.locator("#companion-status")).toHaveText("Saved");
	await expect(page.locator("#habit-checkin-feedback")).toBeEmpty();
	await page.unroute("**/habits/check-in");

	await page.reload({ waitUntil: "load" });
	await habitsTab(page).click();
	button = checkIn(page, "Journal", today);
	await expect(button).toHaveAttribute("aria-pressed", "true");

	await page.route("**/habits/check-in", (route) => route.fulfill({ status: 500, body: "failed" }));
	await button.click();
	await expect(button).toHaveAttribute("aria-pressed", "true");
	await expect(button).toHaveAttribute("data-save-state", "failed");
	await expect(page.locator("#habit-checkin-feedback")).toContainText(/save not confirmed.*retry the original change/i);
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
	const matrix = page.locator("#habit-scroll");
	const button = checkIn(page, "Stretch", today);
	await button.focus();
	await matrix.evaluate((element) => { element.scrollLeft = element.scrollWidth; });
	const scroll = await matrix.evaluate((element) => element.scrollLeft);
	expect(scroll).toBeGreaterThan(0);

	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });
	await habitsTab(other).click();
	await checkIn(other, "Stretch", today).click();
	await expect(button).toHaveAttribute("aria-pressed", "true");
	await expect(button).toBeFocused();
	expect(Math.abs(await matrix.evaluate((element) => element.scrollLeft) - scroll)).toBeLessThan(2);

	// Activate without Playwright introducing a new scroll-to-click position.
	await page.keyboard.press("Enter");
	await expect(button).toHaveAttribute("aria-pressed", "false");
	await expect(checkIn(other, "Stretch", today)).toHaveAttribute("aria-pressed", "false");
	await expect(button).toBeFocused();
	expect(Math.abs(await matrix.evaluate((element) => element.scrollLeft) - scroll)).toBeLessThan(2);
	await other.close();
});

test("habit edits preserve history, drafts, and live state across views", async ({ page, context }) => {
	await habitsTab(page).click();
	const calendar = await localCalendar(page);
	await createHabit(page, "Editable", calendar.previous.first);
	await page.getByRole("button", { name: /Previous month/ }).click();
	await checkIn(page, "Editable", calendar.previous.last).click();
	await page.getByRole("button", { name: "This month", exact: true }).click();

	await page.getByRole("button", { name: "Edit Editable", exact: true }).focus();
	await page.keyboard.press("Enter");
	const dialog = page.getByRole("dialog", { name: "Edit habit" });
	await expect(dialog).toBeVisible();
	await expect(dialog.getByLabel("Name", { exact: true })).toHaveValue("Editable");
	await expect(dialog.getByLabel("Start date", { exact: true })).toHaveValue(calendar.previous.first);
	await dialog.getByLabel("Name", { exact: true }).fill("Unfinished local edit");

	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });
	await habitsTab(other).click();
	await other.getByRole("button", { name: "Edit Editable", exact: true }).click();
	const otherDialog = other.getByRole("dialog", { name: "Edit habit" });
	await otherDialog.getByLabel("Name", { exact: true }).fill("Renamed elsewhere");
	await otherDialog.getByRole("button", { name: "Save", exact: true }).click();
	await expect(otherDialog).toBeHidden();
	await expect(habitRow(page, "Renamed elsewhere")).toBeVisible();
	await expect(dialog).toBeVisible();
	await expect(dialog.getByLabel("Name", { exact: true })).toHaveValue("Unfinished local edit");

	await dialog.getByLabel("Name", { exact: true }).fill("Edited everywhere");
	await dialog.getByLabel("Start date", { exact: true }).fill(calendar.today);
	await dialog.getByRole("button", { name: "Save", exact: true }).click();
	await expect(page.locator("#habit-edit-feedback")).toContainText("existing check-in");
	await expect(dialog.getByLabel("Name", { exact: true })).toHaveValue("Edited everywhere");
	await expect(dialog.getByLabel("Start date", { exact: true })).toHaveValue(calendar.today);

	await dialog.getByLabel("Start date", { exact: true }).fill(calendar.previous.last);
	await dialog.getByLabel("Start date", { exact: true }).press("Enter");
	await expect(dialog).toBeHidden();
	await expect(habitRow(page, "Edited everywhere")).toBeVisible();
	await page.getByRole("button", { name: /Previous month/ }).click();
	await expect(checkIn(page, "Edited everywhere", calendar.previous.last)).toHaveAttribute("aria-pressed", "true");
	await expect(habitRow(other, "Edited everywhere")).toBeVisible();
	await other.close();
});

test("a delayed edit response cannot close or overwrite a newer draft", async ({ page }) => {
	await habitsTab(page).click();
	const { today } = await localCalendar(page);
	await createHabit(page, "Delayed edit", today);
	const edit = page.getByRole("button", { name: "Edit Delayed edit", exact: true });
	await edit.click();
	const dialog = page.getByRole("dialog", { name: "Edit habit" });
	await dialog.getByLabel("Name", { exact: true }).fill("First submission");

	let release;
	const gate = new Promise((resolve) => { release = resolve; });
	await page.route("**/habits/edit", async (route) => {
		await gate;
		await route.continue();
	});
	const request = page.waitForRequest((req) => new URL(req.url()).pathname === "/habits/edit");
	await dialog.getByRole("button", { name: "Save", exact: true }).click();
	await request;
	await page.keyboard.press("Escape");
	await expect(dialog).toBeHidden();
	await edit.click();
	await dialog.getByLabel("Name", { exact: true }).fill("Newer draft");
	const response = page.waitForResponse((res) => new URL(res.url()).pathname === "/habits/edit");
	release();
	expect((await response).ok()).toBe(true);
	await expect(dialog).toBeVisible();
	await expect(dialog.getByLabel("Name", { exact: true })).toHaveValue("Newer draft");
	await page.unroute("**/habits/edit");
});

test("habit editing works in the mobile companion panel", async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await mobilePanel(page, "Notes & Habits");
	await habitsTab(page).click();
	const { today } = await localCalendar(page);
	await createHabit(page, "Mobile habit", today);
	await page.getByRole("button", { name: "Edit Mobile habit", exact: true }).click();
	const dialog = page.getByRole("dialog", { name: "Edit habit" });
	await expect(dialog).toBeVisible();
	await dialog.getByLabel("Name", { exact: true }).fill("Mobile edited");
	await dialog.getByRole("button", { name: "Save", exact: true }).click();
	await expect(dialog).toBeHidden();
	await expect(habitRow(page, "Mobile edited")).toBeVisible();
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("deletion requires native confirmation; Cancel and Escape restore keyboard focus without writing", async ({ page }) => {
	await habitsTab(page).click();
	const { today } = await localCalendar(page);
	await createHabit(page, "Keep me");
	await checkIn(page, "Keep me", today).click();
	await expect(checkIn(page, "Keep me", today)).toHaveAttribute("aria-pressed", "true");
	const requests = [];
	page.on("request", (request) => {
		if (new URL(request.url()).pathname === "/habits/delete") requests.push(request);
	});
	const invoker = deleteHabit(page, "Keep me");
	const dialog = deleteDialog(page);
	for (const dismiss of ["Enter", "Escape"]) {
		await invoker.focus();
		await page.keyboard.press("Enter");
		await expect(dialog).toBeVisible();
		expect(await dialog.evaluate((element) => element instanceof HTMLDialogElement && element.matches(":modal"))).toBe(true);
		await expect(dialog).toContainText("Keep me");
		await expect(dialog).toContainText("All check-in history will be permanently deleted. This cannot be undone.");
		const cancel = dialog.getByRole("button", { name: "Cancel", exact: true });
		await expect(cancel).toHaveAttribute("autofocus", "");
		await expect(cancel).toBeFocused();
		await expect(confirmDelete(page)).toBeVisible();
		await page.keyboard.press(dismiss);
		await expect(dialog).toBeHidden();
		await expect(invoker).toBeFocused();
	}
	await page.reload({ waitUntil: "load" });
	await habitsTab(page).click();
	await expect(checkIn(page, "Keep me", today)).toHaveAttribute("aria-pressed", "true");
	expect(requests).toHaveLength(0);
});

test("deletion clears past and current months across views, restores remote focus, and never reuses history", async ({ page, context }) => {
	await habitsTab(page).click();
	const calendar = await localCalendar(page);
	await createHabit(page, "Fresh start", calendar.previous.first);
	const oldID = await habitRow(page, "Fresh start").getAttribute("id");
	expect(oldID).toMatch(/^habit-\d+$/);
	await checkIn(page, "Fresh start", calendar.today).click();
	await expect(checkIn(page, "Fresh start", calendar.today)).toHaveAttribute("aria-pressed", "true");
	await previousMonth(page);
	await checkIn(page, "Fresh start", calendar.previous.last).click();
	await expect(checkIn(page, "Fresh start", calendar.previous.last)).toHaveAttribute("aria-pressed", "true");
	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });
	await habitsTab(other).click();
	await expect(checkIn(other, "Fresh start", calendar.today)).toHaveAttribute("aria-pressed", "true");
	await checkIn(other, "Fresh start", calendar.today).focus();
	await expect(checkIn(other, "Fresh start", calendar.today)).toBeFocused();
	await deleteHabit(page, "Fresh start").click();
	const request = page.waitForRequest((req) => new URL(req.url()).pathname === "/habits/delete" && req.method() === "POST");
	await confirmDelete(page).click();
	const signals = (await request).postDataJSON();
	expect(String(signals.habitdeleteid)).toBe(oldID.slice("habit-".length));
	expect(signals.habitdeleteview).toBeTruthy();
	await expect(deleteDialog(page)).toBeHidden();
	for (const view of [page, other]) {
		await expect(habitRow(view, "Fresh start")).toHaveCount(0);
		await expect(view.locator("#habit-matrix")).toContainText(emptyHabits);
		await expect(view.locator("#habit-matrix")).toBeFocused();
	}
	await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.previous.month);
	await expect(other.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
	for (const view of [page, other]) {
		await view.reload({ waitUntil: "load" });
		await habitsTab(view).click();
		await expect(view.locator("#habit-matrix")).toContainText(emptyHabits);
		await previousMonth(view);
		await expect(view.locator("#habit-matrix")).toContainText(emptyHabits);
	}
	await page.getByRole("button", { name: "This month", exact: true }).click();
	await createHabit(page, "Fresh start", calendar.previous.first);
	const newID = await habitRow(page, "Fresh start").getAttribute("id");
	expect(newID).toMatch(/^habit-\d+$/);
	expect(newID).not.toBe(oldID);
	await expect(checkIn(page, "Fresh start", calendar.today)).toHaveAttribute("aria-pressed", "false");
	await expect(checkIn(other, "Fresh start", calendar.previous.last)).toHaveAttribute("aria-pressed", "false");
	await previousMonth(page);
	await expect(checkIn(page, "Fresh start", calendar.previous.last)).toHaveAttribute("aria-pressed", "false");
	await expect(habitRow(page, "Fresh start").locator('[aria-pressed="true"]')).toHaveCount(0);
	await page.reload({ waitUntil: "load" });
	await habitsTab(page).click();
	await expect(habitRow(page, "Fresh start")).toHaveAttribute("id", newID);
	await expect(habitRow(page, "Fresh start").locator('[aria-pressed="true"]')).toHaveCount(0);
	await other.close();
});

test("closing confirmation after remote deletion restores focus when its opener is gone", async ({ page, context }) => {
	await habitsTab(page).click();
	const other = await context.newPage();
	await other.goto(baseURL, { waitUntil: "load" });
	await habitsTab(other).click();
	for (const dismiss of ["Enter", "Escape"]) {
		const name = `Gone remotely ${dismiss}`;
		await createHabit(page, name);
		await deleteHabit(page, name).click();
		await expect(deleteDialog(page).getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
		await deleteHabit(other, name).click();
		await confirmDelete(other).click();
		await expect(habitRow(page, name)).toHaveCount(0);
		await expect(deleteDialog(page)).toBeVisible();
		await page.keyboard.press(dismiss);
		await expect(deleteDialog(page)).toBeHidden();
		await expect(page.locator("#habit-matrix")).toBeFocused();
	}
	await other.close();
});

test("a failed deletion keeps confirmation open with retry feedback", async ({ page }) => {
	await habitsTab(page).click();
	await createHabit(page, "Retry deletion");
	await page.route("**/habits/delete", (route) => route.fulfill({ status: 500, body: "failed" }));
	await deleteHabit(page, "Retry deletion").click();
	await confirmDelete(page).click();
	await expect(deleteDialog(page)).toBeVisible();
	await expect(deleteDialog(page)).toContainText(/retry|try again/i);
	await expect(habitRow(page, "Retry deletion")).toBeVisible();
	await expect(confirmDelete(page)).toBeEnabled();
	await page.unroute("**/habits/delete");
	await confirmDelete(page).click();
	await expect(deleteDialog(page)).toBeHidden();
	await expect(habitRow(page, "Retry deletion")).toHaveCount(0);
	await expect(page.locator("#habit-matrix")).toBeFocused();
});

test("a delayed deletion ack cannot close a newer confirmation", async ({ page }) => {
	await habitsTab(page).click();
	await createHabit(page, "First deletion");
	await createHabit(page, "New confirmation");
	let release;
	const gate = new Promise((resolve) => { release = resolve; });
	let firstSignals;
	await page.route("**/habits/delete", async (route) => {
		firstSignals = route.request().postDataJSON();
		const response = await route.fetch();
		await gate;
		await route.fulfill({ response });
	});
	try {
		await deleteHabit(page, "First deletion").click();
		await confirmDelete(page).click();
		// The commit arrives via SSE while its initiating view's ack is held back.
		await expect(habitRow(page, "First deletion")).toHaveCount(0);
		await page.keyboard.press("Escape");
		await expect(deleteDialog(page)).toBeHidden();
		await deleteHabit(page, "New confirmation").click();
		const response = page.waitForResponse((res) => new URL(res.url()).pathname === "/habits/delete");
		release();
		expect((await response).ok()).toBe(true);
		await (await response).finished();
		await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
		await expect(deleteDialog(page)).toBeVisible();
		await expect(deleteDialog(page)).toContainText("New confirmation");
		await expect(habitRow(page, "New confirmation")).toBeVisible();
		await page.unroute("**/habits/delete");
		const next = page.waitForRequest((req) => new URL(req.url()).pathname === "/habits/delete" && req.method() === "POST");
		await confirmDelete(page).click();
		const nextSignals = (await next).postDataJSON();
		expect(nextSignals.habitdeleteview).not.toBe(firstSignals.habitdeleteview);
		expect(nextSignals.habitdeleteid).not.toBe(firstSignals.habitdeleteid);
		await expect(deleteDialog(page)).toBeHidden();
		await expect(habitRow(page, "New confirmation")).toHaveCount(0);
	} finally {
		release();
		await page.unroute("**/habits/delete");
	}
});

test("habit deletion works in the mobile companion panel", async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await mobilePanel(page, "Notes & Habits");
	await habitsTab(page).click();
	await createHabit(page, "Mobile deletion");
	await deleteHabit(page, "Mobile deletion").click();
	await expect(deleteDialog(page)).toBeInViewport();
	await expect(confirmDelete(page)).toBeInViewport();
	await page.screenshot({ path: test.info().outputPath("habit-delete-mobile.png"), fullPage: true, animations: "disabled" });
	await deleteDialog(page).getByRole("button", { name: "Cancel", exact: true }).click();
	await expect(deleteHabit(page, "Mobile deletion")).toBeFocused();
	await deleteHabit(page, "Mobile deletion").click();
	await confirmDelete(page).click();
	await expect(deleteDialog(page)).toBeHidden();
	await expect(page.locator("#habit-matrix")).toContainText(emptyHabits);
	await expect(page.locator("#habit-matrix")).toBeFocused();
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("habit creation fields live in a dismissible modal and keep a dismissed draft", async ({ page }) => {
	await habitsTab(page).click();
	await expect(habitName(page)).toBeHidden();
	await expect(habitStart(page)).toBeHidden();
	await openCreateHabit(page);
	await expect(habitName(page)).toBeFocused();
	await habitName(page).fill("Draft habit");
	await page.locator("#habit-create-dialog").getByRole("button", { name: "Cancel", exact: true }).click();
	await expect(habitName(page)).toBeHidden();
	await expect(habitRow(page, "Draft habit")).toHaveCount(0);
	await openCreateHabit(page);
	await expect(habitName(page)).toBeFocused();
	await expect(habitName(page)).toHaveValue("Draft habit");
	await page.keyboard.press("Escape");
	await expect(page.locator("#habit-create-dialog")).toBeHidden();
});

test("saving a habit clears the create form back to its defaults", async ({ page }) => {
	await habitsTab(page).click();
	const { today, previous } = await localCalendar(page);
	await createHabit(page, "Reset after save", previous.first);
	await openCreateHabit(page);
	await expect(habitName(page)).toHaveValue("");
	await expect(habitStart(page)).toHaveValue(today);
	await page.keyboard.press("Escape");
	await expect(page.locator("#habit-create-dialog")).toBeHidden();
});

test("duplicate and future-start errors retain the submitted name and date", async ({ page }) => {
	await habitsTab(page).click();
	const { today, tomorrow } = await localCalendar(page);
	await createHabit(page, "Walk", today);
	await openCreateHabit(page);
	await habitName(page).fill("Walk");
	await habitStart(page).fill(today);
	await page.locator("#habit-create").getByRole("button", { name: "Create habit", exact: true }).click();
	await expect(page.locator("#habit-feedback")).toContainText(/already|duplicate/i);
	await expect(habitName(page)).toHaveValue("Walk");
	await expect(habitStart(page)).toHaveValue(today);
	await expect(habitRow(page, "Walk")).toHaveCount(1);

	await habitName(page).fill("Tomorrow's run");
	await habitStart(page).fill(tomorrow);
	await page.locator("#habit-create").getByRole("button", { name: "Create habit", exact: true }).click();
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
 await openCreateHabit(page);
 await habitName(page).fill("x".repeat(81));
 await page.locator("#habit-create").getByRole("button", { name: "Create habit", exact: true }).click();
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
	const matrix = page.locator("#habit-scroll");
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

for (const width of [1440, 390]) {
	test(`month controls stay fixed while dates scroll at width ${width}`, async ({ page }) => {
		await page.setViewportSize({ width, height: 900 });
		if (width < 832) await mobilePanel(page, "Notes & Habits");
		await habitsTab(page).click();
		await createHabit(page, "Scroll test");
		const controls = page.locator(".month-nav button, .month-nav h2");
		const positions = () => controls.evaluateAll((elements) => elements.map((el) => el.getBoundingClientRect().x));
		const before = await positions();
		const scroll = await page.locator("#habit-grid table").evaluate((table) => {
			let scroller = table.parentElement;
			while (scroller && !["auto", "scroll"].includes(getComputedStyle(scroller).overflowX)) scroller = scroller.parentElement;
			scroller.scrollLeft = scroller.scrollWidth;
			return scroller.scrollLeft;
		});
		expect(scroll).toBeGreaterThan(0);
		await expect.poll(positions).toEqual(before);
		for (const control of await controls.all()) await expect(control).toBeInViewport({ ratio: 1 });
		for (const [feeling, icon] of [["cozy", "solar"], ["pixel", "pixel"], ["mono", "mono"]]) {
			await page.locator("#theme-modal").getByRole("button", { name: new RegExp(`^${feeling}$`, "i"), includeHidden: true }).evaluate((button) => button.click());
			await expect(page.locator("html")).toHaveAttribute("data-feeling", feeling);
			for (const id of ["habit-previous-month", "habit-next-month"]) {
				const button = page.locator(`#${id}`);
				await expect(button).toHaveText("");
				await expect(button.locator("svg:visible")).toHaveCount(1);
				await expect(button.locator(`.icon-${icon}`)).toBeVisible();
				const arrow = await button.boundingBox();
				const thisMonth = await page.locator("#habit-this-month").boundingBox();
				expect(Math.abs(arrow.width - arrow.height)).toBeLessThan(1);
				expect(Math.abs(arrow.height - thisMonth.height)).toBeLessThan(1);
			}
			for (const control of await controls.all()) await expect(control).toBeInViewport({ ratio: 1 });
		}
	});
}

test("habits created in another tab arrive live without replacing an unfinished form", async ({ page, context }) => {
	await habitsTab(page).click();
	await openCreateHabit(page);
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

test("Jotpad text, caret, scroll and pending save survive habit deletion while the plan stays unchanged", async ({ page }) => {
	await page.getByRole("button", { name: /^Add block at/ }).first().click();
	await page.locator("#create-modal").getByLabel("Name").fill("Keep this plan");
	await page.locator("#create-submit").click();
	await expect(page.locator(".block-item")).toHaveCount(1);
	const planState = () => page.locator(".block-item").evaluateAll((blocks) => blocks.map((block) => ({
		id: block.dataset.id, slot: block.dataset.slot, span: block.dataset.span, text: block.textContent,
	})));
	const plan = await planState();
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
		await expect(page.locator("#companion-status")).not.toHaveAttribute("data-state", "saved");
		await expect.poll(() => page.locator(".cm-scroller").evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
		const state = await page.evaluateHandle(async () => {
			const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
			const view = EditorView.findFromDOM(document.querySelector(".cm-editor"));
			const selection = getSelection();
			return {
				view, originalSelection: view.state.selection.toJSON(),
				editor: document.querySelector(".cm-editor"),
				anchorText: selection.anchorNode.textContent,
				offset: selection.anchorOffset,
				scroll: document.querySelector(".cm-scroller").scrollTop,
			};
		});
		await habitsTab(page).click();
		await createHabit(page, "Keep notes safe");
		const calendar = await localCalendar(page);
		await checkIn(page, "Keep notes safe", calendar.today).click();
		await expect(checkIn(page, "Keep notes safe", calendar.today)).toHaveAttribute("aria-pressed", "true");
		await page.getByRole("button", { name: /Previous month/ }).click();
		await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.previous.month);
		await page.getByRole("button", { name: "This month", exact: true }).click();
		await expect(page.locator("#habit-grid .month-nav h2")).toHaveText(calendar.month);
		await deleteHabit(page, "Keep notes safe").click();
		await confirmDelete(page).click();
		await expect(deleteDialog(page)).toBeHidden();
		await expect(habitRow(page, "Keep notes safe")).toHaveCount(0);
		expect(await planState()).toEqual(plan);
		await expect(page.locator("#companion-status")).toBeVisible();
		await expect(page.locator("#companion-status")).not.toHaveAttribute("data-state", "saved");
		expect(await state.evaluate((saved) => saved.view.state.selection.toJSON())).toEqual(await state.evaluate((saved) => saved.originalSelection));
		await jotTab(page).click();
		await page.keyboard.press("Tab");
		await expect(page.locator("#jot-panel")).toBeFocused();
		await page.keyboard.press("Tab");
		await expect(content).toBeFocused();
		// CodeMirror restores its virtualized viewport on the next measurement frame.
		await expect.poll(() => state.evaluate((saved) => ({
			sameEditor: saved.editor === document.querySelector(".cm-editor"),
			sameCaret: saved.anchorText === getSelection().anchorNode.textContent && saved.offset === getSelection().anchorOffset,
			scrollDifference: Math.abs(saved.scroll - document.querySelector(".cm-scroller").scrollTop),
		}))).toEqual({ sameEditor: true, sameCaret: true, scrollDifference: 0 });
		await state.dispose();
		const saved = page.waitForResponse((response) => new URL(response.url()).pathname === "/jot" && response.request().method() === "POST");
		releaseSave();
		expect((await saved).ok()).toBe(true);
		await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
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
		await expect(habitRow(page, "Keep notes safe")).toHaveCount(0);
		await expect(page.locator("#habit-matrix")).toContainText(emptyHabits);
		expect(await planState()).toEqual(plan);
	} finally {
		releaseSave();
		await page.unrouteAll({ behavior: "wait" });
	}
});

test("adding and clearing plan blocks leaves habits and Jotpad unchanged after reload", async ({ page }) => {
	await page.locator(".cm-content").click();
	await page.keyboard.insertText("Keep my perpetual notes");
	await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
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
