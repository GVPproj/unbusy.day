import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

for (const width of [320, 390, 1200]) {
  test(`theme choices fit the dialog at ${width}px`, async ({ context, page }) => {
    await page.setViewportSize({ width, height: 850 });
    await signIn(context);
    await page.goto(baseURL, { waitUntil: "load" });
    const dialog = page.locator("#theme-modal");
    await dialog.evaluate((el) => el.showModal());
    for (const feeling of ["cozy", "mono", "pixel"]) {
      await dialog.locator(`input[value="${feeling}"]`).evaluate((el) => el.click());
      await page.evaluate(() => document.fonts.ready);
      const overflow = await dialog.evaluate((el) => {
        const bounds = el.getBoundingClientRect();
        return [...el.querySelectorAll(".option-row")].some((row) => {
          const rect = row.getBoundingClientRect();
          return rect.left < bounds.left || rect.right > bounds.right || row.scrollWidth > row.clientWidth;
        });
      });
      expect(overflow, feeling).toBe(false);
    }
    await page.screenshot({ path: test.info().outputPath("theme.png"), animations: "disabled" });
  });
}

for (const width of [320, 390]) {
  test(`habit scroll viewport stays inside its border at ${width}px`, async ({ context, page }) => {
    await page.setViewportSize({ width, height: 850 });
    await signIn(context);
    const response = await context.request.post(`${baseURL}/habits`, {
      data: { habitname: "Breakfast Smoothie", habitstart: "2020-01-01", timezone: "UTC" },
    });
    expect(response.ok()).toBe(true);
    await page.goto(baseURL, { waitUntil: "load" });
    await page.locator(".menu-toggle").click();
    await page.locator("#sidenav").getByRole("button", { name: "Extras", exact: true }).click();
    await page.getByRole("tab", { name: "Habits", exact: true }).click();
    const scroll = page.locator("#habit-scroll");
    await expect(scroll).toBeVisible();
    for (const fraction of [0, 0.5, 1]) {
      const bounds = await scroll.evaluate((el, fraction) => {
        el.scrollLeft = (el.scrollWidth - el.clientWidth) * fraction;
        const viewport = el.getBoundingClientRect();
        const matrix = el.closest(".habits-matrix").getBoundingClientRect();
        return {
          left: viewport.left - matrix.left,
          right: matrix.right - viewport.right,
          scrollable: el.scrollWidth > el.clientWidth,
          position: el.scrollLeft,
        };
      }, fraction);
      expect(bounds.left).toBeGreaterThanOrEqual(0);
      expect(bounds.right).toBeGreaterThanOrEqual(0);
      expect(bounds.scrollable).toBe(true);
      if (fraction > 0) expect(bounds.position).toBeGreaterThan(0);
    }
    await page.screenshot({ path: test.info().outputPath("habits.png"), animations: "disabled" });
  });
}
