import { execFile } from "node:child_process";
import { promisify } from "node:util";

export const baseURL = process.env.BROWSER_SMOKE_URL || "http://127.0.0.1:18199";
const run = promisify(execFile);

// Each call creates a fresh owner; only the scratch runner provides this helper.
export async function signIn(context) {
 const helper = process.env.BROWSER_SMOKE_SESSION_HELPER;
 const database = process.env.BROWSER_SMOKE_DB;
 if (!helper || !database) {
  throw new Error("Run scripts/browser-smoke.sh to provide scratch session authentication");
 }
 const url = new URL(baseURL);
 if (url.protocol !== "http:" || url.hostname !== "127.0.0.1") {
  throw new Error("Scratch sessions require the local HTTP smoke server");
 }
 const { stdout } = await run(helper, [database], { timeout: 15_000 });
 const { token, expires } = JSON.parse(stdout);
 await context.addCookies([{
  name: "session", value: token, url: url.origin, expires,
  httpOnly: true, secure: false, sameSite: "Lax",
 }]);
}
