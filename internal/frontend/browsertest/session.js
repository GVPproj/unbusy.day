import { readFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { expect } from "@playwright/test";

export const baseURL = process.env.BROWSER_SMOKE_URL || "http://127.0.0.1:18199";

// LogMailer is our inbox; real OTP and rate limiting remain enabled.
export async function signIn(context) {
 const logPath = process.env.BROWSER_SMOKE_LOG;
 expect(logPath, "Expose the dev server log as BROWSER_SMOKE_LOG for OTP login").toBeTruthy();
 const email = `browser-${randomUUID()}@example.com`;
 let sent;
 await expect.poll(async () => {
  sent = await context.request.post(`${baseURL}/login/code`, { data: { email, code: "" } });
  return sent.status();
 }, { timeout: 30_000, intervals: [6000] }).not.toBe(429);
 expect(sent.ok(), `OTP send returned HTTP ${sent.status()}`).toBeTruthy();
 let code;
 await expect.poll(async () => {
  const log = await readFile(logPath, "utf8");
  code = log.match(new RegExp(`login code for ${email.replaceAll(".", "\\.")}: (\\d{6})`))?.[1];
  return code;
 }).toMatch(/^\d{6}$/);
 const verified = await context.request.post(`${baseURL}/login/verify`, { data: { email, code } });
 expect(verified.ok()).toBeTruthy();
 expect((await context.cookies(baseURL)).some((cookie) => cookie.name === "session" && cookie.value !== ""),
  "OTP verification must establish a session").toBe(true);
}
