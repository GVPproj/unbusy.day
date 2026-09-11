import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "UTC" });

const block = (page, label) => page.locator(".block-item", { has: page.locator(".block-label", { hasText: label }) });
const placement = async (locator) => locator.evaluate((el) => ({
	id: el.dataset.id,
	slot: Number(el.dataset.slot),
	span: Number(el.dataset.span),
}));

async function create(context, slot, label) {
	const response = await context.request.post(`${baseURL}/blocks`, {
		data: { addslot: slot, addlabel: label, addtype: "shallow" },
	});
	expect(response.ok()).toBeTruthy();
}

async function fixture(context, page, layout) {
	await signIn(context);
	for (const item of layout) await create(context, item.slot, item.label);
	await page.goto(baseURL, { waitUntil: "load" });
	const placements = [];
	for (const item of layout) {
		const current = await placement(block(page, item.label));
		placements.push({ id: current.id, slot: item.slot, span: item.span });
	}
	const response = await context.request.post(`${baseURL}/blocks/layout`, {
		data: { layout: placements },
	});
	expect(response.ok()).toBeTruthy();
	await page.reload({ waitUntil: "load" });
	return Object.fromEntries(await Promise.all(layout.map(async (item) => [item.label, await placement(block(page, item.label))])));
}

async function pitch(page) {
	return page.locator("#block-list > .slot").evaluateAll((slots) =>
		slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top,
	);
}

async function begin(page, locator, dy, target = locator) {
	const box = await target.boundingBox();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + dy, { steps: 3 });
}

async function expectClean(locator) {
	await expect.poll(() => locator.evaluate((el) => ({
		transform: el.style.transform,
		height: el.style.height,
		transient: ["dragging", "resizing", "gesture-preview", "gesture-settling", "gesture-flip"].some((name) => el.classList.contains(name)),
	}))).toEqual({ transform: "", height: "", transient: false });
}

test("drag previews push, settles to the grid, and cleans transient state", async ({ context, page }) => {
	const ids = await fixture(context, page, [
		{ label: "First", slot: 18, span: 1 },
		{ label: "Second", slot: 19, span: 1 },
	]);
	const first = block(page, "First");
	const second = block(page, "Second");
	const request = page.waitForRequest((req) => new URL(req.url()).pathname === "/blocks/layout");
	await begin(page, first, await pitch(page));
	await expect.poll(() => second.evaluate((el) => el.style.transform)).not.toBe("");
	await page.mouse.up();
	await request;
	await expect(first).toHaveAttribute("data-slot", "19");
	await expect(second).toHaveAttribute("data-slot", "18");
	expect(await placement(first)).toEqual({ ...ids.First, slot: 19 });
	expect(await placement(second)).toEqual({ ...ids.Second, slot: 18 });
	await expectClean(first);
	await expectClean(second);
});

test("resize previews compression and pointer cancellation restores the snapshot", async ({ context, page }) => {
	await fixture(context, page, [
		{ label: "Wide", slot: 18, span: 2 },
		{ label: "Below", slot: 20, span: 2 },
	]);
	const wide = block(page, "Wide");
	const below = block(page, "Below");
	const bounded = await context.request.post(`${baseURL}/blocks/bounds`, {
		data: { start: 18, end: 22 },
	});
	expect(bounded.ok()).toBeTruthy();
	await expect(page.locator("#block-list")).toHaveAttribute("data-day-end", "22");
	const grip = wide.locator(".grip");
	const slotHeight = await pitch(page);
	await begin(page, wide, slotHeight, grip);
	await expect.poll(() => below.evaluate((el) => ({ height: el.style.height, transform: el.style.transform })))
		.not.toEqual({ height: "", transform: "" });
	await page.locator("#block-list").dispatchEvent("pointercancel", { pointerId: 1 });
	await page.mouse.up();
	await expect(wide).toHaveAttribute("data-span", "2");
	await expect(below).toHaveAttribute("data-span", "2");
	await expectClean(wide);
	await expectClean(below);

	const request = page.waitForRequest((req) => new URL(req.url()).pathname === "/blocks/layout");
	await begin(page, wide, slotHeight, grip);
	await page.mouse.up();
	await request;
	await expect(wide).toHaveAttribute("data-span", "3");
	await expect(below).toHaveAttribute("data-span", "1");
	await expect(wide.locator(".grip")).toHaveAttribute("aria-valuenow", "3");
	await expectClean(wide);
	await expectClean(below);
});

