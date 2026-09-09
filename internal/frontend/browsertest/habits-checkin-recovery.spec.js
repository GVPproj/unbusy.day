import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "UTC" });

function latch() {
  let release;
  const promise = new Promise((resolve) => { release = resolve; });
  return { promise, release };
}

async function openHabit(context, page) {
  await signIn(context);
  const today = new Date().toISOString().slice(0, 10);
  const response = await context.request.post(`${baseURL}/habits`, {
    data: { habitname: "Read", habitstart: today, timezone: "UTC" },
  });
  expect(response.ok()).toBe(true);
  await page.goto(baseURL, { waitUntil: "load" });
  await page.getByRole("tab", { name: "Habits", exact: true }).click();
  const button = page.getByRole("button", { name: `Read on ${today}`, exact: true });
  await expect(button).toHaveAttribute("aria-pressed", "false");
  return { button, today, id: Number((await button.getAttribute("id")).split("-")[1]) };
}

for (const superseded of [false, true]) {
  test(`committed recovery is read-only after owner HTML arrives before receipt${superseded ? " and another device supersedes it" : ""}`, async ({ context, page }) => {
    const { button, today, id } = await openHabit(context, page);
    const receipt = latch();
    const writes = [];
    await page.route("**/habits/check-in", async (route) => {
      writes.push(route.request().postDataJSON().habitchecked);
      const response = await route.fetch();
      await receipt.promise;
      await route.fulfill({ response });
    });
    await page.route("**/habits/month?*", async (route) => {
      const signals = JSON.parse(new URL(route.request().url()).searchParams.get("datastar"));
      if (signals.habitread === 1) {
        await route.fulfill({ contentType: "text/event-stream", body: ": truncated\n\n" });
      } else {
        await route.continue();
      }
    });
    try {
      await button.click();
      await expect(button).toHaveAttribute("aria-pressed", "true");
      receipt.release();
      await expect(button).toHaveAttribute("data-save-state", "failed");
      await expect(page.locator("#habit-checkin-feedback")).toContainText("Saved, but refresh not confirmed");
      if (superseded) {
        const invalidated = page.waitForResponse((response) => new URL(response.url()).pathname === "/habits/month");
        const response = await context.request.post(`${baseURL}/habits/check-in`, {
          data: { habitid: id, habitdate: today, habitchecked: false, timezone: "UTC" },
        });
        expect(response.ok()).toBe(true);
        await (await invalidated).finished();
        await expect(button).toHaveAttribute("data-save-state", "failed");
      }
      await button.click();
      await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
      await expect(button).toHaveAttribute("aria-pressed", String(!superseded));
      expect(writes).toEqual([true]);
      await page.reload({ waitUntil: "load" });
      await page.getByRole("tab", { name: "Habits", exact: true }).click();
      await expect(button).toHaveAttribute("aria-pressed", String(!superseded));
    } finally {
      receipt.release();
      await page.unrouteAll({ behavior: "wait" });
    }
  });
}

for (const liveHTML of [false, true]) {
  test(`unknown recovery repeats original desired write with${liveHTML ? "" : "out"} matching owner HTML`, async ({ context, page }) => {
    const { button } = await openHabit(context, page);
    const receipt = latch();
    const writes = [];
    let recovering = false;
    if (!liveHTML) {
      await page.route("**/habits/month?*", (route) => recovering ? route.continue() : route.fulfill({
        contentType: "text/event-stream", body: ": truncated\n\n",
      }));
    }
    await page.route("**/habits/check-in", async (route) => {
      writes.push(route.request().postDataJSON().habitchecked);
      if (writes.length > 1) return route.continue();
      await route.fetch();
      await receipt.promise;
      await route.fulfill({ contentType: "text/event-stream", body: ": receipt lost\n\n" });
    });
    try {
      await button.click();
      if (liveHTML) await expect(button).toHaveAttribute("aria-pressed", "true");
      receipt.release();
      await expect(button).toHaveAttribute("data-save-state", "failed");
      await expect(page.locator("#habit-checkin-feedback")).toContainText("Save not confirmed");
      recovering = true;
      await button.click();
      await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
      expect(writes).toEqual([true, true]);
      await expect(button).toHaveAttribute("aria-pressed", "true");
      await page.reload({ waitUntil: "load" });
      await page.getByRole("tab", { name: "Habits", exact: true }).click();
      await expect(button).toHaveAttribute("aria-pressed", "true");
    } finally {
      receipt.release();
      await page.unrouteAll({ behavior: "wait" });
    }
  });
}
