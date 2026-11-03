import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

const notes = ["Daily", "Weekly", "Quarterly"];
const tab = (page, name) => page.getByRole("tab", { name, exact: true });
const toggle = (page) => page.locator("#jot-vim");
const mode = (page) => page.locator("#jot-vim-mode");

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
	return selected.locator(".cm-content");
}

async function openNotes(page) {
	if (page.viewportSize().width >= 832) return;
	await page.locator(".menu-toggle").click();
	await page.locator("#sidenav").getByRole("button", { name: "Extras", exact: true }).click();
	await expect(page.locator(".menu-toggle")).toHaveAttribute("aria-expanded", "false");
}

async function expectMode(page, name) {
	await expect(mode(page)).toBeVisible();
	await expect(mode(page)).toHaveText(new RegExp(`^${name}(?: mode)?$`, "i"));
}

function isSave(response, name) {
	const url = new URL(response.url());
	return response.request().method() === "POST" && url.pathname === "/jot"
		&& url.searchParams.get("note") === name.toLowerCase();
}

async function saveAfter(page, name, action) {
	const saved = page.waitForResponse((response) => isSave(response, name));
	await action();
	const response = await saved;
	expect(response.ok()).toBe(true);
	await response.finished();
	await expect(tab(page, name)).toHaveAttribute("data-state", "saved");
}

async function seed(page, name, text) {
	const editor = await select(page, name);
	await editor.focus();
	await saveAfter(page, name, () => page.keyboard.insertText(text));
}

async function seedWithRedoHistory(page, name, text) {
	await seed(page, name, text);
	await saveAfter(page, name, () => page.keyboard.insertText(" extra"));
	await saveAfter(page, name, () => page.keyboard.press("ControlOrMeta+z"));
	await expectText(page, name, text);
	return (await panel(page, name)).locator(".cm-editor").evaluateHandle(async (element) => {
		const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
		return EditorView.findFromDOM(element);
	});
}

// Read the public model: long notes are virtualized, including while hidden.
async function editorState(page, name) {
	return (await panel(page, name)).locator(".cm-editor").evaluate(async (element) => {
		const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
		const { undoDepth, redoDepth } = await import("/static/vendor/codemirror/modules/@codemirror__commands__commands.mjs");
		const view = EditorView.findFromDOM(element);
		return {
			text: view.state.doc.toString(),
			selection: view.state.selection.toJSON(),
			scroll: view.scrollDOM.scrollTop,
			undo: undoDepth(view.state),
			redo: redoDepth(view.state),
		};
	});
}

async function expectText(page, name, text) {
	await expect.poll(async () => (await editorState(page, name)).text).toBe(text);
}

async function expectSameEditorView(page, name, original) {
	expect(await (await panel(page, name)).locator(".cm-editor").evaluate(async (element, original) => {
		const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
		return original.dom.isConnected && EditorView.findFromDOM(element) === original;
	}, original)).toBe(true);
}

async function expectVimExtensions(page, enabled) {
	for (const name of notes) {
		const editor = (await panel(page, name)).locator(".cm-editor");
		await expect(editor).toHaveCount(1);
		await expect.poll(() => editor.evaluate(async (element) => {
			const { EditorView } = await import("/static/vendor/codemirror/modules/@codemirror__view__view.mjs");
			const { getCM } = await import("/static/vendor/codemirror/modules/@replit__codemirror-vim__codemirror-vim.mjs");
			return Boolean(getCM(EditorView.findFromDOM(element)));
		}), { message: `${name} Vim extension enabled: ${enabled}` }).toBe(enabled);
	}
}

async function expectMobileVimOff(page) {
	await expect(page.locator(".jot-tools")).toBeHidden();
	await expect(page.locator('label[for="jot-vim"]')).toBeHidden();
	await expect(toggle(page)).toBeHidden();
	await expect(toggle(page)).toBeDisabled();
	await expect(toggle(page)).not.toBeChecked();
	await expect(mode(page)).toBeHidden();
	await expectVimExtensions(page, false);
}

async function chooseTheme(page, name) {
	const theme = page.locator("#theme-modal");
	await theme.evaluate((dialog) => dialog.showModal());
	await theme.getByRole("radio", { name, exact: true }).locator("..").click();
	await theme.getByRole("button", { name: "Done", exact: true }).click();
	await page.evaluate(() => document.fonts.ready);
}

