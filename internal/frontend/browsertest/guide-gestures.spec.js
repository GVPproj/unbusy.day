import { expect, test } from "@playwright/test";
import { baseURL } from "./session.js";

test.use({ viewport: { width: 1200, height: 800 } });

async function openDemo(page) {
	await page.goto(`${baseURL}/login`, { waitUntil: "load" });
	await page.getByRole("button", { name: /What is it/ }).click();
	const dialog = page.locator("#guide-modal");
	await expect(dialog).toBeVisible();
	await dialog.getByRole("button", { name: "Next" }).click();
	await dialog.getByRole("button", { name: "Next" }).click();
	await expect(dialog.locator(".gc-demo")).toBeVisible();
	return dialog;
}

async function drag(page, locator, dy) {
	const box = await locator.boundingBox();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + dy, { steps: 3 });
}

const clean = (locator) => expect.poll(() => locator.evaluateAll((blocks) => blocks.every((el) =>
	el.style.translate === "" &&
	el.style.height === "" &&
	!["gc-dragging", "gc-resizing", "gc-flip"].some((name) => el.classList.contains(name)),
))).toBe(true);

test("demo observes CSS completion and cleans drag and resize state", async ({ page }) => {
	const dialog = await openDemo(page);
	const demo = dialog.locator(".gc-demo");
	const blocks = demo.locator(".gc-block");
	const slotPitch = await demo.locator(".gc-slot").evaluateAll((slots) =>
		slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top,
	);
	await drag(page, blocks.first(), slotPitch);
	await expect.poll(() => blocks.nth(1).evaluate((el) => el.style.translate)).not.toBe("");
	await page.mouse.up();
	await expect(blocks.first()).toHaveAttribute("data-slot", "2");
	await clean(blocks);

	await drag(page, blocks.first().locator(".gc-grip"), slotPitch);
	await page.mouse.up();
	await expect(blocks.first()).toHaveAttribute("data-span", "2");
	await clean(blocks);
});

test("reduced motion keeps direct demo manipulation without easing", async ({ page }) => {
	await page.emulateMedia({ reducedMotion: "reduce" });
	const dialog = await openDemo(page);
	const demo = dialog.locator(".gc-demo");
	const blocks = demo.locator(".gc-block");
	const slotPitch = await demo.locator(".gc-slot").evaluateAll((slots) =>
		slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top,
	);
	await drag(page, blocks.first(), slotPitch);
	await expect.poll(() => blocks.first().evaluate((el) => el.style.translate)).not.toBe("");
	await expect.poll(() => demo.evaluate((el) =>
		[...el.querySelectorAll(".gc-block")].flatMap((block) => block.getAnimations())
			.filter((animation) => ["translate", "height"].includes(animation.transitionProperty)).length,
	)).toBe(0);
	await page.mouse.up();
	await clean(blocks);
});

for (const phase of ["active", "settling"]) {
	test(`closing during ${phase} drag cannot commit before the queued close event`, async ({ page }) => {
		const dialog = await openDemo(page);
		const demo = dialog.locator(".gc-demo");
		const blocks = demo.locator(".gc-block");
		await demo.evaluate((el) => el.style.setProperty("--gesture-duration", "1s"));
		const pitch = await demo.locator(".gc-slot").evaluateAll((slots) =>
			slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top,
		);
		await blocks.first().evaluate((el) => {
			el.addEventListener("pointerdown", (event) => el.dataset.testPointerId = event.pointerId, { once: true });
		});
		await drag(page, blocks.first(), pitch);
		await expect(blocks.first()).toHaveClass(/gc-dragging/);
		await blocks.first().evaluate((el, phase) => {
			const release = () => el.dispatchEvent(new PointerEvent("pointerup", {
				bubbles: true, pointerId: Number(el.dataset.testPointerId),
			}));
			// Keep close and release in one task, before the queued close listener.
			if (phase === "settling") release();
			el.closest("dialog").close();
			void el.offsetHeight;
			if (phase === "active") release();
		}, phase);
		await page.mouse.up();
		await clean(blocks);
		await expect(blocks.first()).toHaveAttribute("data-slot", "1");
	});
}

test("closing the dialog invalidates active and settling completions", async ({ page }) => {
	const dialog = await openDemo(page);
	const demo = dialog.locator(".gc-demo");
	const blocks = demo.locator(".gc-block");
	await demo.evaluate((el) => el.style.setProperty("--gesture-duration", "1s"));
	const slotPitch = await demo.locator(".gc-slot").evaluateAll((slots) =>
		slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top,
	);
	await drag(page, blocks.first(), slotPitch);
	await page.keyboard.press("Escape");
	await expect(dialog).toBeHidden();
	await page.mouse.up();
	await clean(blocks);
	await expect(blocks.first()).toHaveAttribute("data-slot", "1");

	await dialog.evaluate((el) => el.showModal());
	await dialog.getByRole("button", { name: "Next" }).click();
	await dialog.getByRole("button", { name: "Next" }).click();
	await drag(page, blocks.first(), slotPitch);
	await page.mouse.up();
	await page.keyboard.press("Escape");
	await expect(dialog).toBeHidden();
	await dialog.evaluate((el) => el.showModal());
	await clean(blocks);
	await page.waitForTimeout(1100);
	await clean(blocks);
	await expect(blocks.first()).toHaveAttribute("data-slot", "1");
});
