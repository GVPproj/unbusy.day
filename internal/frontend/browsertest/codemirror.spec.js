import { expect, test } from "@playwright/test";

const baseURL = process.env.BROWSER_SMOKE_URL || "http://127.0.0.1:18199";

test("the Jotpad mounts with local Markdown support", async ({ page }) => {
	const browserErrors = [];
	const failedRequests = [];
	const externalRequests = [];
	const saveRequests = [];

	page.on("pageerror", (error) => browserErrors.push(error.message));
	page.on("console", (message) => {
		if (message.type() === "error") browserErrors.push(message.text());
	});
	page.on("requestfailed", (request) =>
		failedRequests.push(`${request.url()}: ${request.failure()?.errorText}`),
	);
	page.on("response", (response) => {
		if (response.status() >= 400) {
			failedRequests.push(`${response.url()}: HTTP ${response.status()}`);
		}
	});
	page.on("request", (request) => {
		const requestURL = new URL(request.url());
		if (requestURL.origin !== new URL(baseURL).origin) {
			externalRequests.push(request.url());
		}
		if (request.method() === "POST" && requestURL.pathname === "/_smoke/codemirror/save") {
			saveRequests.push(request.url());
		}
	});

	await page.goto(`${baseURL}/_smoke/codemirror`, {
		waitUntil: "load",
	});

	await expect(page.locator("html")).toHaveAttribute("data-smoke", "pass");
	await expect(page.locator("#smoke-result")).toHaveText("pass");
	await expect(page.locator(".cm-editor")).toHaveCount(1);
	await expect(page.locator(".tok-heading", { hasText: "Heading" })).toHaveCount(1);
	await expect(page.locator(".tok-strong", { hasText: "strong" })).toHaveCount(1);
	expect(browserErrors).toEqual([]);
	expect(failedRequests).toEqual([]);
	expect(externalRequests).toEqual([]);
	expect(saveRequests).toHaveLength(1);
});