test.describe("optional Jotpad Vim", () => {
	test.use({ viewport: { width: 1440, height: 900 } });
	test.beforeEach(async ({ context, page }) => {
		await signIn(context);
		await page.goto(baseURL, { waitUntil: "load" });
	});

	test("defaults off and one native toggle enables all three mounted editors", { tag: "@smoke" }, async ({ page }) => {
		await expect(toggle(page)).toHaveCount(1);
		await expect(toggle(page)).toHaveAttribute("type", "checkbox");
		await expect(toggle(page)).toHaveAccessibleName("Vim");
		await expect(toggle(page)).not.toBeChecked();
		await expect(mode(page)).toHaveCount(1);
		await expect(mode(page)).toBeHidden();
		for (const name of notes) {
			await expect((await panel(page, name)).locator(".cm-editor")).toHaveCount(1);
			// Ordinary letters must still type when Vim has never been enabled.
			await seed(page, name, "plain hjkl");
			await expectText(page, name, "plain hjkl");
		}
		await toggle(page).check();
		await expect.poll(() => page.evaluate(() => localStorage.getItem("unbusy:jot-vim"))).toBe("true");
		for (const name of notes) {
			const editor = await select(page, name);
			await expectMode(page, "Normal");
			await editor.focus();
			await page.keyboard.press("h");
			await expectText(page, name, "plain hjkl");
			await page.keyboard.press("i");
			await expectMode(page, "Insert");
			await saveAfter(page, name, () => page.keyboard.type("X"));
			await expect.poll(async () => (await editorState(page, name)).text).toContain("X");
			expect((await editorState(page, name)).text.replace("X", "")).toBe("plain hjkl");
			await page.keyboard.press("Escape");
			await expectMode(page, "Normal");
		}
	});

	test("Normal, Insert, Visual and Replace feedback belongs to the selected note", async ({ page }) => {
		for (const name of notes) await seed(page, name, "alpha beta gamma");
		await toggle(page).check();
		for (const [name, key, expected] of [["Daily", "i", "Insert"], ["Weekly", "v", "Visual"], ["Quarterly", "R", "Replace"]]) {
			const editor = await select(page, name);
			await expectMode(page, "Normal");
			await editor.focus();
			await page.keyboard.press(key);
			await expectMode(page, expected);
		}
		for (const [name, expected] of [["Daily", "Insert"], ["Weekly", "Visual"], ["Quarterly", "Replace"], ["Daily", "Insert"]]) {
			await select(page, name);
			await expectMode(page, expected);
		}
		for (const name of notes) {
			await (await select(page, name)).focus();
			await page.keyboard.press("Escape");
			await expectMode(page, "Normal");
			await expectText(page, name, "alpha beta gamma");
		}
	});

	test("preference survives reload and disabling then reenabling resets every note to Normal", async ({ page }) => {
		await toggle(page).check();
		for (const name of notes) {
			await (await select(page, name)).focus();
			await page.keyboard.press("i");
			await expectMode(page, "Insert");
		}
		await toggle(page).uncheck();
		await expect(mode(page)).toBeHidden();
		await expect.poll(() => page.evaluate(() => localStorage.getItem("unbusy:jot-vim"))).toBe("false");
		await page.reload({ waitUntil: "load" });
		await expect(toggle(page)).not.toBeChecked();
		await expect(mode(page)).toBeHidden();
		await toggle(page).check();
		for (const name of notes) {
			await select(page, name);
			await expectMode(page, "Normal");
		}
		await (await select(page, "Weekly")).focus();
		await page.keyboard.press("i");
		await expectMode(page, "Insert");
		await page.reload({ waitUntil: "load" });
		await expect(toggle(page)).toBeChecked();
		await expect(tab(page, "Weekly")).toHaveAttribute("aria-selected", "true");
		await expectMode(page, "Normal");
		// Also exercise reset without a reload, which must remove old Vim state.
		for (const name of notes) {
			await (await select(page, name)).focus();
			await page.keyboard.press("i");
			await expectMode(page, "Insert");
		}
		await toggle(page).uncheck();
		await toggle(page).check();
		for (const name of notes) {
			await select(page, name);
			await expectMode(page, "Normal");
		}
	});

	test("local Vim typing, undo and redo retain Markdown, autosave and native refocus", async ({ page }) => {
		const markdown = "# Heading\n\n- **bold** and `code`";
		await toggle(page).check();
		const editor = await select(page, "Daily");
		await editor.focus();
		await page.keyboard.press("i");
		await saveAfter(page, "Daily", () => page.keyboard.insertText(markdown));
		await expectText(page, "Daily", markdown);
		const daily = await panel(page, "Daily");
		await expect(daily.locator(".tok-heading", { hasText: "Heading" })).toHaveCount(1);
		await expect(daily.locator(".tok-strong", { hasText: "bold" })).toHaveCount(1);
		await expect(daily.locator(".jot-codespan")).toHaveText("`code`");
		await page.keyboard.press("Escape");
		await expectMode(page, "Normal");
		await saveAfter(page, "Daily", () => page.keyboard.press("u"));
		await expectText(page, "Daily", "");
		await saveAfter(page, "Daily", () => page.keyboard.press("Control+r"));
		await expectText(page, "Daily", markdown);
		await page.keyboard.press("G");
		await page.keyboard.press("A");
		await expectMode(page, "Insert");
		const before = await editorState(page, "Daily");
		await tab(page, "Habits").click();
		await tab(page, "Jotpad").click();
		await editor.focus();
		await expect(editor).toBeFocused();
		await expectMode(page, "Insert");
		await expect.poll(async () => (await editorState(page, "Daily")).selection).toEqual(before.selection);
		await saveAfter(page, "Daily", () => page.keyboard.type("!"));
		await expectText(page, "Daily", `${markdown}!`);
		await page.reload({ waitUntil: "load" });
		await expectText(page, "Daily", `${markdown}!`);
		await expectMode(page, "Normal");
	});

	test("toggle preserves every mounted EditorView, document, selection, history and scroll without POST", async ({ page }) => {
		const snapshots = new Map();
		const views = new Map();
		for (const [index, name] of notes.entries()) {
			const text = Array.from({ length: 100 }, (_, line) => `${name} line ${line + 1}: keep this note`).join("\n");
			const view = await seedWithRedoHistory(page, name, text);
			await view.evaluate(async (view, index) => {
				view.dispatch({ selection: { anchor: view.state.doc.line(20 + index * 5).from + 3 } });
				view.scrollDOM.scrollTop = 150 + index * 120;
				await new Promise((resolve) => view.requestMeasure({ read: () => null, write: () => queueMicrotask(resolve) }));
			}, index);
			views.set(name, view);
			snapshots.set(name, await editorState(page, name));
			expect(snapshots.get(name).scroll).toBeGreaterThan(0);
			expect(snapshots.get(name).undo).toBeGreaterThan(0);
			expect(snapshots.get(name).redo).toBeGreaterThan(0);
		}
		const posts = [];
		page.on("request", (request) => {
			if (request.method() === "POST" && new URL(request.url()).pathname === "/jot") posts.push(request.url());
		});
		for (const enabled of [true, false, true, false]) {
			await toggle(page).setChecked(enabled);
			for (const name of notes) {
				await select(page, name);
				await expectSameEditorView(page, name, views.get(name));
				await expect.poll(() => editorState(page, name)).toEqual(snapshots.get(name));
			}
		}
		// Negative assertion must outlast the sync driver's one-second debounce.
		await page.waitForTimeout(1200);
		expect(posts).toEqual([]);
		for (const name of notes) {
			await (await select(page, name)).focus();
			await saveAfter(page, name, () => page.keyboard.press("ControlOrMeta+y"));
			await expectText(page, name, `${snapshots.get(name).text} extra`);
			await views.get(name).dispose();
		}
	});

	test("real remote SSE and another tab's storage changes never switch the active note", async ({ context, page }) => {
		for (const name of notes) await seed(page, name, `${name} original`);
		await toggle(page).check();
		await (await select(page, "Daily")).focus();
		await page.keyboard.press("i");
		const other = await context.newPage();
		await other.goto(baseURL, { waitUntil: "load" });
		await expect(toggle(other)).toBeChecked();
		for (const name of ["Weekly", "Quarterly", "Daily"]) {
			const editor = await select(other, name);
			await editor.focus();
			await other.keyboard.press("A");
			await saveAfter(other, name, () => other.keyboard.type(" remote"));
			await other.keyboard.press("Escape");
			await expectText(page, name, `${name} original remote`);
			await expect(tab(page, "Daily")).toHaveAttribute("aria-selected", "true");
			await expectMode(page, "Insert");
		}
		await select(other, "Quarterly");
		const storageChanged = await page.evaluateHandle(() => ({
			promise: new Promise((resolve) => {
				const changed = (event) => {
					if (event.key !== "unbusy:jot-vim" || event.newValue !== "false") return;
					window.removeEventListener("storage", changed);
					resolve(event.newValue);
				};
				window.addEventListener("storage", changed);
			}),
		}));
		await toggle(other).uncheck();
		expect(await storageChanged.evaluate(({ promise }) => promise)).toBe("false");
		await storageChanged.dispose();
		await expect(toggle(page)).toBeChecked();
		await expectMode(page, "Insert");
		await expect(tab(page, "Daily")).toHaveAttribute("aria-selected", "true");
		await expect(await panel(page, "Daily")).toBeVisible();
		for (const name of notes) await expectText(page, name, `${name} original remote`);
	});

	test("pending counted Insert and dot-repeat keep each note's typing across switches", async ({ page }) => {
		const inserts = [
			["Daily", "Z", "ZZZdaily", "ZZZZZZdaily"],
			["Weekly", "Y", "YYYweekly", "YYYYYYweekly"],
			["Quarterly", "X", "XXXquarterly", "XXXXXXquarterly"],
		];
		for (const [name] of inserts) await seed(page, name, name.toLowerCase());
		await toggle(page).check();
		for (const [name, letter] of inserts) {
			await (await select(page, name)).focus();
			for (const key of ["0", "3", "i"]) await page.keyboard.press(key);
			await saveAfter(page, name, () => page.keyboard.type(letter));
		}
		for (const [name, , counted] of [...inserts].reverse()) {
			await (await select(page, name)).focus();
			await expectMode(page, "Insert");
			await saveAfter(page, name, () => page.keyboard.press("Escape"));
			await expectText(page, name, counted);
		}
		for (const [name, , , repeated] of inserts) {
			await (await select(page, name)).focus();
			await saveAfter(page, name, () => page.keyboard.press("."));
			await expectText(page, name, repeated);
		}
		await page.reload({ waitUntil: "load" });
		for (const [name, , , repeated] of inserts) await expectText(page, name, repeated);
	});

	test("remote text cancels counted Insert without replaying authoritative text", async ({ context, page }) => {
		await seed(page, "Daily", "one alpha beta");
		const other = await context.newPage();
		await other.goto(baseURL, { waitUntil: "load" });
		await toggle(page).check();
		const editor = await select(page, "Daily");
		await editor.focus();
		for (const key of ["0", "3", "i"]) await page.keyboard.press(key);
		await saveAfter(page, "Daily", () => page.keyboard.type("Z"));
		const otherEditor = await select(other, "Daily");
		await otherEditor.focus();
		await other.keyboard.press("ControlOrMeta+Home");
		await saveAfter(other, "Daily", () => other.keyboard.insertText("prefix "));
		await expectText(page, "Daily", "prefix Zone alpha beta");
		await expectMode(page, "Insert");
		await editor.focus();
		await page.keyboard.press("Escape");
		await expectText(page, "Daily", "prefix Zone alpha beta");
		await saveAfter(page, "Daily", () => page.keyboard.press("."));
		await expectText(page, "Daily", "prefix ZZone alpha beta");
		await expectText(other, "Daily", "prefix ZZone alpha beta");
		await page.reload({ waitUntil: "load" });
		await expectText(page, "Daily", "prefix ZZone alpha beta");
	});

	test("a hidden note's remote update cannot replace the active note's pending Insert", async ({ context, page }) => {
		await seed(page, "Daily", "daily");
		await seed(page, "Weekly", "weekly");
		const other = await context.newPage();
		await other.goto(baseURL, { waitUntil: "load" });
		await toggle(page).check();
		for (const [name, letter] of [["Daily", "Z"], ["Weekly", "Y"]]) {
			await (await select(page, name)).focus();
			for (const key of ["0", "3", "i"]) await page.keyboard.press(key);
			await saveAfter(page, name, () => page.keyboard.type(letter));
		}
		await (await select(other, "Daily")).focus();
		await other.keyboard.press("ControlOrMeta+Home");
		await saveAfter(other, "Daily", () => other.keyboard.insertText("prefix "));
		await expectText(page, "Daily", "prefix Zdaily");
		await expect(tab(page, "Weekly")).toHaveAttribute("aria-selected", "true");
		await expectMode(page, "Insert");
		await (await select(page, "Weekly")).focus();
		await saveAfter(page, "Weekly", () => page.keyboard.press("Escape"));
		await expectText(page, "Weekly", "YYYweekly");
		await (await select(page, "Daily")).focus();
		await page.keyboard.press("Escape");
		await expectText(page, "Daily", "prefix Zdaily");
		await saveAfter(page, "Daily", () => page.keyboard.press("."));
		await expectText(other, "Daily", "prefix ZZdaily");
		await page.reload({ waitUntil: "load" });
		await expectText(page, "Daily", "prefix ZZdaily");
		await expectText(page, "Weekly", "YYYweekly");
	});

	test("remote text still converges after native paste enters Insert", async ({ context, page }) => {
		await context.grantPermissions(["clipboard-read", "clipboard-write"]);
		await seed(page, "Daily", "one alpha beta");
		const other = await context.newPage();
		await other.goto(baseURL, { waitUntil: "load" });
		await toggle(page).check();
		await (await select(page, "Daily")).focus();
		await page.keyboard.press("0");
		await page.bringToFront();
		await page.evaluate(() => navigator.clipboard.writeText("Z"));
		await saveAfter(page, "Daily", () => page.keyboard.press("ControlOrMeta+Shift+v"));
		await expectText(page, "Daily", "Zone alpha beta");
		await expectMode(page, "Insert");
		await (await select(other, "Daily")).focus();
		await other.keyboard.press("ControlOrMeta+Home");
		await saveAfter(other, "Daily", () => other.keyboard.insertText("prefix "));
		await expectText(page, "Daily", "prefix Zone alpha beta");
		await expectMode(page, "Insert");
	});

	test("remote text cancels stale Visual selections and pending operators", async ({ context, page }) => {
		await seed(page, "Daily", "one alpha beta");
		const other = await context.newPage();
		await other.goto(baseURL, { waitUntil: "load" });
		await expect((await panel(other, "Daily")).locator(".cm-editor")).toHaveCount(1);
		await toggle(page).check();
		const editor = await select(page, "Daily");
		await editor.focus();
		for (const key of ["0", "w", "v", "l", "l"]) await page.keyboard.press(key);
		await expectMode(page, "Visual");
		const otherEditor = await select(other, "Daily");
		await otherEditor.focus();
		await other.keyboard.press("ControlOrMeta+Home");
		await saveAfter(other, "Daily", () => other.keyboard.insertText("prefix "));
		await expectText(page, "Daily", "prefix one alpha beta");
		await expectMode(page, "Normal");
		await editor.focus();
		await page.keyboard.press("d");
		await expectText(page, "Daily", "prefix one alpha beta");
		// A second remote edit must also clear that pending delete operator.
		await otherEditor.focus();
		await other.keyboard.press("ControlOrMeta+Home");
		await saveAfter(other, "Daily", () => other.keyboard.insertText("new "));
		await expectText(page, "Daily", "new prefix one alpha beta");
		await editor.focus();
		await page.keyboard.press("w");
		await expectText(page, "Daily", "new prefix one alpha beta");
		await page.keyboard.press("0");
		await page.keyboard.press("3");
		await page.keyboard.press("R");
		await expectMode(page, "Replace");
		await saveAfter(page, "Daily", () => page.keyboard.type("Z"));
		const replaced = (await editorState(page, "Daily")).text;
		await expectText(other, "Daily", replaced);
		await otherEditor.focus();
		await other.keyboard.press("ControlOrMeta+Home");
		await saveAfter(other, "Daily", () => other.keyboard.insertText("authoritative "));
		await expectText(page, "Daily", `authoritative ${replaced}`);
		await expectMode(page, "Normal");
	});

	for (const preference of ["true", "false"]) {
		test(`mobile load ignores desktop Vim preference ${preference} and rejects synthetic enabling`, async ({ page }) => {
			await page.evaluate((value) => localStorage.setItem("unbusy:jot-vim", value), preference);
			await page.setViewportSize({ width: 390, height: 844 });
			await page.reload({ waitUntil: "load" });
			await openNotes(page);
			await expectMobileVimOff(page);
			await toggle(page).evaluate((input) => {
				input.checked = true;
				input.dispatchEvent(new Event("change", { bubbles: true }));
			});
			await expectMobileVimOff(page);
			for (const name of notes) {
				await (await select(page, name)).focus();
				await saveAfter(page, name, () => page.keyboard.type("hjkl"));
				await expectText(page, name, "hjkl");
			}
			expect(await page.evaluate(() => localStorage.getItem("unbusy:jot-vim"))).toBe(preference);
			await page.setViewportSize({ width: 832, height: 900 });
			await expect(toggle(page)).toBeVisible();
			await expect(toggle(page)).toBeEnabled();
			await expect(toggle(page)).toBeChecked({ checked: preference === "true" });
			await expectVimExtensions(page, preference === "true");
			for (const name of notes) {
				await select(page, name);
				await expectText(page, name, "hjkl");
				if (preference === "true") await expectMode(page, "Normal");
				else await expect(mode(page)).toBeHidden();
			}
		});
	}

	test("desktop-mobile-desktop resize preserves mounted editors, documents, history and desktop preference", async ({ page }) => {
		test.setTimeout(60_000);
		const views = new Map();
		const snapshots = new Map();
		const historyState = async (name) => {
			const { text, undo, redo } = await editorState(page, name);
			return { text, undo, redo };
		};
		for (const name of notes) {
			views.set(name, await seedWithRedoHistory(page, name, `${name} desktop history`));
		}
		await toggle(page).check();
		for (const [name, key, expected] of [["Daily", "i", "Insert"], ["Weekly", "v", "Visual"], ["Quarterly", "R", "Replace"]]) {
			await (await select(page, name)).focus();
			await page.keyboard.press(key);
			await expectMode(page, expected);
			snapshots.set(name, await historyState(name));
			expect(snapshots.get(name).undo).toBeGreaterThan(0);
			expect(snapshots.get(name).redo).toBeGreaterThan(0);
		}
		const posts = [];
		const recordPost = (request) => {
			if (request.method() === "POST" && new URL(request.url()).pathname === "/jot") posts.push(request.url());
		};
		page.on("request", recordPost);
		// Cross the exact 52rem boundary, then exercise a phone-sized viewport.
		for (const width of [831, 832, 390, 1440]) {
			await page.setViewportSize({ width, height: 900 });
			if (width < 832) {
				await openNotes(page);
				await expectMobileVimOff(page);
			} else {
				await expect(toggle(page)).toBeVisible();
				await expect(toggle(page)).toBeEnabled();
				await expect(toggle(page)).toBeChecked();
				await expectVimExtensions(page, true);
			}
			for (const name of notes) {
				await select(page, name);
				await expectSameEditorView(page, name, views.get(name));
				await expect.poll(() => historyState(name)).toEqual(snapshots.get(name));
				if (width >= 832) await expectMode(page, "Normal");
			}
			expect(await page.evaluate(() => localStorage.getItem("unbusy:jot-vim"))).toBe("true");
		}
		await page.waitForTimeout(1200);
		expect(posts).toEqual([]);
		page.off("request", recordPost);
		await page.setViewportSize({ width: 390, height: 844 });
		await openNotes(page);
		await expectMobileVimOff(page);
		for (const name of notes) {
			await (await select(page, name)).focus();
			await page.keyboard.press("ControlOrMeta+End");
			await saveAfter(page, name, () => page.keyboard.type("hjkl"));
			await expectText(page, name, `${name} desktop historyhjkl`);
			snapshots.set(name, await historyState(name));
		}
		await page.setViewportSize({ width: 832, height: 900 });
		await expect(toggle(page)).toBeChecked();
		await expectVimExtensions(page, true);
		for (const name of notes) {
			const editor = await select(page, name);
			await expectMode(page, "Normal");
			await expect.poll(() => historyState(name)).toEqual(snapshots.get(name));
			await expectSameEditorView(page, name, views.get(name));
			await editor.focus();
			await saveAfter(page, name, () => page.keyboard.press("u"));
			await expectText(page, name, `${name} desktop history`);
			await saveAfter(page, name, () => page.keyboard.press("Control+r"));
			await expectText(page, name, `${name} desktop historyhjkl`);
			await views.get(name).dispose();
		}
		expect(await page.evaluate(() => localStorage.getItem("unbusy:jot-vim"))).toBe("true");
	});

	for (const feeling of ["Mono", "Cozy", "Pixel"]) {
		test(`note tabs and Vim tools share one non-overlapping header row in ${feeling}`, async ({ page }) => {
			await chooseTheme(page, feeling);
			await toggle(page).check();
			await (await select(page, "Quarterly")).focus();
			await page.keyboard.press("R");
			for (const width of [1440, 1024, 832]) {
				await page.setViewportSize({ width, height: 900 });
				await expectMode(page, "Replace");
				await expect(mode(page)).toHaveText("Replace");
				const header = page.locator(".note-header");
				const noteTabs = header.locator(":scope > .note-tabs");
				const tools = header.locator(":scope > .jot-tools");
				await expect(header).toHaveCSS("display", "flex");
				await expect(header).toHaveCSS("flex-direction", "row");
				await expect(noteTabs).toBeVisible();
				await expect(tools).toBeVisible();
				const frame = await header.boundingBox();
				const tabsBox = await noteTabs.boundingBox();
				const toolsBox = await tools.boundingBox();
				const insets = await header.evaluate((element) => {
					const style = getComputedStyle(element);
					return {
						left: parseFloat(style.paddingLeft) + parseFloat(style.borderLeftWidth),
						right: parseFloat(style.paddingRight) + parseFloat(style.borderRightWidth),
					};
				});
				expect(Math.abs(tabsBox.x - frame.x - insets.left)).toBeLessThanOrEqual(2);
				expect(Math.abs(toolsBox.x + toolsBox.width - (frame.x + frame.width - insets.right))).toBeLessThanOrEqual(2);
				expect(tabsBox.x + tabsBox.width).toBeLessThanOrEqual(toolsBox.x + 1);
				const centerY = (box) => box.y + box.height / 2;
				expect(Math.abs(centerY(tabsBox) - centerY(toolsBox))).toBeLessThanOrEqual(2);
				const controls = [page.locator('label[for="jot-vim"]'), mode(page), ...notes.map((name) => tab(page, name))];
				const boxes = [];
				for (const control of controls) {
					await expect(control).toBeVisible();
					const box = await control.boundingBox();
					expect(box.width).toBeGreaterThan(0);
					expect(box.x).toBeGreaterThanOrEqual(Math.max(0, frame.x) - 1);
					expect(box.x + box.width).toBeLessThanOrEqual(Math.min(width, frame.x + frame.width) + 1);
					expect(box.y).toBeGreaterThanOrEqual(frame.y - 1);
					expect(box.y + box.height).toBeLessThanOrEqual(frame.y + frame.height + 1);
					expect(Math.abs(centerY(box) - centerY(tabsBox))).toBeLessThanOrEqual(2);
					boxes.push(box);
				}
				for (let i = 0; i < boxes.length; i++) {
					for (const b of boxes.slice(i + 1)) {
						const a = boxes[i];
						const overlapX = Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x);
						const overlapY = Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y);
						expect(overlapX <= 1 || overlapY <= 1).toBe(true);
					}
				}
				expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
			}
		});
	}

	test("Vim checkbox and feedback use the active light and dark theme tokens", async ({ page }) => {
		const accents = [];
		for (const colorMode of ["Light", "Dark"]) {
			await chooseTheme(page, colorMode);
			await expect(page.locator("html")).toHaveAttribute("data-colormode", colorMode.toLowerCase());
			for (const checked of [false, true]) {
				await toggle(page).setChecked(checked);
				const colors = await toggle(page).evaluate((input) => {
					const probe = document.createElement("span");
					input.parentElement.append(probe);
					const resolve = (token) => {
						probe.style.color = `var(${token})`;
						return getComputedStyle(probe).color;
					};
					const style = getComputedStyle(input);
					const colors = {
						background: style.backgroundColor,
						expectedBackground: resolve(input.checked ? "--accent" : "--surface"),
						border: style.borderTopColor,
						expectedBorder: resolve(input.checked ? "--accent" : "--ink-muted"),
						checkmark: getComputedStyle(input, "::after").borderTopColor,
						expectedCheckmark: resolve("--bg"),
						feedback: getComputedStyle(document.getElementById("jot-vim-mode")).color,
						expectedFeedback: resolve("--ink-muted"),
					};
					probe.remove();
					return colors;
				});
				expect(colors.background).toBe(colors.expectedBackground);
				expect(colors.border).toBe(colors.expectedBorder);
				if (checked) {
					await expectMode(page, "Normal");
					expect(colors.checkmark).toBe(colors.expectedCheckmark);
					expect(colors.feedback).toBe(colors.expectedFeedback);
					accents.push(colors.background);
				}
			}
		}
		expect(accents[0]).not.toBe(accents[1]);
	});
});