test("a keyed server morph during a gesture remains authoritative", async ({ context, page }) => {
	const ids = await fixture(context, page, [
		{ label: "Held", slot: 18, span: 1 },
		{ label: "Peer", slot: 19, span: 1 },
	]);
	const held = block(page, "Held");
	const identity = await held.elementHandle();
	let clientCommits = 0;
	page.on("request", (req) => {
		if (new URL(req.url()).pathname === "/blocks/layout" && req.method() === "POST") clientCommits++;
	});
	await begin(page, held, await pitch(page));
	// Model the identity-preserving result of an authoritative keyed morph.
	await page.evaluate(({ heldID, peerID }) => {
		const held = document.querySelector(`[data-id="${heldID}"]`);
		const peer = document.querySelector(`[data-id="${peerID}"]`);
		held.dataset.slot = "20";
		held.style.gridRow = "3";
		peer.dataset.slot = "19";
		peer.style.gridRow = "2";
	}, { heldID: ids.Held.id, peerID: ids.Peer.id });
	await expect(held).toHaveAttribute("data-slot", "20");
	expect(await page.evaluate((node) => node === document.querySelector(`[data-id="${node.dataset.id}"]`), identity)).toBe(true);
	await page.mouse.up();
	await page.waitForTimeout(300);
	expect(clientCommits).toBe(0);
	await expect(held).toHaveAttribute("data-slot", "20");
	await expect(block(page, "Peer")).toHaveAttribute("data-slot", "19");
	await expectClean(held);
});

for (const [phase, label] of [
	["drag", "Renamed"], ["resize", "Renamed"], ["settle", "Renamed"],
	["drag", "Held"], ["resize", "Held"], ["settle", "Held"],
]) {
	test(`a ${label === "Held" ? "no-op" : "rename-only"} keyed morph cancels ${phase} without a stale commit`, async ({ context, page }) => {
		const ids = await fixture(context, page, [
			{ label: "Held", slot: 18, span: 1 },
			{ label: "Peer", slot: 20, span: 1 },
		]);
		const held = page.locator(`[data-id="${ids.Held.id}"]`);
		const identity = await held.elementHandle();
		await page.locator("#block-list").evaluate((list) => list.style.setProperty("--gesture-duration", "1s"));
		await page.evaluate(() => {
			window.gestureCommits = 0;
			document.querySelector("#block-list").addEventListener("layout", () => window.gestureCommits++);
		});
		await begin(page, held, (await pitch(page)) * 0.75, phase === "resize" ? held.locator(".grip") : held);
		if (phase === "settle") {
			await page.mouse.up();
			await expect(held).toHaveClass(/gesture-settling/);
		}
		const response = await context.request.post(`${baseURL}/blocks/rename`, {
			data: { renameid: ids.Held.id, renamelabel: label },
		});
		expect(response.ok()).toBeTruthy();
		await expect(held.locator(".block-label")).toHaveText(label);
		await expectClean(held);
		expect(await held.evaluate((el, original) => el === original, identity)).toBe(true);
		await page.mouse.up();
		await expectClean(held);
		await page.waitForTimeout(1100);
		expect(await page.evaluate(() => window.gestureCommits)).toBe(0);
		expect(await placement(held)).toEqual(ids.Held);
		const request = page.waitForRequest((req) => new URL(req.url()).pathname === "/blocks/layout");
		await begin(page, held, await pitch(page));
		await page.mouse.up();
		await request;
		await expect(held).toHaveAttribute("data-slot", "19");
	});
}

