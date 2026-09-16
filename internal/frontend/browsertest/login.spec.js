import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { baseURL } from "./session.js";

test("real OTP browser login establishes a session", { tag: "@smoke" }, async ({ page, context }) => {
 const logPath = process.env.BROWSER_SMOKE_LOG;
 expect(logPath, "Run through scripts/browser-smoke.sh for the LogMailer inbox").toBeTruthy();
 const email = `browser-${randomUUID()}@example.com`;
 await page.goto(baseURL, { waitUntil: "load" });
 await expect(page).toHaveURL(`${baseURL}/login`);
 await page.getByRole("textbox", { name: "Email address" }).fill(email);
 const sent = page.waitForResponse(response => response.url() === `${baseURL}/login/code`);
 await page.getByRole("button", { name: "Send code", exact: true }).click();
 expect((await sent).status()).toBe(200);
 let code;
 await expect.poll(async () => {
  const log = await readFile(logPath, "utf8");
  code = log.match(new RegExp(`login code for ${email.replaceAll(".", "\\.")}: (\\d{6})`))?.[1];
  return code;
 }).toMatch(/^\d{6}$/);
 await page.getByRole("textbox", { name: "One-time code" }).fill(code);
 await expect(page).toHaveURL(`${baseURL}/`);
 await expect(page.locator("#block-list")).toBeVisible();
 const cookie = (await context.cookies(baseURL)).find(cookie => cookie.name === "session");
 expect(cookie).toMatchObject({ httpOnly: true, sameSite: "Lax" });
 expect(cookie.value).toMatch(/^[a-f0-9]{64}$/);
 await page.reload({ waitUntil: "load" });
 await expect(page.locator("#block-list")).toBeVisible();
});
