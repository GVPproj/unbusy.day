import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

const notes = ["Daily", "Weekly", "Quarterly"];
const tab = (page, name) => page.getByRole("tab", { name, exact: true });
const status = (page) => page.locator("#companion-status");

// Resolve the accessible relationship rather than depending on mount IDs.
async function panel(page, name) {
	const id = await tab(page, name).getAttribute("aria-controls");
	expect(id).toBeTruthy();
	return page.locator(`[id=${JSON.stringify(id)}]`);
}

async function select(page, name) {
	await tab(page, name).click();
	await expect(tab(page, name)).toHaveAttribute("aria-selected", "true");
	const selected = await panel(page, name);
	await expect(selected).toBeVisible();
	await expect(selected.locator(".cm-content")).toHaveCount(1);
	return selected.locator(".cm-content");
}

async function openNotes(page) {
	if (page.viewportSize().width >= 832) return;
	await page.locator(".menu-toggle").click();
	await page.locator("#sidenav").getByRole("button", { name: "Extras", exact: true }).click();
	await expect(page.locator(".menu-toggle")).toHaveAttribute("aria-expanded", "false");
}

function isSave(response, name) {
	const url = new URL(response.url());
	return response.request().method() === "POST" && url.pathname === "/jot"
		&& url.searchParams.get("note") === name.toLowerCase();
}

async function appendAndSave(page, name, text) {
	const editor = await select(page, name);
	await editor.click();
	await page.keyboard.press("ControlOrMeta+End");
	const saved = page.waitForResponse((response) => isSave(response, name));
	await page.keyboard.insertText(text);
	const response = await saved;
	expect(response.ok()).toBe(true);
	await response.finished();
	await expect(status(page)).toHaveAttribute("data-state", "saved");
}

// CodeMirror virtualizes long documents; read its public model, not DOM text.
async function editorState(page, name) {
	return (await panel(page, name)).locator(".cm-editor").evaluate(async (element) => {
		const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
		const view = EditorView.findFromDOM(element);
		return {
			text: view.state.doc.toString(),
			selection: view.state.selection.toJSON(),
			scroll: view.scrollDOM.scrollTop,
		};
	});
}

async function expectText(page, name, text) {
	await expect.poll(async () => (await editorState(page, name)).text).toBe(text);
}

function latch() {
	let release;
	const promise = new Promise((resolve) => { release = resolve; });
	return { promise, release };
}

