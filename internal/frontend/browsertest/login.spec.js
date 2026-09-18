import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { baseURL } from "./session.js";

for (const mode of ["typed", "pasted"]) {
 test(`eight-digit OTP browser login establishes a session (${mode})`, { tag: "@smoke" }, async ({ page, context }) => {
  const logPath = process.env.BROWSER_SMOKE_LOG;
  expect(logPath, "Run through scripts/browser-smoke.sh for the LogMailer inbox").toBeTruthy();
  const email = `browser-${randomUUID()}@example.com`;
  if (mode === "typed") await page.setViewportSize({ width: 320, height: 720 });
  await page.goto(baseURL, { waitUntil: "load" });
  await expect(page).toHaveURL(`${baseURL}/login`);
  await page.getByRole("textbox", { name: "Email address" }).fill(email);
  const sent = page.waitForResponse(response => response.url() === `${baseURL}/login/code`);
  await page.getByRole("button", { name: "Send code", exact: true }).click();
  expect((await sent).status()).toBe(200);
  let code;
  await expect.poll(async () => {
   const log = await readFile(logPath, "utf8");
   code = log.match(new RegExp(`login code for ${email.replaceAll(".", "\\.")}: (\\d{8})(?!\\d)`))?.[1];
   return code;
  }).toMatch(/^\d{8}$/);
  const input = page.getByRole("textbox", { name: "One-time code" });
  await expect(page.getByText("An 8-digit code is on its way.", { exact: true })).toBeVisible();
  await expect(input).toHaveAttribute("maxlength", "8");
  await expect(page.locator(".otp-box")).toHaveCount(8);
  const verifications = [];
  page.on("request", request => {
   if (request.url() === `${baseURL}/login/verify` && request.method() === "POST") {
    verifications.push(request);
   }
  });
  if (mode === "typed") {
   await input.pressSequentially(code.slice(0, 7), { delay: 50 });
   await expect(input).toHaveValue(code.slice(0, 7));
   await expect(page.locator(".otp-box")).toHaveText([...code.slice(0, 7), ""]);
   expect(await page.locator(".otp-box").evaluateAll(boxes => boxes.every(box => {
    const bounds = box.getBoundingClientRect();
    return bounds.left >= 0 && bounds.right <= window.innerWidth && box.scrollWidth <= box.clientWidth;
   }))).toBe(true);
   // Allow an erroneous early auto-submit to reach the browser's request listener.
   await page.waitForTimeout(250);
   expect(verifications).toHaveLength(0);
   await input.pressSequentially(code.slice(7));
  } else {
   await context.grantPermissions(["clipboard-read", "clipboard-write"]);
   await page.evaluate(code => navigator.clipboard.writeText(code), code);
   await input.focus();
   await input.press("ControlOrMeta+V");
  }
  await expect(page).toHaveURL(`${baseURL}/`);
  await expect(page.locator("#block-list")).toBeVisible();
  expect(verifications).toHaveLength(1);
  expect(verifications[0].postDataJSON().code).toBe(code);
  const cookie = (await context.cookies(baseURL)).find(cookie => cookie.name === "session");
  expect(cookie).toMatchObject({ httpOnly: true, sameSite: "Lax" });
  expect(cookie.value).toMatch(/^[a-f0-9]{64}$/);
  await page.reload({ waitUntil: "load" });
  await expect(page.locator("#block-list")).toBeVisible();
 });
}
