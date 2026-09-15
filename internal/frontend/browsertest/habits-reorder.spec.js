import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1440, height: 900 }, timezoneId: "UTC" });

async function openHabits(page) {
  await page.goto(baseURL, { waitUntil: "load" });
  await page.getByRole("tab", { name: "Habits", exact: true }).click();
  await expect(page.locator("#habit-scroll")).toBeVisible();
}

async function namesIn(page) {
  return page.locator("#habit-grid tbody .habit-name").allTextContents();
}

test("dragging a habit persists and converges in a second tab", async ({ context, page }) => {
  await signIn(context);
  for (const name of ["Read", "Walk", "Write"]) {
    const response = await context.request.post(`${baseURL}/habits`, {
      data: { habitname: name, habitstart: "2020-01-01", timezone: "UTC" },
    });
    expect(response.ok()).toBe(true);
  }

  const second = await context.newPage();
  await openHabits(page);
  await openHabits(second);
  await expect.poll(() => namesIn(page)).toEqual(["Read", "Walk", "Write"]);
  await expect.poll(() => namesIn(second)).toEqual(["Read", "Walk", "Write"]);

  const source = page.locator("#habit-grid tbody tr", { hasText: "Write" }).locator("th");
  const target = page.locator("#habit-grid tbody tr", { hasText: "Read" }).locator("th");
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();

  let checkInRequests = 0;
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/habits/check-in") checkInRequests++;
  });
  let releaseSave;
  const saveGate = new Promise((resolve) => { releaseSave = resolve; });
  await page.route("**/habits/reorder", async (route) => {
    await saveGate;
    await route.continue();
  });
  const saved = page.waitForResponse((response) =>
    new URL(response.url()).pathname === "/habits/reorder" && response.request().method() === "POST");
  await page.mouse.move(sourceBox.x + 24, sourceBox.y + sourceBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(targetBox.x + 24, targetBox.y + 2, { steps: 8 });
  await expect.soft(source).toHaveCSS("cursor", "grabbing");
  await page.mouse.up();
  try {
    await expect.configure({ soft: true }).poll(() => namesIn(page)).toEqual(["Write", "Read", "Walk"]);
  } finally {
    releaseSave();
  }
  const savedResponse = await saved;
  expect(savedResponse.ok()).toBe(true);
  expect(savedResponse.request().postDataJSON().habitorder).toEqual([
    expect.objectContaining({ sortOrder: 0 }),
    expect.objectContaining({ sortOrder: 1 }),
    expect.objectContaining({ sortOrder: 2 }),
  ]);
  expect(checkInRequests).toBe(0);

  await expect.poll(() => namesIn(page)).toEqual(["Write", "Read", "Walk"]);
  await expect.poll(() => namesIn(second)).toEqual(["Write", "Read", "Walk"]);

  await page.reload({ waitUntil: "load" });
  await page.getByRole("tab", { name: "Habits", exact: true }).click();
  await expect.poll(() => namesIn(page)).toEqual(["Write", "Read", "Walk"]);
});

test("habit grab cursor lasts from press through returning to the original order", async ({ context, page }) => {
  await signIn(context);
  for (const name of ["Read", "Walk", "Write"]) {
    const response = await context.request.post(`${baseURL}/habits`, {
      data: { habitname: name, habitstart: "2020-01-01", timezone: "UTC" },
    });
    expect(response.ok()).toBe(true);
  }
  await openHabits(page);
  await expect.poll(() => namesIn(page)).toEqual(["Read", "Walk", "Write"]);
  const header = page.locator("#habit-grid tbody tr", { hasText: "Write" }).locator("th");
  const box = await header.boundingBox();
  const first = await page.locator("#habit-grid tbody tr").first().boundingBox();
  const x = box.x + 24;
  const y = box.y + box.height / 2;
  const cursorAt = (y) => page.evaluate(({ x, y }) => {
    return getComputedStyle(document.elementFromPoint(x, y)).cursor;
  }, { x, y });
  await page.mouse.move(x, y);
  await page.mouse.down();
  expect.soft(await cursorAt(y)).toBe("grabbing");
  await page.mouse.move(x, y - 6);
  expect.soft(await cursorAt(y - 6)).toBe("grabbing");
  await expect(page.locator("#habit-grid tbody tr").first().locator("th")).toHaveCSS("cursor", "grabbing");
  await page.mouse.move(x, first.y + 2, { steps: 8 });
  await expect.poll(() => namesIn(page)).toEqual(["Write", "Read", "Walk"]);
  expect.soft(await cursorAt(first.y + 2)).toBe("grabbing");
  await page.mouse.move(x, y, { steps: 8 });
  await expect.poll(() => namesIn(page)).toEqual(["Read", "Walk", "Write"]);
  expect.soft(await cursorAt(y)).toBe("grabbing");
  await page.mouse.up();
  await expect(header).toHaveCSS("cursor", "grab");
  await expect(page.locator("#habit-matrix")).not.toHaveClass(/reordering/);

  await page.mouse.down();
  await expect(header).toHaveCSS("cursor", "grabbing");
  await page.locator("#habit-matrix").evaluate((matrix) => {
    matrix.addEventListener("pointermove", (event) => {
      matrix.releasePointerCapture(event.pointerId);
    }, { once: true });
  });
  await page.mouse.move(x, y - 6);
  await page.mouse.move(x, y - 7);
  await expect(header).toHaveCSS("cursor", "grab");
  await expect(page.locator("#habit-matrix")).not.toHaveClass(/reordering/);
  await page.mouse.up();
});

test("rejected habit reorder restores stored order and shows feedback", async ({ context, page }) => {
  await signIn(context);
  for (const name of ["Read", "Walk"]) {
    const response = await context.request.post(`${baseURL}/habits`, {
      data: { habitname: name, habitstart: "2020-01-01", timezone: "UTC" },
    });
    expect(response.ok()).toBe(true);
  }
  await openHabits(page);
  await expect.poll(() => namesIn(page)).toEqual(["Read", "Walk"]);

  let releaseSave;
  const saveGate = new Promise((resolve) => { releaseSave = resolve; });
  await page.route("**/habits/reorder", async (route) => {
    await saveGate;
    const body = route.request().postDataJSON();
    body.habitorder[1].id = body.habitorder[0].id;
    await route.continue({ postData: JSON.stringify(body) });
  });
  await page.locator("#habit-grid tbody tr", { hasText: "Walk" }).focus();
  await page.keyboard.press("Alt+ArrowUp");
  try {
    await expect.poll(() => namesIn(page)).toEqual(["Walk", "Read"]);
  } finally {
    releaseSave();
  }
  await expect(page.locator("#habit-reorder-feedback")).toContainText("current habit set");
  await expect.poll(() => namesIn(page)).toEqual(["Read", "Walk"]);
});
