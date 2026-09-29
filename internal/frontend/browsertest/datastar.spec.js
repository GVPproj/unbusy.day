import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

const origin = new URL(baseURL).origin;
const datastarPath = /^\/static\/vendor\/datastar\/datastar-\d+\.\d+\.\d+\.js$/;

async function expectLocalDatastar(page, path) {
	const responses = [];
	page.on("response", response => {
		const url = new URL(response.url());
		if (url.origin === origin && datastarPath.test(url.pathname)
			&& response.request().resourceType() === "script") {
			responses.push(response);
		}
	});
	await page.goto(`${baseURL}${path}`, { waitUntil: "load" });
	const scripts = await page.locator('script[type="module"][src]').evaluateAll(
		nodes => nodes.map(node => node.src),
	);
	expect(scripts.filter(src => {
		const url = new URL(src);
		return url.origin === origin && datastarPath.test(url.pathname);
	})).toHaveLength(1);
	expect(responses).toHaveLength(1);
	expect(responses[0].status()).toBe(200);
	expect(await responses[0].finished()).toBeNull();
}

test.beforeEach(async ({ page }) => {
	await page.route("**/*", route => {
		const request = route.request();
		return request.resourceType() === "script" && new URL(request.url()).origin !== origin
			? route.abort("blockedbyclient")
			: route.continue();
	});
});

test("vendored Datastar applies smoke patches and round-trips underscore signals", { tag: "@smoke" }, async ({ page }) => {
	await expectLocalDatastar(page, "/_smoke");
	await expect(page.locator("#smoke-target")).toHaveText("patched by datastar");
	await expect(page.locator("#smoke-signal")).toHaveText("underscore signal stored");
	await expect(page.locator("#smoke-echo")).toHaveText("echoed: underscore signal stored");
});

for (const shell of ["login", "app"]) {
	test(`${shell} shell loads vendored Datastar without off-origin scripts`, { tag: "@smoke" }, async ({ page, context }) => {
		if (shell === "app") await signIn(context);
		await expectLocalDatastar(page, shell === "app" ? "/" : "/login");
		if (shell === "app") {
			await expect(page.locator("#block-list")).toBeVisible();
		} else {
			await expect(page.getByRole("textbox", { name: "Email address" })).toBeVisible();
		}
	});
}
