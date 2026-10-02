import { expect, test } from "@playwright/test";
import { baseURL } from "./session.js";

test("landing empty slots create local blocks and follow gestures", async ({ page }) => {
  const writes = [];
  page.on("request", (request) => {
    if (request.method() === "POST") writes.push(request.url());
  });
  await page.goto(`${baseURL}/login`, { waitUntil: "load" });
  const demo = page.locator(".landing-guide-figure .gc-demo");
  const dialog = page.locator("#landing-create-modal");
  const firstSlot = demo.getByRole("button", { name: "Add block at 9:00", exact: true });
  await expect(firstSlot).toHaveClass("slot-add");
  await expect(demo.locator(".gc-slot > .slot-add")).toHaveCount(2);
  await firstSlot.click();
  await expect(dialog).toBeVisible();
  await dialog.getByRole("textbox", { name: "Name", exact: true }).fill("Discard me");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await firstSlot.click();
  await expect(dialog.getByRole("textbox", { name: "Name", exact: true })).toHaveValue("");
  await dialog.getByRole("textbox", { name: "Name", exact: true }).fill("  Local meeting  ");
  await dialog.getByRole("radio", { name: "Fixed", exact: true }).locator("..").click();
  await dialog.getByRole("button", { name: "Create", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const block = demo.locator('.gc-block[data-id="demo-created-1"]');
  await expect(block).toContainText("Local meeting");
  await expect(block).toHaveAttribute("data-type", "appointment");
  await expect(firstSlot).toBeHidden();
  await expect(demo.locator(".gc-block")).toHaveCount(4);

  const pitch = await demo.locator(".gc-slot").evaluateAll((slots) =>
    slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top);
  const box = await block.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + 6 * pitch, { steps: 6 });
  await page.mouse.up();
  await expect(block).toHaveAttribute("data-slot", "7");
  await expect(firstSlot).toBeVisible();
  await expect(demo.getByRole("button", { name: "Add block at 12:00", exact: true })).toBeHidden();
  const coffee = demo.locator('[data-id="coffee"]');
  const grip = await coffee.locator(".gc-grip").boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2 - pitch, { steps: 3 });
  await page.mouse.up();
  await expect(coffee).toHaveAttribute("data-span", "1");
  await expect(demo.getByRole("button", { name: "Add block at 11:30", exact: true })).toBeVisible();
  expect(writes).toEqual([]);
  await page.reload({ waitUntil: "load" });
  await expect(demo.locator(".gc-block")).toHaveCount(3);
  await expect(firstSlot).toBeVisible();
});
