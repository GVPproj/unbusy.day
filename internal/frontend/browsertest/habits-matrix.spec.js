import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ timezoneId: "UTC" });

async function openMatrix(context, page) {
  await signIn(context);
  const today = new Date().toISOString().slice(0, 10);
  const response = await context.request.post(`${baseURL}/habits`, {
    data: { habitname: "Read", habitstart: "2020-01-01", timezone: "UTC" },
  });
  expect(response.ok()).toBe(true);
  await page.goto(baseURL, { waitUntil: "load" });
  if (page.viewportSize().width < 832) {
    await page.locator(".menu-toggle").click();
    await page.locator("#sidenav").getByRole("button", { name: "Notes & Habits", exact: true }).click();
  }
  await page.getByRole("tab", { name: "Habits", exact: true }).click();
  await expect(page.locator("#habit-scroll")).toBeVisible();
  return today;
}

for (const width of [1440, 390]) {
  test(`live matrix changes preserve synchronous scroll intent at ${width}px`, async ({ context, page }) => {
    await page.setViewportSize({ width, height: 900 });
    await openMatrix(context, page);
    const position = await page.evaluate(async () => {
      const scroll = document.getElementById("habit-scroll");
      scroll.scrollLeft = 160;
      scroll.dispatchEvent(new Event("scroll", { bubbles: true }));
      // Deliver a live attribute update before the scroll handler's next frame.
      document.querySelector("[data-checkin-action]").setAttribute("aria-busy", "true");
      await new Promise((resolve) => requestAnimationFrame(resolve));
      return scroll.scrollLeft;
    });
    expect(position).toBe(160);

    const restored = await page.evaluate(async () => {
      const scroll = document.getElementById("habit-scroll");
      scroll.scrollLeft = 240;
      scroll.dispatchEvent(new Event("scroll", { bubbles: true }));
      // Replacement models the browser's lost scroll state during an authoritative morph.
      scroll.replaceWith(scroll.cloneNode(true));
      await new Promise((resolve) => requestAnimationFrame(resolve));
      return document.getElementById("habit-scroll").scrollLeft;
    });
    expect(restored).toBe(240);
    const newer = await page.evaluate(async () => {
      const old = document.getElementById("habit-scroll");
      const replacement = old.cloneNode(true);
      old.replaceWith(replacement);
      replacement.scrollLeft = 320;
      replacement.dispatchEvent(new Event("scroll", { bubbles: true }));
      await new Promise((resolve) => requestAnimationFrame(resolve));
      return replacement.scrollLeft;
    });
    expect(newer).toBe(320);
  });

  test(`keyboard scroll and focus survive an authoritative month update at ${width}px`, async ({ context, page }) => {
    await page.setViewportSize({ width, height: 900 });
    await openMatrix(context, page);
    const heading = page.locator("#habit-grid time").first();
    const current = await heading.getAttribute("datetime");
    await page.getByRole("button", { name: /Previous month/ }).click();
    await expect(heading).not.toHaveAttribute("datetime", current);
    const month = await heading.getAttribute("datetime");
    const twentieth = page.getByRole("button", { name: `Read on ${month}-20`, exact: true });
    await twentieth.focus();
    await page.keyboard.press("Tab");
    const focused = page.getByRole("button", { name: `Read on ${month}-21`, exact: true });
    await expect(focused).toBeFocused();
    const scroll = page.locator("#habit-scroll");
    await expect.poll(() => scroll.evaluate((el) => el.scrollLeft)).toBeGreaterThan(240);
    const position = await scroll.evaluate((el) => el.scrollLeft);
    const id = Number((await focused.getAttribute("id")).split("-")[1]);
    const response = await context.request.post(`${baseURL}/habits/check-in`, {
      data: { habitid: id, habitdate: `${month}-21`, habitchecked: true, timezone: "UTC" },
    });
    expect(response.ok()).toBe(true);
    await expect(focused).toHaveAttribute("aria-pressed", "true");
    await expect(focused).toBeFocused();
    expect(await scroll.evaluate((el) => el.scrollLeft)).toBe(position);
  });
}

for (const width of [1440, 390]) {
  for (const theme of ["solarized", "nord", "catppuccin"].flatMap((family) =>
    ["light", "dark"].map((mode) => ({ family, mode })))) {
    test(`habit dialog spacing is independent of the matrix: ${theme.family} ${theme.mode}, ${width}px`, async ({ context, page }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.addInitScript(({ family, mode }) => {
        localStorage.setItem("colorscheme", family);
        localStorage.setItem("colormode", mode);
      }, theme);
      await openMatrix(context, page);
      await page.getByRole("button", { name: "Delete Read", exact: true }).click();
      await expect(page.locator("#habit-delete")).toBeVisible();
      const padding = await page.locator("#habit-delete-warning").evaluate((el) => getComputedStyle(el).padding);
      expect(padding).toBe("0px");
      for (const selector of ["#habit-delete output", "#habit-edit-feedback", "#habit-feedback"]) {
        const styles = await page.locator(selector).evaluate((el) => {
          const style = getComputedStyle(el);
          return { padding: style.padding, minHeight: style.minHeight };
        });
        expect(styles.padding).toBe("0px");
        expect(["0px", "auto"]).toContain(styles.minHeight);
      }
      await page.screenshot({ path: test.info().outputPath("delete-dialog.png"), animations: "disabled" });
    });
  }
}