test("pickup release keeps opacity and scale easing through the grid swap", async ({ context, page }) => {
	const ids = await fixture(context, page, [{ label: "Held", slot: 18, span: 1 }]);
	const held = page.locator(`[data-id="${ids.Held.id}"]`);
	await begin(page, held, (await pitch(page)) * 0.75);
	await expect.poll(() => held.evaluate((el) => getComputedStyle(el).scale)).toBe("1.015");
	const release = await held.evaluate(async (el) => {
		const properties = new Set();
		const observer = new MutationObserver(() => {
			for (const animation of el.getAnimations()) properties.add(animation.transitionProperty);
		});
		observer.observe(el, { attributes: true });
		const pointerId = Array.from({ length: 16 }, (_, i) => i + 1).find((id) => el.hasPointerCapture(id));
		el.dispatchEvent(new PointerEvent("pointercancel", { bubbles: true, pointerId }));
		while (el.classList.contains("dragging")) await new Promise(requestAnimationFrame);
		observer.disconnect();
		return [...properties];
	});
	await page.mouse.up();
	expect(release).toEqual(expect.arrayContaining(["opacity", "scale"]));
	await expectClean(held);
	expect(await placement(held)).toEqual(ids.Held);
});

test("a bounds-only server morph aborts stale gesture state", async ({ context, page }) => {
	await fixture(context, page, [
		{ label: "Held", slot: 18, span: 1 },
	]);
	let clientCommits = 0;
	page.on("request", (req) => {
		if (new URL(req.url()).pathname === "/blocks/layout" && req.method() === "POST") clientCommits++;
	});
	const held = block(page, "Held");
	await begin(page, held, await pitch(page));
	const response = await context.request.post(`${baseURL}/blocks/bounds`, {
		data: { start: 18, end: 30 },
	});
	expect(response.ok()).toBeTruthy();
	await expect(page.locator("#block-list")).toHaveAttribute("data-day-end", "30");
	await page.mouse.up();
	await page.waitForTimeout(300);
	expect(clientCommits).toBe(0);
	await expect(held).toHaveAttribute("data-slot", "18");
	await expectClean(held);
});

test("an active server replacement releases the pointer path", async ({ context, page }) => {
	const ids = await fixture(context, page, [
		{ label: "Removed", slot: 18, span: 1 },
		{ label: "Survivor", slot: 20, span: 1 },
	]);
	await begin(page, block(page, "Removed"), await pitch(page));
	const response = await context.request.post(`${baseURL}/blocks/delete`, {
		data: { deleteid: ids.Removed.id },
	});
	expect(response.ok()).toBeTruthy();
	await expect(block(page, "Removed")).toHaveCount(0);
	await page.mouse.up();

	const survivor = block(page, "Survivor");
	const request = page.waitForRequest((req) => new URL(req.url()).pathname === "/blocks/layout");
	await begin(page, survivor, -await pitch(page));
	await page.mouse.up();
	await request;
	await expectClean(survivor);
});

test("rapid retargeting commits its final target exactly once", async ({ context, page }) => {
	await fixture(context, page, [
		{ label: "Held", slot: 18, span: 1 },
		{ label: "Peer", slot: 19, span: 1 },
	]);
	const held = block(page, "Held");
	const box = await held.boundingBox();
	const slotHeight = await pitch(page);
	let commits = 0;
	page.on("request", (req) => {
		if (new URL(req.url()).pathname === "/blocks/layout" && req.method() === "POST") commits++;
	});
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + slotHeight * 2);
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + slotHeight);
	await page.mouse.up();
	await expect(held).toHaveAttribute("data-slot", "19");
	await expect.poll(() => commits).toBe(1);
	await page.waitForTimeout(300);
	expect(commits).toBe(1);
	await expectClean(held);
});