for (const [device, viewport] of [
	["desktop", { width: 1440, height: 900 }],
	["mobile", { width: 390, height: 844 }],
]) {
	test.describe(`Jotpad notes (${device})`, () => {
		test.use({ viewport });
		test.beforeEach(async ({ context, page }) => {
			await signIn(context);
			await page.goto(baseURL, { waitUntil: "load" });
			await openNotes(page);
		});

		test("accessible nested tabs support keyboard navigation without selecting Habits", async ({ page }) => {
			await expect(tab(page, "Daily")).toHaveAttribute("aria-selected", "true");
			for (const name of notes) {
				const notePanel = await panel(page, name);
				await expect(notePanel).toHaveAttribute("role", "tabpanel");
				await expect(notePanel).toHaveAttribute("aria-labelledby", await tab(page, name).getAttribute("id"));
			}
			await tab(page, "Daily").focus();
			for (const [key, name] of [
				["ArrowRight", "Weekly"], ["End", "Quarterly"],
				["ArrowRight", "Daily"], ["ArrowLeft", "Quarterly"], ["Home", "Daily"],
			]) {
				await page.keyboard.press(key);
				await expect(tab(page, name)).toBeFocused();
				await expect(tab(page, name)).toHaveAttribute("aria-selected", "true");
				for (const candidate of notes) {
					await expect(tab(page, candidate)).toHaveAttribute("tabindex", candidate === name ? "0" : "-1");
					if (candidate === name) await expect(await panel(page, candidate)).toBeVisible();
					else await expect(await panel(page, candidate)).toBeHidden();
				}
				await expect(tab(page, "Habits")).toHaveAttribute("aria-selected", "false");
			}
			await select(page, "Weekly");
			await tab(page, "Habits").click();
			await tab(page, "Jotpad").click();
			await expect(tab(page, "Weekly")).toHaveAttribute("aria-selected", "true");
			const editor = (await panel(page, "Weekly")).locator(".cm-content");
			await expect(editor).toHaveAttribute("contenteditable", "true");
			await expect(editor).toHaveAccessibleName(/\S/);
			await editor.click();
			await expect(editor).toBeFocused();
		});

		test("note header stays themed and fits alongside the Habits header", async ({ page }) => {
			for (const colorscheme of ["solarized", "nord", "catppuccin"]) {
				for (const colormode of ["light", "dark"]) {
					await page.evaluate(({ colorscheme, colormode }) => {
						Object.assign(document.documentElement.dataset, { colorscheme, colormode });
					}, { colorscheme, colormode });
					await select(page, "Quarterly");
					const header = page.getByRole("tablist", { name: "Planning horizon" });
					const noteStyle = await header.evaluate((element) => {
						const style = getComputedStyle(element);
						return { background: style.backgroundColor, border: style.borderTopColor, padding: style.padding, radius: style.borderRadius };
					});
					const bounds = await header.boundingBox();
					for (const name of notes) {
						const button = await tab(page, name).boundingBox();
						expect(button.x).toBeGreaterThanOrEqual(bounds.x);
						expect(button.x + button.width).toBeLessThanOrEqual(bounds.x + bounds.width);
					}
					await tab(page, "Habits").click();
					const habitHeader = page.locator("#habit-grid .week-nav");
					await expect(habitHeader).toBeVisible();
					const habitStyle = await habitHeader.evaluate((element) => {
						const style = getComputedStyle(element);
						const frame = getComputedStyle(element.closest(".habits-matrix"));
						return { background: style.backgroundColor, border: frame.borderTopColor, padding: style.padding, radius: frame.borderRadius };
					});
					expect(noteStyle).toEqual(habitStyle);
					await tab(page, "Jotpad").click();
					await expect(tab(page, "Quarterly")).toHaveAttribute("aria-selected", "true");
				}
			}
		});

		test("three documents persist independently and selected note survives through localStorage", async ({ context, page }) => {
			for (const name of notes) await appendAndSave(page, name, `${name} only`);
			await select(page, "Daily");
			const before = await page.evaluate(() => ({ ...localStorage }));
			await select(page, "Weekly");
			await expect.poll(() => page.evaluate(() => ({ ...localStorage }))).not.toEqual(before);
			await page.reload({ waitUntil: "load" });
			await openNotes(page);
			await expect(tab(page, "Weekly")).toHaveAttribute("aria-selected", "true");
			const other = await context.newPage();
			await other.goto(baseURL, { waitUntil: "load" });
			await openNotes(other);
			await expect(tab(other, "Weekly")).toHaveAttribute("aria-selected", "true");
			for (const name of notes) {
				await select(page, name);
				await expectText(page, name, `${name} only`);
				await select(other, name);
				await expectText(other, name, `${name} only`);
			}
		});

		test("pending Daily save and delayed authoritative ack cannot replace another note", async ({ page }) => {
			const receipt = latch();
			const entered = latch();
			await page.route("**/jot?note=daily", async (route) => {
				entered.release();
				await receipt.promise;
				// Force the reconciliation branch, not merely a version-only ack.
				await route.fulfill({ json: { version: 1, text: "Daily acknowledged" } });
			});
			try {
				const editor = await select(page, "Daily");
				await editor.click();
				await page.keyboard.insertText("Daily pending");
				await entered.promise;
				await expect(status(page)).toHaveAttribute("data-state", "saving");
				await appendAndSave(page, "Weekly", "Weekly unaffected");
				await expect(tab(page, "Daily")).toHaveAttribute("data-state", "saving");
				await expect(tab(page, "Daily")).toHaveAccessibleDescription("Unsaved changes");
				await appendAndSave(page, "Quarterly", "Quarterly unaffected");
				await select(page, "Daily");
				await expectText(page, "Daily", "Daily pending");
				await expect(status(page)).toHaveAttribute("data-state", "saving");
				await select(page, "Quarterly");
				const acknowledged = page.waitForResponse((response) => isSave(response, "Daily"));
				receipt.release();
				await (await acknowledged).finished();
				await expectText(page, "Daily", "Daily acknowledged");
				await expectText(page, "Weekly", "Weekly unaffected");
				await expectText(page, "Quarterly", "Quarterly unaffected");
				await expect(tab(page, "Quarterly")).toHaveAttribute("aria-selected", "true");
				await expect(status(page)).toHaveAttribute("data-state", "saved");
			} finally {
				receipt.release();
				await page.unrouteAll({ behavior: "wait" });
			}
		});

		test("failed Weekly saves retain their draft and status while other notes save and retry recovers", async ({ page }) => {
			let failing = true;
			await page.route("**/jot?note=weekly", (route) => failing
				? route.fulfill({ status: 503, body: "retry later" }) : route.continue());
			const editor = await select(page, "Weekly");
			await editor.click();
			await page.keyboard.insertText("Weekly unsaved draft");
			await expect(status(page)).toHaveAttribute("data-state", "offline");
			await appendAndSave(page, "Daily", "Daily saved");
			await expect(tab(page, "Weekly")).toHaveAttribute("data-state", "offline");
			await expect(tab(page, "Weekly")).toHaveAccessibleDescription("Offline, will retry");
			await appendAndSave(page, "Quarterly", "Quarterly saved");
			await select(page, "Weekly");
			await expectText(page, "Weekly", "Weekly unsaved draft");
			await expect(status(page)).toHaveAttribute("data-state", "offline");
			const recovered = page.waitForResponse((response) => isSave(response, "Weekly") && response.ok());
			failing = false;
			await page.evaluate(() => window.dispatchEvent(new Event("online")));
			await (await recovered).finished();
			await expect(status(page)).toHaveAttribute("data-state", "saved");
			await page.reload({ waitUntil: "load" });
			await openNotes(page);
			for (const [name, text] of [["Daily", "Daily saved"], ["Weekly", "Weekly unsaved draft"], ["Quarterly", "Quarterly saved"]]) {
				await select(page, name);
				await expectText(page, name, text);
			}
		});

		test("real SSE updates route to hidden and visible notes without changing the selected tab", async ({ context, page }) => {
			for (const name of notes) await appendAndSave(page, name, `${name} original`);
			const other = await context.newPage();
			await other.goto(baseURL, { waitUntil: "load" });
			await openNotes(other);
			await select(page, "Daily");
			for (const name of ["Weekly", "Quarterly", "Daily"]) {
				await appendAndSave(other, name, " remote");
				await expectText(page, name, `${name} original remote`);
				await expect(tab(page, "Daily")).toHaveAttribute("aria-selected", "true");
			}
			for (const name of notes) {
				await select(page, name);
				await expectText(page, name, `${name} original remote`);
			}
		});

		test("each mounted editor retains its own undo history, cursor and scroll across switches", async ({ page }) => {
			const originals = new Map();
			const snapshots = new Map();
			for (const [index, name] of notes.entries()) {
				const text = Array.from({ length: 100 }, (_, line) => `${name} line ${line + 1}: keep this note`).join("\n");
				originals.set(name, text);
				await appendAndSave(page, name, text);
				await page.keyboard.press("ArrowLeft");
				const saved = page.waitForResponse((response) => isSave(response, name));
				await page.keyboard.insertText(` ${name} edit `);
				await page.keyboard.press("ArrowLeft");
				const response = await saved;
				expect(response.ok()).toBe(true);
				await response.finished();
				await expect(status(page)).toHaveAttribute("data-state", "saved");
				const editor = (await panel(page, name)).locator(".cm-editor");
				await editor.evaluate(async (element, index) => {
					const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
					const view = EditorView.findFromDOM(element);
					view.scrollDOM.scrollTop = 150 + index * 120;
					await new Promise((resolve) => view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) }));
				}, index);
				snapshots.set(name, await editorState(page, name));
				expect(snapshots.get(name).scroll).toBeGreaterThan(0);
			}
			for (const name of notes) {
				await select(page, name);
				// Keyboard focus avoids a pointer deliberately moving the caret.
				await (await panel(page, name)).locator(".cm-content").focus();
				await (await panel(page, name)).locator(".cm-editor").evaluate(async (element) => {
					const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
					const view = EditorView.findFromDOM(element);
					await new Promise((resolve) => view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) }));
				});
				await expect.poll(() => editorState(page, name)).toEqual(snapshots.get(name));
				await page.keyboard.press("ControlOrMeta+z");
				await expectText(page, name, originals.get(name));
				for (const other of notes.filter((candidate) => candidate !== name)) {
					await expectText(page, other, snapshots.get(other).text);
				}
				await page.keyboard.press("ControlOrMeta+y");
				await expectText(page, name, snapshots.get(name).text);
			}
		});
	});
}
