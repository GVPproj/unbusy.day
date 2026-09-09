import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "UTC" });

async function openHabit(context, page) {
  await signIn(context);
  const today = new Date().toISOString().slice(0, 10);
  const created = await context.request.post(`${baseURL}/habits`, {
    data: { habitname: "Read", habitstart: today, timezone: "UTC" },
  });
  expect(created.ok()).toBe(true);
  await page.goto(baseURL, { waitUntil: "load" });
  await page.getByRole("tab", { name: "Habits", exact: true }).click();
  const button = page.getByRole("button", { name: `Read on ${today}`, exact: true });
  await expect(button).toHaveAttribute("aria-pressed", "false");
  return { button, today, id: Number((await button.getAttribute("id")).split("-")[1]) };
}

function latch() {
  let release;
  const promise = new Promise((resolve) => { release = resolve; });
  return { promise, release };
}

test("a superseded check-in settles only after its post-commit authoritative read", async ({ context, page }) => {
  const { button, today, id } = await openHabit(context, page);
  const reads = latch();
  let requests = 0;
  await page.route("**/habits/month?*", async (route) => {
    requests++;
    await reads.promise;
    await route.continue();
  });
  try {
    const posted = page.waitForResponse((response) => response.url().endsWith("/habits/check-in") && response.request().method() === "POST");
    await button.click();
    await (await posted).finished();
    await expect.poll(() => requests).toBeGreaterThan(0);
    await expect(button).toHaveAttribute("aria-busy", "true");
    await expect(button).toHaveAttribute("aria-pressed", "false");

    // Another device wins before any pending authoritative read reaches the store.
    const superseded = await context.request.post(`${baseURL}/habits/check-in`, {
      data: { habitid: id, habitdate: today, habitchecked: false, timezone: "UTC" },
    });
    expect(superseded.ok()).toBe(true);
    reads.release();
    await expect(button).not.toHaveAttribute("aria-busy", "true");
    await expect(button).toHaveAttribute("aria-pressed", "false");
    await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");

    await page.unroute("**/habits/month?*");
    await button.click();
    await expect(button).toHaveAttribute("aria-pressed", "true");
    await expect(button).not.toHaveAttribute("aria-busy", "true");
  } finally {
    reads.release();
    await page.unrouteAll({ behavior: "wait" });
  }
});

test("an empty correlated month stream releases busy for an explicit retry", async ({ context, page }) => {
  const { button } = await openHabit(context, page);
  await page.route("**/habits/month?*", (route) => route.fulfill({
    contentType: "text/event-stream",
    body: ": truncated before the snapshot\n\n",
  }));
  await button.click();
  await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "failed");
  await expect(button).not.toHaveAttribute("aria-busy", "true");
  await expect(button).toHaveAttribute("aria-pressed", "false");
  await page.unroute("**/habits/month?*");
  await button.click();
  await expect(button).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
});

test("a failed correlated month read releases busy and reconnect restores the last committed state", async ({ context, page }) => {
  await page.addInitScript(() => {
    const fetch = window.fetch.bind(window);
    window.fetch = (input, options) => {
      if (new URL(input, location.href).pathname !== "/events") return fetch(input, options);
      const controller = new AbortController();
      window.disconnectHabitEvents = () => controller.abort();
      return fetch(input, { ...options, signal: AbortSignal.any([options.signal, controller.signal]) });
    };
  });
  const { button, today, id } = await openHabit(context, page);
  await page.route("**/habits/month?*", (route) => route.fulfill({ status: 500, body: "read unavailable" }));
  await button.click();
  await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "failed");
  await expect(button).not.toHaveAttribute("aria-busy", "true");
  await expect(button).toHaveAttribute("aria-pressed", "false");
  await page.unroute("**/habits/month?*");
  const reconnect = latch();
  await page.route("**/events?*", async (route) => {
    await reconnect.promise;
    await route.continue();
  });
  try {
    const reconnected = page.waitForRequest((request) => new URL(request.url()).pathname === "/events");
    await page.evaluate(() => window.disconnectHabitEvents());
    await reconnected;
    const superseded = await context.request.post(`${baseURL}/habits/check-in`, {
      data: { habitid: id, habitdate: today, habitchecked: false, timezone: "UTC" },
    });
    expect(superseded.ok()).toBe(true);
    reconnect.release();
    await expect(page.locator("#companion-status")).toHaveAttribute("data-state", "saved");
    await expect(button).toHaveAttribute("aria-pressed", "false");
    await expect(button).not.toHaveAttribute("aria-busy", "true");
  } finally {
    reconnect.release();
    await page.unrouteAll({ behavior: "wait" });
  }
});