test("a server morph during rapid-retarget settle wins", async ({ context, page }) => {
	const ids = await fixture(context, page, [
		{ label: "Held", slot: 18, span: 1 },
		{ label: "Peer", slot: 19, span: 1 },
	]);
	const held = block(page, "Held");
	const box = await held.boundingBox();
	const slotHeight = await pitch(page);
	await page.locator("#block-list").evaluate((list) => list.style.setProperty("--gesture-duration", "1s"));
	let clientCommits = 0;
	page.on("request", (req) => {
		if (new URL(req.url()).pathname === "/blocks/layout" && req.method() === "POST") clientCommits++;
	});
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + slotHeight * 2);
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + slotHeight);
	await page.mouse.up();
	const authoritative = [
		{ id: ids.Held.id, slot: 20, span: 1 },
		{ id: ids.Peer.id, slot: 19, span: 1 },
	];
	const response = await context.request.post(`${baseURL}/blocks/layout`, { data: { layout: authoritative } });
	expect(response.ok()).toBeTruthy();
	await expect(block(page, "Held")).toHaveAttribute("data-slot", "20");
	await page.waitForTimeout(1100);
	expect(clientCommits).toBe(0);
	await expect(block(page, "Held")).toHaveAttribute("data-slot", "20");
	await expectClean(block(page, "Held"));
});

test("pointer input supersedes a keyboard preview and edge scrolling remains live", async ({ context, page }) => {
	await fixture(context, page, [
		{ label: "Keyboard", slot: 18, span: 1 },
		{ label: "Pointer", slot: 20, span: 1 },
	]);
	const keyboard = block(page, "Keyboard");
	await keyboard.focus();
	await page.keyboard.press("Space");
	await page.keyboard.press("ArrowDown");
	await expect(keyboard).toHaveAttribute("data-slot", "19");

	const pointer = block(page, "Pointer");
	await page.locator("#block-list").evaluate((list) => {
		list.style.height = "240px";
		list.style.flex = "none";
		list.scrollTop = 0;
	});
	const box = await pointer.boundingBox();
	const listBox = await page.locator("#block-list").boundingBox();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2, listBox.y + listBox.height - 2);
	await expect(keyboard).toHaveAttribute("data-slot", "18");
	await expect.poll(() => page.locator("#block-list").evaluate((list) => list.scrollTop)).toBeGreaterThan(0);
	await page.locator("#block-list").evaluate((list) => {
		const pointerID = Array.from({ length: 16 }, (_, i) => i + 1).find((id) =>
			[...list.querySelectorAll(".block-item")].some((el) => el.hasPointerCapture(id)),
		);
		list.dispatchEvent(new PointerEvent("pointercancel", { bubbles: true, pointerId: pointerID }));
	});
	await page.mouse.up();
	await expectClean(pointer);
});

test.describe("reduced motion", () => {
	test.use({ reducedMotion: "reduce" });

	test("keeps direct manipulation but removes tilt and easing", async ({ context, page }) => {
		await page.emulateMedia({ reducedMotion: "reduce" });
		await fixture(context, page, [
			{ label: "Held", slot: 18, span: 1 },
			{ label: "Peer", slot: 19, span: 1 },
		]);
		const held = block(page, "Held");
		const box = await held.boundingBox();
		await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
		await page.mouse.down();
		await page.mouse.move(box.x + box.width / 2 + 40, box.y + box.height / 2 + await pitch(page));
		await expect.poll(() => held.evaluate((el) => el.style.transform)).toContain("rotate(0deg)");
		await expect.poll(() => page.locator("#block-list").evaluate((list) =>
			[...list.querySelectorAll(".block-item")].flatMap((el) => el.getAnimations())
				.filter((animation) => ["transform", "height"].includes(animation.transitionProperty)).length,
		)).toBe(0);
		await page.mouse.up();
		await expect(held).toHaveAttribute("data-slot", "19");
		await expectClean(held);
	});
});
